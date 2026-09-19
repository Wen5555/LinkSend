#!/usr/bin/env bash
set -euo pipefail

# E5-D disposable IPv6 ULA lab.
[[ "$LINKSEND_ISOLATED_LAB" == "1" ]] || { echo "set LINKSEND_ISOLATED_LAB=1" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "root is required for network namespaces" >&2; exit 2; }
: "$LINKSEND_CLI_BIN"
: "$LINKSEND_RENDEZVOUS_BIN"
: "$LINKSEND_LAB_RUN_ID"
[[ "$LINKSEND_LAB_RUN_ID" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "unsafe run id" >&2; exit 2; }
[[ "$LINKSEND_CLI_BIN" == /tmp/codex-ssh/* && "$LINKSEND_RENDEZVOUS_BIN" == /tmp/codex-ssh/* ]] || { echo "binaries must be under /tmp/codex-ssh" >&2; exit 2; }
for tool in ip openssl curl python3 sha256sum cmp dd grep; do command -v "$tool" >/dev/null || { echo "missing tool: $tool" >&2; exit 2; }; done

RUN_ID="$LINKSEND_LAB_RUN_ID"
NS_A="linksend-e5d-v6-a"
NS_B="linksend-e5d-v6-b"
A_IF="lse5dv6a"
B_IF="lse5dv6b"
A_ADDR="fd42:5d:1::2"
B_ADDR="fd42:5d:1::3"
SERVER_PORT=8787
A_PORT=41001
B_PORT=41002
ROOT="/tmp/linksend-e5d-v6-$RUN_ID"
EVIDENCE="$ROOT/evidence"
SERVER_PID=""
RECEIVER_PID=""
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
cleanup_sensitive() {
  [[ "$CLEANUP_DONE" == 0 ]] || return 0
  CLEANUP_DONE=1
  stop_and_wait "$RECEIVER_PID"
  stop_and_wait "$SERVER_PID"
  if [[ "$HOST_VETH_CREATED" == 1 ]] && ip link show dev "$A_IF" >/dev/null 2>&1; then
    ip link del "$A_IF" 2>/dev/null || true
  fi
  if [[ "$CREATED_NS_A" == 1 ]]; then
    ip netns pids "$NS_A" 2>/dev/null | xargs -r kill 2>/dev/null || true
    ip netns del "$NS_A" 2>/dev/null || true
  fi
  if [[ "$CREATED_NS_B" == 1 ]]; then
    ip netns pids "$NS_B" 2>/dev/null | xargs -r kill 2>/dev/null || true
    ip netns del "$NS_B" 2>/dev/null || true
  fi
  rm -rf -- "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received-a" "$EVIDENCE/received-b"
  rm -f -- "$EVIDENCE/a-to-b.bin" "$EVIDENCE/b-to-a.bin" \
    "$EVIDENCE/a-to-b-send.json" "$EVIDENCE/a-to-b-receive.json" "$EVIDENCE/b-to-a-send.json" "$EVIDENCE/b-to-a-receive.json" \
    "$EVIDENCE/a-to-b-send.stderr" "$EVIDENCE/a-to-b-receive.stderr" "$EVIDENCE/b-to-a-send.stderr" "$EVIDENCE/b-to-a-receive.stderr" \
    "$EVIDENCE/bootstrap.json" "$EVIDENCE/invite.json" "$EVIDENCE/join.json" "$EVIDENCE/devices-a.json" "$EVIDENCE/devices-b.json" "$EVIDENCE/server.log" "$EVIDENCE/health.json" "$EVIDENCE/control-tls.txt" \
    "$EVIDENCE/server.toml" "$EVIDENCE/control.db" "$EVIDENCE/control.db-shm" "$EVIDENCE/control.db-wal" \
    "$EVIDENCE/pki/ca.key" "$EVIDENCE/pki/ca.crt" "$EVIDENCE/pki/ca.srl" "$EVIDENCE/pki/server.key" "$EVIDENCE/pki/server.csr" "$EVIDENCE/pki/server.crt" "$EVIDENCE/pki/server.ext"
  rmdir "$EVIDENCE/pki" 2>/dev/null || true
  rm -f -- "$LINKSEND_CLI_BIN" "$LINKSEND_RENDEZVOUS_BIN"
}
on_exit() {
  local code=$?
  trap - EXIT HUP INT TERM
  if [[ "$code" -ne 0 && -d "$EVIDENCE" ]]; then
    printf 'stage=%s\nexit_code=%s\n' "$STAGE" "$code" > "$EVIDENCE/failure-stage.txt"
  fi
  cleanup_sensitive
  exit "$code"
}
# Preconditions run before installing cleanup so pre-existing lab artifacts
# cannot be removed by a failed new invocation.
for ns in "$NS_A" "$NS_B"; do ip netns list | awk '{print $1}' | grep -Fxq "$ns" && { echo "namespace already exists: $ns" >&2; exit 2; }; done
[[ ! -e "$ROOT" ]] || { echo "root already exists: $ROOT" >&2; exit 2; }
[[ -f "$LINKSEND_CLI_BIN" && -f "$LINKSEND_RENDEZVOUS_BIN" ]] || { echo "clean binaries missing" >&2; exit 2; }
trap on_exit EXIT HUP INT TERM
STAGE="create_lab"
chmod 700 "$LINKSEND_CLI_BIN" "$LINKSEND_RENDEZVOUS_BIN"
mkdir -p "$EVIDENCE/pki" "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received-a" "$EVIDENCE/received-b"
chmod 700 "$ROOT" "$EVIDENCE" "$EVIDENCE/pki" "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received-a" "$EVIDENCE/received-b"
sha256sum "$LINKSEND_CLI_BIN" "$LINKSEND_RENDEZVOUS_BIN" > "$EVIDENCE/binary.sha256"
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
ip -n "$NS_A" -6 route get "$B_ADDR" > "$EVIDENCE/route-a-to-b.txt"
ip -n "$NS_B" -6 route get "$A_ADDR" > "$EVIDENCE/route-b-to-a.txt"
{
  echo "TOPOLOGY=two_endpoint_same_ula_veth"
  echo "NS_A=$NS_A A_ADDR=$A_ADDR/64 IF=$A_IF"
  echo "NS_B=$NS_B B_ADDR=$B_ADDR/64 IF=$B_IF"
  echo "ROUTER_OR_HOST_FORWARDING=not_used"
  ip -n "$NS_A" -br -6 address
  ip -n "$NS_A" -6 route show
  ip -n "$NS_B" -br -6 address
  ip -n "$NS_B" -6 route show
} > "$EVIDENCE/topology.txt"

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=LinkSend E5-D ULA CA" -keyout "$EVIDENCE/pki/ca.key" -out "$EVIDENCE/pki/ca.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj "/CN=$A_ADDR" -keyout "$EVIDENCE/pki/server.key" -out "$EVIDENCE/pki/server.csr" >/dev/null 2>&1
printf "subjectAltName=IP:%s\nextendedKeyUsage=serverAuth\n" "$A_ADDR" > "$EVIDENCE/pki/server.ext"
openssl x509 -req -days 1 -in "$EVIDENCE/pki/server.csr" -CA "$EVIDENCE/pki/ca.crt" -CAkey "$EVIDENCE/pki/ca.key" -CAcreateserial -extfile "$EVIDENCE/pki/server.ext" -out "$EVIDENCE/pki/server.crt" >/dev/null 2>&1
chmod 600 "$EVIDENCE/pki/"*.key
cat > "$EVIDENCE/server.toml" <<EOF
listen = "[$A_ADDR]:$SERVER_PORT"
database = "$EVIDENCE/control.db"
allow_insecure_loopback = false
allow_loopback_candidates = false
tls_cert = "$EVIDENCE/pki/server.crt"
tls_key = "$EVIDENCE/pki/server.key"
EOF

BOOTSTRAP_TOKEN="$(openssl rand -hex 32)"
STAGE="start_ipv6_tls_control"
ip netns exec "$NS_A" env LINKSEND_BOOTSTRAP_TOKEN="$BOOTSTRAP_TOKEN" "$LINKSEND_RENDEZVOUS_BIN" -config "$EVIDENCE/server.toml" > "$EVIDENCE/server.log" 2>&1 &
SERVER_PID=$!
READY=0
for _ in $(seq 1 50); do
  if ip netns exec "$NS_A" env HTTPS_PROXY= HTTP_PROXY= ALL_PROXY= NO_PROXY='*' curl -g --noproxy '*' -6 -fsS --cacert "$EVIDENCE/pki/ca.crt" "https://[$A_ADDR]:$SERVER_PORT/healthz" > "$EVIDENCE/health.json"; then READY=1; break; fi
  kill -0 "$SERVER_PID" 2>/dev/null || break
  sleep 0.2
done
[[ "$READY" == 1 ]] || { echo "isolated IPv6 rendezvous did not become ready" >&2; exit 1; }
printf '' | ip netns exec "$NS_A" openssl s_client -connect "[$A_ADDR]:$SERVER_PORT" -CAfile "$EVIDENCE/pki/ca.crt" -verify_return_error -verify_ip "$A_ADDR" -brief > "$EVIDENCE/control-tls.txt" 2>&1
grep -q "Verification: OK" "$EVIDENCE/control-tls.txt"
SERVER="https://[$A_ADDR]:$SERVER_PORT"
run_a=(ip netns exec "$NS_A" env "SSL_CERT_FILE=$EVIDENCE/pki/ca.crt" HTTPS_PROXY= HTTP_PROXY= ALL_PROXY= NO_PROXY='*' "$LINKSEND_CLI_BIN" --server "$SERVER" --data-dir "$EVIDENCE/profile-a")
run_b=(ip netns exec "$NS_B" env "SSL_CERT_FILE=$EVIDENCE/pki/ca.crt" HTTPS_PROXY= HTTP_PROXY= ALL_PROXY= NO_PROXY='*' "$LINKSEND_CLI_BIN" --server "$SERVER" --data-dir "$EVIDENCE/profile-b")
STAGE="bootstrap"
"${run_a[@]}" bootstrap --token "$BOOTSTRAP_TOKEN" --name e5d-v6-a > "$EVIDENCE/bootstrap.json"
STAGE="invite"
"${run_a[@]}" invite > "$EVIDENCE/invite.json"
INVITE_TOKEN="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["token"])' "$EVIDENCE/invite.json")"
STAGE="join"
"${run_b[@]}" join --token "$INVITE_TOKEN" --name e5d-v6-b > "$EVIDENCE/join.json"
A_ID="$("${run_a[@]}" identity | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
B_ID="$("${run_b[@]}" identity | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
# membership-v2 pairing pins the authenticated group snapshot; refresh A after
# B joins so both profiles hold the current authorization generation. Manual
# legacy trust would discard that membership metadata.
STAGE="refresh_membership"
"${run_a[@]}" devices > "$EVIDENCE/devices-a.json"
"${run_b[@]}" devices > "$EVIDENCE/devices-b.json"
python3 - "$EVIDENCE/devices-a.json" "$EVIDENCE/devices-b.json" "$A_ID" "$B_ID" <<'PY'
import json, sys
for source, local, remote in ((sys.argv[1], sys.argv[3], sys.argv[4]), (sys.argv[2], sys.argv[4], sys.argv[3])):
    devices = json.load(open(source, encoding="utf-8"))
    by_id = {device["id"]: device for device in devices}
    if set((local, remote)) - set(by_id):
        raise SystemExit("MEMBERSHIP_SNAPSHOT_MISSING_DEVICE")
    if not by_id[local].get("trusted") or not by_id[remote].get("trusted"):
        raise SystemExit("MEMBERSHIP_SNAPSHOT_NOT_PINNED")
print("MEMBERSHIP_SNAPSHOT=PASS")
PY
dd if=/dev/urandom of="$EVIDENCE/a-to-b.bin" bs=1048576 count=1 status=none
dd if=/dev/urandom of="$EVIDENCE/b-to-a.bin" bs=1048576 count=1 status=none

"${run_b[@]}" receive --peer "$A_ID" --dir "$EVIDENCE/received-b" --bind "[$B_ADDR]:$B_PORT" --auto-accept --wait-timeout 30s --evidence > "$EVIDENCE/a-to-b-receive.json" 2> "$EVIDENCE/a-to-b-receive.stderr" &
RECEIVER_PID=$!
ONLINE=0
for _ in $(seq 1 50); do
  if "${run_a[@]}" devices 2>/dev/null | python3 -c 'import json,sys; p=sys.argv[1]; raise SystemExit(0 if any(x["id"]==p and x["online"] for x in json.load(sys.stdin)) else 1)' "$B_ID"; then ONLINE=1; break; fi
  sleep 0.2
done
[[ "$ONLINE" == 1 ]] || { echo "B receiver did not become online" >&2; exit 1; }
"${run_a[@]}" send --peer "$B_ID" --bind "[$A_ADDR]:$A_PORT" --evidence "$EVIDENCE/a-to-b.bin" > "$EVIDENCE/a-to-b-send.json" 2> "$EVIDENCE/a-to-b-send.stderr"
wait "$RECEIVER_PID"
RECEIVER_PID=""
cmp "$EVIDENCE/a-to-b.bin" "$EVIDENCE/received-b/a-to-b.bin"

"${run_a[@]}" receive --peer "$B_ID" --dir "$EVIDENCE/received-a" --bind "[$A_ADDR]:$A_PORT" --auto-accept --wait-timeout 30s --evidence > "$EVIDENCE/b-to-a-receive.json" 2> "$EVIDENCE/b-to-a-receive.stderr" &
RECEIVER_PID=$!
ONLINE=0
for _ in $(seq 1 50); do
  if "${run_b[@]}" devices 2>/dev/null | python3 -c 'import json,sys; p=sys.argv[1]; raise SystemExit(0 if any(x["id"]==p and x["online"] for x in json.load(sys.stdin)) else 1)' "$A_ID"; then ONLINE=1; break; fi
  sleep 0.2
done
[[ "$ONLINE" == 1 ]] || { echo "A receiver did not become online" >&2; exit 1; }
"${run_b[@]}" send --peer "$A_ID" --bind "[$B_ADDR]:$B_PORT" --evidence "$EVIDENCE/b-to-a.bin" > "$EVIDENCE/b-to-a-send.json" 2> "$EVIDENCE/b-to-a-send.stderr"
wait "$RECEIVER_PID"
RECEIVER_PID=""
cmp "$EVIDENCE/b-to-a.bin" "$EVIDENCE/received-a/b-to-a.bin"
python3 - "$EVIDENCE" "$A_ADDR" > "$EVIDENCE/summary.json" <<'PY'
import hashlib, json, os, sys
root, a_addr = sys.argv[1:]
size = 1024 * 1024
pairs = (
    ("a_to_b", "a-to-b-send.json", "a-to-b-receive.json", "a-to-b.bin", "received-b/a-to-b.bin"),
    ("b_to_a", "b-to-a-send.json", "b-to-a-receive.json", "b-to-a.bin", "received-a/b-to-a.bin"),
)
out = {
    "result": "PASS",
    "scope": "isolated_same_link_ula_ipv6_signal_ice_quic",
    "topology": {"namespaces": 2, "prefix": "fd42:5d:1::/64", "router_or_host_forwarding": "not_used"},
    "control_tls": {"scheme": "https_wss", "server_ip_san": a_addr, "openssl_verify_ip": "PASS", "cli_authenticated_https_wss": "PASS"},
    "transfers": [],
}
for direction, sf, rf, source, received in pairs:
    send = json.load(open(os.path.join(root, sf), encoding="utf-8"))
    receive = json.load(open(os.path.join(root, rf), encoding="utf-8"))
    sha = hashlib.sha256(open(os.path.join(root, source), "rb").read()).hexdigest()
    recv_sha = hashlib.sha256(open(os.path.join(root, received), "rb").read()).hexdigest()
    if sha != recv_sha:
        raise AssertionError(direction + ": sha256 mismatch")
    item = {"direction": direction, "source_sha256": sha, "received_sha256": recv_sha, "participants": []}
    for role, record in (("send", send), ("receive", receive)):
        transfer, evidence = record["transfer"], record["evidence"]
        if transfer["state"] != "Completed" or transfer["bytes"] != size:
            raise AssertionError(direction + "/" + role + ": transfer")
        if evidence["address_family"] != "ipv6" or evidence["transport_protocol"] != "quic" or evidence["relay"] is not False:
            raise AssertionError(direction + "/" + role + ": IPv6 QUIC relay")
        if evidence["tls_version"] != 772 or evidence["alpn"] != "linksend/1":
            raise AssertionError(direction + "/" + role + ": TLS")
        if evidence["local_type"] != "host" or evidence["remote_type"] != "host":
            raise AssertionError(direction + "/" + role + ": candidates")
        if evidence["signaling_bytes_sent"] >= size or evidence["signaling_bytes_received"] >= size:
            raise AssertionError(direction + "/" + role + ": signaling accounting")
        item["participants"].append({
            "role": role,
            "transfer_bytes": transfer["bytes"],
            "transfer_digest": transfer["digest"],
            "address_family": evidence["address_family"],
            "local_candidate_type": evidence["local_type"],
            "remote_candidate_type": evidence["remote_type"],
            "connection_method": evidence["connection_method"],
            "transport_protocol": evidence["transport_protocol"],
            "relay": evidence["relay"],
            "tls_version": evidence["tls_version"],
            "alpn": evidence["alpn"],
            "signaling_bytes_sent": evidence["signaling_bytes_sent"],
            "signaling_bytes_received": evidence["signaling_bytes_received"],
        })
    out["transfers"].append(item)
json.dump(out, sys.stdout, sort_keys=True, indent=2)
print()
PY

grep -q "TOPOLOGY=two_endpoint_same_ula_veth" "$EVIDENCE/topology.txt"
grep -q "Verification: OK" "$EVIDENCE/control-tls.txt"
echo "E5D_IPV6_QUIC_RESULT=PASS"
echo "E5D_EVIDENCE=$EVIDENCE"
cleanup_sensitive
trap - EXIT HUP INT TERM
for sensitive in "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received-a" "$EVIDENCE/received-b" "$EVIDENCE/a-to-b.bin" "$EVIDENCE/b-to-a.bin" "$EVIDENCE/bootstrap.json" "$EVIDENCE/invite.json" "$EVIDENCE/join.json" "$EVIDENCE/server.toml" "$EVIDENCE/control.db" "$EVIDENCE/pki"; do
  [[ ! -e "$sensitive" ]] || { echo "sensitive material remains: $sensitive" >&2; exit 1; }
done
echo "E5D_SENSITIVE_CLEANUP=PASS"
