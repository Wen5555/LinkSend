#!/usr/bin/env bash
set -euo pipefail

# Reproducible E5-D test-only IPv6 signed discovery and LAN-TLS pin prototype.
# It requires a clean Linux discovery test binary. It only creates the named
# namespaces and a unique /tmp root; it never changes host routing or firewalls.
[[ "$LINKSEND_ISOLATED_LAB" == "1" ]] || { echo "set LINKSEND_ISOLATED_LAB=1" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "root is required for network namespaces" >&2; exit 2; }
: "$LINKSEND_DISCOVERY_TEST_BIN"
: "$LINKSEND_LAB_RUN_ID"
[[ "$LINKSEND_LAB_RUN_ID" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "unsafe run id" >&2; exit 2; }
[[ "$LINKSEND_DISCOVERY_TEST_BIN" == /tmp/codex-ssh/* ]] || { echo "test binary must be under /tmp/codex-ssh" >&2; exit 2; }
for tool in ip python3 sha256sum grep; do command -v "$tool" >/dev/null || { echo "missing tool: $tool" >&2; exit 2; }; done

RUN_ID="$LINKSEND_LAB_RUN_ID"
NS_A="linksend-e5d-v6-a"
NS_B="linksend-e5d-v6-b"
A_IF="lse5dv6a"
B_IF="lse5dv6b"
A_PRIMARY="fd42:5d:1::2"
A_SECONDARY="fd42:5d:1::4"
B_ADDR="fd42:5d:1::3"
ROOT="/tmp/linksend-e5d-v6-discovery-$RUN_ID"
EVIDENCE="$ROOT/evidence"
A_PID=""
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
  stop_and_wait "$A_PID"
  if [[ "$HOST_VETH_CREATED" == 1 ]] && ip link show dev "$A_IF" >/dev/null 2>&1; then ip link del "$A_IF" 2>/dev/null || true; fi
  if [[ "$CREATED_NS_A" == 1 ]]; then ip netns pids "$NS_A" 2>/dev/null | xargs -r kill 2>/dev/null || true; ip netns del "$NS_A" 2>/dev/null || true; fi
  if [[ "$CREATED_NS_B" == 1 ]]; then ip netns pids "$NS_B" 2>/dev/null | xargs -r kill 2>/dev/null || true; ip netns del "$NS_B" 2>/dev/null || true; fi
  rm -f -- "$EVIDENCE/a.log" "$EVIDENCE/b.log" "$LINKSEND_DISCOVERY_TEST_BIN"
}
on_exit() {
  local code=$?
  trap - EXIT HUP INT TERM
  if [[ "$code" -ne 0 && -d "$EVIDENCE" ]]; then printf 'stage=%s\nexit_code=%s\n' "$STAGE" "$code" > "$EVIDENCE/failure-stage.txt"; fi
  cleanup
  exit "$code"
}

# Existing runs are protected because these checks precede the cleanup trap.
for ns in "$NS_A" "$NS_B"; do ip netns list | awk '{print $1}' | grep -Fxq "$ns" && { echo "namespace already exists: $ns" >&2; exit 2; }; done
[[ ! -e "$ROOT" ]] || { echo "root already exists: $ROOT" >&2; exit 2; }
[[ -f "$LINKSEND_DISCOVERY_TEST_BIN" ]] || { echo "discovery test binary missing" >&2; exit 2; }
trap on_exit EXIT HUP INT TERM

STAGE="create_same_l2_ula"
chmod 700 "$LINKSEND_DISCOVERY_TEST_BIN"
mkdir -p "$EVIDENCE"
chmod 700 "$ROOT" "$EVIDENCE"
sha256sum "$LINKSEND_DISCOVERY_TEST_BIN" > "$EVIDENCE/binary.sha256"
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
ip -n "$NS_A" -6 addr add "$A_PRIMARY/64" dev "$A_IF" nodad
ip -n "$NS_A" -6 addr add "$A_SECONDARY/64" dev "$A_IF" nodad
ip -n "$NS_B" -6 addr add "$B_ADDR/64" dev "$B_IF" nodad
ip -n "$NS_A" link set "$A_IF" up
ip -n "$NS_B" link set "$B_IF" up
{
  echo "TOPOLOGY=two_endpoint_same_ula_veth"
  echo "A_ADDRESSES=$A_PRIMARY/64,$A_SECONDARY/64"
  echo "B_ADDRESS=$B_ADDR/64"
  echo "DISCOVERY_MULTICAST=ff12::4c69:6e6b:7365:53318"
  ip -n "$NS_A" -br -6 address
  ip -n "$NS_B" -br -6 address
} > "$EVIDENCE/topology.txt"

STAGE="start_signed_discovery_a"
ip netns exec "$NS_A" env \
  LINKSEND_E5D_DISCOVERY_ROLE=a \
  LINKSEND_E5D_DISCOVERY_INTERFACE="$A_IF" \
  LINKSEND_E5D_DISCOVERY_LOCAL="$A_PRIMARY,$A_SECONDARY" \
  "$LINKSEND_DISCOVERY_TEST_BIN" -test.run '^TestE5DIPv6DiscoveryPrototype$' -test.v > "$EVIDENCE/a.log" 2>&1 &
A_PID=$!
READY=0
for _ in $(seq 1 50); do
  if grep -q 'E5D_DISCOVERY_READY role=a' "$EVIDENCE/a.log"; then READY=1; break; fi
  kill -0 "$A_PID" 2>/dev/null || break
  sleep 0.1
done
[[ "$READY" == 1 ]] || { echo "IPv6 signed discovery A not ready" >&2; exit 1; }

STAGE="run_signed_discovery_b"
ip netns exec "$NS_B" env \
  LINKSEND_E5D_DISCOVERY_ROLE=b \
  LINKSEND_E5D_DISCOVERY_INTERFACE="$B_IF" \
  LINKSEND_E5D_DISCOVERY_LOCAL="$B_ADDR" \
  "$LINKSEND_DISCOVERY_TEST_BIN" -test.run '^TestE5DIPv6DiscoveryPrototype$' -test.v > "$EVIDENCE/b.log" 2>&1
wait "$A_PID"
A_PID=""

STAGE="validate_signed_discovery_and_tls"
python3 - "$EVIDENCE/a.log" "$EVIDENCE/b.log" "$A_PRIMARY" "$A_SECONDARY" > "$EVIDENCE/summary.json" <<'PY'
import json, sys
a_path, b_path, a_primary, a_secondary = sys.argv[1:]
def result(path, role):
    for line in open(path, encoding="utf-8"):
        if line.startswith("E5D_DISCOVERY_RESULT="):
            item = json.loads(line.split("=", 1)[1])
            if item.get("result") == "PASS" and item.get("role") == role:
                return item
    raise SystemExit("MISSING_%s_RESULT" % role.upper())
a = result(a_path, "a")
b = result(b_path, "b")
for item in (a, b):
    if item["family"] != "ipv6" or item["tls_version"] != 772 or item["alpn"] != "linksend/1" or item["valid_signed_packets"] < 1:
        raise SystemExit("INVALID_SIGNED_DISCOVERY_OR_TLS_EVIDENCE")
if b["route_count"] != 2 or sorted(b["remote_addresses"]) != sorted([a_primary, a_secondary]):
    raise SystemExit("DEVICE_ID_MULTI_ADDRESS_MERGE_FAILED")
json.dump({
    "result": "PASS",
    "scope": "test_only_isolated_ipv6_signed_discovery_and_lan_tls_pin",
    "signed_discovery": {"family": "ipv6", "accepted_packets": {"a": a["valid_signed_packets"], "b": b["valid_signed_packets"]}},
    "device_id_route_merge": {"routes": b["route_count"], "remote_addresses": sorted(b["remote_addresses"])},
    "lan_tls": {"version": b["tls_version"], "alpn": b["alpn"], "identity_pin": "PASS"},
    "production_ipv6_discovery": "not_enabled",
}, sys.stdout, sort_keys=True, indent=2)
print()
PY
echo "E5D_IPV6_DISCOVERY_RESULT=PASS"
echo "E5D_IPV6_DISCOVERY_EVIDENCE=$EVIDENCE"
cleanup
trap - EXIT HUP INT TERM
for runtime in "$EVIDENCE/a.log" "$EVIDENCE/b.log"; do
  [[ ! -e "$runtime" ]] || { echo "runtime log remains: $runtime" >&2; exit 1; }
done
echo "E5D_IPV6_DISCOVERY_CLEANUP=PASS"
