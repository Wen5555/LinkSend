#!/usr/bin/env bash
set -euo pipefail

# E5-D standalone mDNS/DNS-SD IPv6 feasibility lab. It does not exercise
# LinkSend discovery or grant any identity/trust to DNS-SD records.
[[ "$LINKSEND_ISOLATED_LAB" == "1" ]] || { echo "set LINKSEND_ISOLATED_LAB=1" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "root is required for network namespaces" >&2; exit 2; }
: "$LINKSEND_MDNS_BIN"
: "$LINKSEND_MDNS_RUN_ID"
[[ "$LINKSEND_MDNS_RUN_ID" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "unsafe run id" >&2; exit 2; }
[[ "$LINKSEND_MDNS_BIN" == /tmp/codex-ssh/* ]] || { echo "binary must be under /tmp/codex-ssh" >&2; exit 2; }
for tool in ip python3 sha256sum grep tail; do command -v "$tool" >/dev/null || { echo "missing tool: $tool" >&2; exit 2; }; done

RUN_ID="$LINKSEND_MDNS_RUN_ID"
NS_A="linksend-e5d-v6-a"
NS_B="linksend-e5d-v6-b"
A_IF="lse5dv6a"
B_IF="lse5dv6b"
A_ADDR="fd42:5d:1::2"
B_ADDR="fd42:5d:1::3"
ROOT="/tmp/linksend-e5d-mdns-$RUN_ID"
EVIDENCE="$ROOT/evidence"
PUBLISHER_PID=""
CLEANUP_DONE=0
CREATED_NS_A=0
CREATED_NS_B=0
HOST_VETH_CREATED=0
STAGE="preflight"

stop_and_wait() {
  local pid="$1"
  [[ -n "$pid" ]] || return 0
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
cleanup() {
  [[ "$CLEANUP_DONE" == 0 ]] || return 0
  CLEANUP_DONE=1
  stop_and_wait "$PUBLISHER_PID"
  if [[ "$HOST_VETH_CREATED" == 1 ]] && ip link show dev "$A_IF" >/dev/null 2>&1; then ip link del "$A_IF" 2>/dev/null || true; fi
  if [[ "$CREATED_NS_A" == 1 ]]; then ip netns pids "$NS_A" 2>/dev/null | xargs -r kill 2>/dev/null || true; ip netns del "$NS_A" 2>/dev/null || true; fi
  if [[ "$CREATED_NS_B" == 1 ]]; then ip netns pids "$NS_B" 2>/dev/null | xargs -r kill 2>/dev/null || true; ip netns del "$NS_B" 2>/dev/null || true; fi
  rm -f -- "$EVIDENCE/publisher.log" "$EVIDENCE/publisher.json" "$EVIDENCE/browser.json"
  rm -f -- "$LINKSEND_MDNS_BIN"
}
on_exit() {
  local code=$?
  trap - EXIT HUP INT TERM
  if [[ "$code" -ne 0 && -d "$EVIDENCE" ]]; then
    printf 'stage=%s\nexit_code=%s\n' "$STAGE" "$code" > "$EVIDENCE/failure-stage.txt"
    if [[ "$CREATED_NS_A" == 1 && "$CREATED_NS_B" == 1 ]]; then
      {
        printf 'a_tx_packets='
        ip netns exec "$NS_A" cat "/sys/class/net/$A_IF/statistics/tx_packets" 2>/dev/null || true
        printf 'b_rx_packets='
        ip netns exec "$NS_B" cat "/sys/class/net/$B_IF/statistics/rx_packets" 2>/dev/null || true
      } > "$EVIDENCE/failure-network.txt"
    fi
    if [[ -f "$EVIDENCE/publisher.log" ]]; then tail -n 20 "$EVIDENCE/publisher.log" > "$EVIDENCE/failure-publisher-tail.txt"; fi
  fi
  cleanup
  exit "$code"
}

# Preconditions deliberately precede the trap, protecting resources from a
# prior run if this new one cannot start.
for ns in "$NS_A" "$NS_B"; do ip netns list | awk '{print $1}' | grep -Fxq "$ns" && { echo "namespace already exists: $ns" >&2; exit 2; }; done
[[ ! -e "$ROOT" ]] || { echo "root already exists: $ROOT" >&2; exit 2; }
[[ -f "$LINKSEND_MDNS_BIN" ]] || { echo "mDNS binary missing" >&2; exit 2; }
trap on_exit EXIT HUP INT TERM

STAGE="create_same_l2_ula"
chmod 700 "$LINKSEND_MDNS_BIN"
mkdir -p "$EVIDENCE"
chmod 700 "$ROOT" "$EVIDENCE"
sha256sum "$LINKSEND_MDNS_BIN" > "$EVIDENCE/binary.sha256"
ip netns add "$NS_A"
CREATED_NS_A=1
ip netns add "$NS_B"
CREATED_NS_B=1
ip -n "$NS_A" link set lo up
ip -n "$NS_B" link set lo up
ip link add "$A_IF" type veth peer name "$B_IF"
HOST_VETH_CREATED=1
ip link set "$A_IF" netns "$NS_A"
ip link set "$B_IF" netns "$NS_B"
ip -n "$NS_A" -6 addr add "$A_ADDR/64" dev "$A_IF" nodad
ip -n "$NS_B" -6 addr add "$B_ADDR/64" dev "$B_IF" nodad
ip -n "$NS_A" link set "$A_IF" up
ip -n "$NS_B" link set "$B_IF" up
{
  echo "TOPOLOGY=two_endpoint_same_ula_veth"
  echo "NS_A=$NS_A A_ADDR=$A_ADDR/64 IF=$A_IF"
  echo "NS_B=$NS_B B_ADDR=$B_ADDR/64 IF=$B_IF"
  echo "MDNS_GROUP=ff02::fb:5353"
  ip -n "$NS_A" -br -6 address
  ip -n "$NS_B" -br -6 address
} > "$EVIDENCE/topology.txt"

A_TX_BEFORE="$(ip netns exec "$NS_A" cat "/sys/class/net/$A_IF/statistics/tx_packets")"
B_RX_BEFORE="$(ip netns exec "$NS_B" cat "/sys/class/net/$B_IF/statistics/rx_packets")"
STAGE="publish_dns_sd"
ip netns exec "$NS_A" "$LINKSEND_MDNS_BIN" -mode publish -interface "$A_IF" -address "$A_ADDR" -timeout 12s > "$EVIDENCE/publisher.log" 2>&1 &
PUBLISHER_PID=$!
READY=0
for _ in $(seq 1 50); do
  if grep -q "MDNS_PUBLISH_READY" "$EVIDENCE/publisher.log"; then READY=1; break; fi
  kill -0 "$PUBLISHER_PID" 2>/dev/null || break
  sleep 0.1
done
[[ "$READY" == 1 ]] || { echo "mDNS publisher not ready" >&2; exit 1; }

STAGE="browse_dns_sd"
ip netns exec "$NS_B" "$LINKSEND_MDNS_BIN" -mode browse -interface "$B_IF" -address "$B_ADDR" -timeout 8s > "$EVIDENCE/browser.json"
wait "$PUBLISHER_PID"
PUBLISHER_PID=""
grep -v "^MDNS_PUBLISH_READY$" "$EVIDENCE/publisher.log" > "$EVIDENCE/publisher.json"
A_TX_AFTER="$(ip netns exec "$NS_A" cat "/sys/class/net/$A_IF/statistics/tx_packets")"
B_RX_AFTER="$(ip netns exec "$NS_B" cat "/sys/class/net/$B_IF/statistics/rx_packets")"

STAGE="validate_dns_sd"
python3 - "$EVIDENCE/publisher.json" "$EVIDENCE/browser.json" "$A_ADDR" "$A_TX_BEFORE" "$A_TX_AFTER" "$B_RX_BEFORE" "$B_RX_AFTER" > "$EVIDENCE/summary.json" <<'PY'
import json, sys
publisher_path, browser_path, expected_addr, a_tx_before, a_tx_after, b_rx_before, b_rx_after = sys.argv[1:]
publisher = json.load(open(publisher_path, encoding="utf-8"))
browser = json.load(open(browser_path, encoding="utf-8"))
if publisher["result"] != "PASS" or publisher["mode"] != "publish":
    raise SystemExit("PUBLISH_RESULT_INVALID")
if browser["result"] != "PASS" or browser["mode"] != "browse":
    raise SystemExit("BROWSE_RESULT_INVALID")
if browser["service"] != "_linksend._udp" or browser["address"] != expected_addr or browser["port"] != 41001:
    raise SystemExit("DNS_SD_CANDIDATE_INVALID")
if set(browser["txt_keys"]) != {"candidate", "proto"} or publisher["trust_basis"] != "none" or browser["trust_basis"] != "none":
    raise SystemExit("DNS_SD_TRUST_BOUNDARY_INVALID")
if int(a_tx_after) <= int(a_tx_before) or int(b_rx_after) <= int(b_rx_before):
    raise SystemExit("VETH_PACKET_COUNTER_NOT_INCREASED")
json.dump({
    "result": "PASS",
    "scope": "isolated_same_link_ula_ipv6_dns_sd_publish_browse",
    "service": browser["service"],
    "candidate_address": browser["address"],
    "candidate_port": browser["port"],
    "txt_keys": browser["txt_keys"],
    "transport": {"multicast_group": "ff02::fb:5353", "a_tx_packets": [int(a_tx_before), int(a_tx_after)], "b_rx_packets": [int(b_rx_before), int(b_rx_after)]},
    "trust_boundary": "dns_sd_records_are_untrusted_candidate_hints_only",
    "production_integration": "not_implemented",
}, sys.stdout, sort_keys=True, indent=2)
print()
PY
echo "E5D_MDNS_RESULT=PASS"
echo "E5D_MDNS_EVIDENCE=$EVIDENCE"
cleanup
trap - EXIT HUP INT TERM
for sensitive in "$EVIDENCE/publisher.log" "$EVIDENCE/publisher.json" "$EVIDENCE/browser.json"; do
  [[ ! -e "$sensitive" ]] || { echo "runtime result remains: $sensitive" >&2; exit 1; }
done
echo "E5D_MDNS_CLEANUP=PASS"
