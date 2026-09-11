#!/usr/bin/env bash
set -euo pipefail

# Destructive only inside names prefixed linksend-lab-. Run on a disposable
# Linux VM, never on a production host. This topology uses two independent NAT
# namespaces and overlapping LAN prefixes; it is not a Docker bridge shortcut.
[[ "${LINKSEND_ISOLATED_LAB:-}" == "1" ]] || { echo 'set LINKSEND_ISOLATED_LAB=1 on a disposable Linux lab host' >&2; exit 2; }
[[ ${EUID} -eq 0 ]] || { echo 'root is required for network namespaces' >&2; exit 2; }
: "${LINKSEND_RENDEZVOUS_BIN:?set an absolute Linux rendezvous binary path}"
: "${LINKSEND_CLI_BIN:?set an absolute Linux linksend CLI binary path}"
command -v ip >/dev/null
command -v iptables >/dev/null
command -v openssl >/dev/null
command -v turnserver >/dev/null
command -v python3 >/dev/null

RUN_ID="${LINKSEND_LAB_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
[[ "$RUN_ID" =~ ^[A-Za-z0-9._-]+$ ]] || { echo 'LINKSEND_LAB_RUN_ID contains unsafe characters' >&2; exit 2; }
ARTIFACT_ROOT="$PWD/.artifacts"
EVIDENCE="${LINKSEND_LAB_EVIDENCE:-$ARTIFACT_ROOT/dual-nat-$RUN_ID}"
case "$EVIDENCE" in
  "$ARTIFACT_ROOT"/dual-nat-*) ;;
  *) echo "LINKSEND_LAB_EVIDENCE must be a new path below $ARTIFACT_ROOT with prefix dual-nat-" >&2; exit 2 ;;
esac
[[ ! -e "$EVIDENCE" ]] || { echo "evidence path already exists: $EVIDENCE" >&2; exit 2; }
PREFIX=linksend-lab
PEER_A=$PREFIX-peer-a
NAT_A=$PREFIX-nat-a
WAN=$PREFIX-wan
NAT_B=$PREFIX-nat-b
PEER_B=$PREFIX-peer-b
A_PORT=41001
B_PORT=41002
SERVER_PID=
STUN_PID=
RECEIVER_PID=
SNAPSHOT_DONE=0

snapshot_network() {
  [[ "$SNAPSHOT_DONE" == 0 && -d "$EVIDENCE" ]] || return 0
  SNAPSHOT_DONE=1
  ip netns exec "$NAT_A" iptables -t nat -nvL > "$EVIDENCE/nat-a.txt" 2>/dev/null || true
  ip netns exec "$NAT_B" iptables -t nat -nvL > "$EVIDENCE/nat-b.txt" 2>/dev/null || true
  for ns in "$PEER_A" "$NAT_A" "$WAN" "$NAT_B" "$PEER_B"; do
    ip -n "$ns" -br address 2>/dev/null || true
    ip -n "$ns" route 2>/dev/null || true
  done > "$EVIDENCE/topology.txt"
}

stop_and_wait() {
  local pid="$1"
  [[ -n "$pid" ]] || return 0
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 50); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

cleanup() {
  snapshot_network
  stop_and_wait "$RECEIVER_PID"
  stop_and_wait "$SERVER_PID"
  stop_and_wait "$STUN_PID"
  for ns in "$PEER_A" "$NAT_A" "$WAN" "$NAT_B" "$PEER_B"; do
    ip netns pids "$ns" 2>/dev/null | xargs -r kill 2>/dev/null || true
    ip netns del "$ns" 2>/dev/null || true
  done
  if [[ -d "$EVIDENCE" ]]; then
    rm -rf -- "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received"
    rm -f -- "$EVIDENCE/source.bin" "$EVIDENCE/invite.json" "$EVIDENCE/bootstrap.json" \
      "$EVIDENCE/join.json" \
      "$EVIDENCE/pki/ca.key" "$EVIDENCE/pki/server.key" "$EVIDENCE/pki/server.csr" \
      "$EVIDENCE/pki/ca.srl" "$EVIDENCE/server.toml" "$EVIDENCE/control.db" \
      "$EVIDENCE/control.db-shm" "$EVIDENCE/control.db-wal" "$EVIDENCE/turn.db" \
      "$EVIDENCE/turn.db-shm" "$EVIDENCE/turn.db-wal" "$EVIDENCE/turnserver.pid"
  fi
}
trap cleanup EXIT HUP INT TERM

for ns in "$PEER_A" "$NAT_A" "$WAN" "$NAT_B" "$PEER_B"; do
  ip netns list | awk '{print $1}' | grep -Fxq "$ns" && { echo "namespace already exists: $ns" >&2; exit 2; }
done
mkdir -p "$EVIDENCE" "$EVIDENCE/pki" "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received"
chmod 700 "$EVIDENCE" "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received"

for ns in "$PEER_A" "$NAT_A" "$WAN" "$NAT_B" "$PEER_B"; do
  ip netns add "$ns"
  ip -n "$ns" link set lo up
done

# Two independent point-to-point WAN links; both private sides deliberately use
# 10.77.0.0/24 to exercise overlapping RFC1918 addressing.
ip link add lspa-lan type veth peer name lsna-lan
ip link set lspa-lan netns "$PEER_A"
ip link set lsna-lan netns "$NAT_A"
ip link add lsna-wan type veth peer name lsw-a
ip link set lsna-wan netns "$NAT_A"
ip link set lsw-a netns "$WAN"
ip link add lspb-lan type veth peer name lsnb-lan
ip link set lspb-lan netns "$PEER_B"
ip link set lsnb-lan netns "$NAT_B"
ip link add lsnb-wan type veth peer name lsw-b
ip link set lsnb-wan netns "$NAT_B"
ip link set lsw-b netns "$WAN"

ip -n "$PEER_A" addr add 10.77.0.2/24 dev lspa-lan
ip -n "$NAT_A" addr add 10.77.0.1/24 dev lsna-lan
ip -n "$NAT_A" addr add 198.18.0.2/30 dev lsna-wan
ip -n "$WAN" addr add 198.18.0.1/30 dev lsw-a
ip -n "$PEER_B" addr add 10.77.0.2/24 dev lspb-lan
ip -n "$NAT_B" addr add 10.77.0.1/24 dev lsnb-lan
ip -n "$NAT_B" addr add 198.18.0.6/30 dev lsnb-wan
ip -n "$WAN" addr add 198.18.0.5/30 dev lsw-b
for spec in "$PEER_A:lspa-lan" "$NAT_A:lsna-lan" "$NAT_A:lsna-wan" "$WAN:lsw-a" "$PEER_B:lspb-lan" "$NAT_B:lsnb-lan" "$NAT_B:lsnb-wan" "$WAN:lsw-b"; do
  ip -n "${spec%%:*}" link set "${spec##*:}" up
done
ip -n "$PEER_A" route add default via 10.77.0.1
ip -n "$NAT_A" route add default via 198.18.0.1
ip -n "$PEER_B" route add default via 10.77.0.1
ip -n "$NAT_B" route add default via 198.18.0.5

for nat in "$NAT_A" "$NAT_B"; do
  ip netns exec "$nat" sysctl -q -w net.ipv4.ip_forward=1
  ip netns exec "$nat" iptables -P FORWARD DROP
done
ip netns exec "$WAN" sysctl -q -w net.ipv4.ip_forward=1
ip netns exec "$NAT_A" iptables -A FORWARD -i lsna-lan -o lsna-wan -j ACCEPT
ip netns exec "$NAT_A" iptables -A FORWARD -i lsna-wan -o lsna-lan -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
ip netns exec "$NAT_A" iptables -t nat -A POSTROUTING -o lsna-wan -j MASQUERADE
ip netns exec "$NAT_B" iptables -A FORWARD -i lsnb-lan -o lsnb-wan -j ACCEPT
ip netns exec "$NAT_B" iptables -A FORWARD -i lsnb-wan -o lsnb-lan -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
ip netns exec "$NAT_B" iptables -t nat -A POSTROUTING -o lsnb-wan -j MASQUERADE

# Deterministic full-cone-style UDP mappings make this a reproducible positive
# dual-NAT case. The earlier MASQUERADE-only run remains a separate negative
# result for endpoint-dependent filtering; relay remains disabled.
ip netns exec "$NAT_A" iptables -t nat -I POSTROUTING 1 -p udp -s 10.77.0.2 --sport "$A_PORT" -o lsna-wan -j SNAT --to-source "198.18.0.2:$A_PORT"
ip netns exec "$NAT_A" iptables -t nat -A PREROUTING -i lsna-wan -p udp -d 198.18.0.2 --dport "$A_PORT" -j DNAT --to-destination "10.77.0.2:$A_PORT"
ip netns exec "$NAT_A" iptables -A FORWARD -i lsna-wan -o lsna-lan -p udp -d 10.77.0.2 --dport "$A_PORT" -j ACCEPT
ip netns exec "$NAT_B" iptables -t nat -I POSTROUTING 1 -p udp -s 10.77.0.2 --sport "$B_PORT" -o lsnb-wan -j SNAT --to-source "198.18.0.6:$B_PORT"
ip netns exec "$NAT_B" iptables -t nat -A PREROUTING -i lsnb-wan -p udp -d 198.18.0.6 --dport "$B_PORT" -j DNAT --to-destination "10.77.0.2:$B_PORT"
ip netns exec "$NAT_B" iptables -A FORWARD -i lsnb-wan -o lsnb-lan -p udp -d 10.77.0.2 --dport "$B_PORT" -j ACCEPT

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=LinkSend isolated lab CA' -keyout "$EVIDENCE/pki/ca.key" -out "$EVIDENCE/pki/ca.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj '/CN=198.18.0.1' -keyout "$EVIDENCE/pki/server.key" -out "$EVIDENCE/pki/server.csr" >/dev/null 2>&1
printf 'subjectAltName=IP:198.18.0.1\nextendedKeyUsage=serverAuth\n' > "$EVIDENCE/pki/server.ext"
openssl x509 -req -days 1 -in "$EVIDENCE/pki/server.csr" -CA "$EVIDENCE/pki/ca.crt" -CAkey "$EVIDENCE/pki/ca.key" -CAcreateserial -extfile "$EVIDENCE/pki/server.ext" -out "$EVIDENCE/pki/server.crt" >/dev/null 2>&1
cat > "$EVIDENCE/server.toml" <<EOF
listen = "198.18.0.1:8787"
database = "$EVIDENCE/control.db"
allow_insecure_loopback = false
allow_loopback_candidates = false
tls_cert = "$EVIDENCE/pki/server.crt"
tls_key = "$EVIDENCE/pki/server.key"
EOF
chmod 600 "$EVIDENCE/server.toml" "$EVIDENCE/pki/"*.key
bootstrap_token="$(openssl rand -hex 32)"
ip netns exec "$WAN" env LINKSEND_BOOTSTRAP_TOKEN="$bootstrap_token" "$LINKSEND_RENDEZVOUS_BIN" -config "$EVIDENCE/server.toml" > "$EVIDENCE/server.log" 2>&1 &
SERVER_PID=$!
ip netns exec "$WAN" turnserver -n --stun-only --no-auth --no-cli --no-tls --no-dtls \
  --listening-ip=198.18.0.1 --listening-port=3478 --simple-log \
  --relay-threads 1 \
  --pidfile "$EVIDENCE/turnserver.pid" --log-file "$EVIDENCE/stun.log" \
  --db "$EVIDENCE/turn.db" > "$EVIDENCE/stun-launch.log" 2>&1 &
STUN_PID=$!

ready=0
for _ in $(seq 1 50); do
  if ip netns exec "$WAN" curl -fsS --cacert "$EVIDENCE/pki/ca.crt" https://198.18.0.1:8787/healthz > "$EVIDENCE/health.json"; then ready=1; break; fi
  kill -0 "$SERVER_PID" 2>/dev/null || break
  sleep 0.2
done
[[ "$ready" == 1 ]] || { echo 'isolated rendezvous did not become ready' >&2; exit 1; }

SERVER=https://198.18.0.1:8787
STUN=stun:198.18.0.1:3478
stun_preflight() {
  local namespace="$1"
  local output="$2"
  ip netns exec "$namespace" python3 - > "$output" <<'PY'
import json
import os
import socket
import struct
import time

server = ("198.18.0.1", 3478)
transaction = os.urandom(12)
request = struct.pack("!HHI12s", 0x0001, 0, 0x2112A442, transaction)
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.bind(("10.77.0.2", 0))
sock.settimeout(0.5)
deadline = time.monotonic() + 5
response = None
source = None
while time.monotonic() < deadline:
    sock.sendto(request, server)
    try:
        response, source = sock.recvfrom(2048)
        break
    except TimeoutError:
        continue
if response is None:
    raise SystemExit("STUN_PREFLIGHT_TIMEOUT")
if len(response) < 20:
    raise SystemExit("STUN_PREFLIGHT_SHORT_RESPONSE")
message_type, length, cookie, response_transaction = struct.unpack("!HHI12s", response[:20])
if message_type != 0x0101 or cookie != 0x2112A442 or response_transaction != transaction:
    raise SystemExit("STUN_PREFLIGHT_INVALID_RESPONSE")
print(json.dumps({"result": "PASS", "response_bytes": len(response), "source": f"{source[0]}:{source[1]}"}, sort_keys=True))
PY
}
stun_preflight "$PEER_A" "$EVIDENCE/stun-client-a.json"
stun_preflight "$PEER_B" "$EVIDENCE/stun-client-b.json"
run_a=(ip netns exec "$PEER_A" env SSL_CERT_FILE="$EVIDENCE/pki/ca.crt" "$LINKSEND_CLI_BIN" --server "$SERVER" --data-dir "$EVIDENCE/profile-a")
run_b=(ip netns exec "$PEER_B" env SSL_CERT_FILE="$EVIDENCE/pki/ca.crt" "$LINKSEND_CLI_BIN" --server "$SERVER" --data-dir "$EVIDENCE/profile-b")
"${run_a[@]}" bootstrap --token "$bootstrap_token" --name dual-nat-a > "$EVIDENCE/bootstrap.json"
"${run_a[@]}" invite > "$EVIDENCE/invite.json"
invite_token="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["token"])' "$EVIDENCE/invite.json")"
"${run_b[@]}" join --token "$invite_token" --name dual-nat-b > "$EVIDENCE/join.json"
a_id="$("${run_a[@]}" identity | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
b_id="$("${run_b[@]}" identity | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
"${run_a[@]}" trust --id "$b_id" --fingerprint "$b_id"
"${run_b[@]}" trust --id "$a_id" --fingerprint "$a_id"
dd if=/dev/urandom of="$EVIDENCE/source.bin" bs=1048576 count=8 status=none

"${run_b[@]}" receive --peer "$a_id" --dir "$EVIDENCE/received" --bind "10.77.0.2:$B_PORT" --stun "$STUN" --auto-accept --wait-timeout 30s --evidence > "$EVIDENCE/receive.json" 2> "$EVIDENCE/receive.stderr" &
RECEIVER_PID=$!
online=0
for _ in $(seq 1 50); do
  if "${run_a[@]}" devices 2>/dev/null | python3 -c 'import json,sys; p=sys.argv[1]; raise SystemExit(0 if any(x["id"]==p and x["online"] for x in json.load(sys.stdin)) else 1)' "$b_id"; then online=1; break; fi
  sleep 0.2
done
[[ "$online" == 1 ]] || { echo 'receiver did not become online' >&2; exit 1; }
"${run_a[@]}" send --peer "$b_id" --bind "10.77.0.2:$A_PORT" --stun "$STUN" --evidence "$EVIDENCE/source.bin" > "$EVIDENCE/send.json" 2> "$EVIDENCE/send.stderr"
wait "$RECEIVER_PID"
RECEIVER_PID=
cmp "$EVIDENCE/source.bin" "$EVIDENCE/received/source.bin"
sha256sum "$EVIDENCE/source.bin" "$EVIDENCE/received/source.bin" > "$EVIDENCE/content.sha256"
snapshot_network
nat_a_packets="$(awk '/MASQUERADE|SNAT|DNAT/ {sum += $1} END {print sum + 0}' "$EVIDENCE/nat-a.txt")"
nat_b_packets="$(awk '/MASQUERADE|SNAT|DNAT/ {sum += $1} END {print sum + 0}' "$EVIDENCE/nat-b.txt")"
[[ "$nat_a_packets" -gt 0 && "$nat_b_packets" -gt 0 ]] || { echo 'NAT packet counters did not increase' >&2; exit 1; }
python3 - "$EVIDENCE/send.json" "$EVIDENCE/receive.json" <<'PY'
import json, sys
send = json.load(open(sys.argv[1]))
receive = json.load(open(sys.argv[2]))
for role, result in (("send", send), ("receive", receive)):
    evidence = result["evidence"]
    assert result["transfer"]["bytes"] == 8 * 1024 * 1024, role
    assert result["transfer"]["state"] == "Completed", role
    assert evidence["relay"] is False, role
    assert evidence["connection_method"] == "direct_unknown", role
    assert evidence["stun_requests_sent"] > 0, role
    assert evidence["stun_responses_received"] > 0, role
print("DUAL_NAT_RESULT=PASS")
print(f"SEND_SESSION={send['evidence']['session_id']} SEND_BASE_SOCKET={send['evidence']['base_socket']} SEND_REMOTE={send['evidence']['remote_address']}")
print(f"RECEIVE_SESSION={receive['evidence']['session_id']} RECEIVE_BASE_SOCKET={receive['evidence']['base_socket']} RECEIVE_REMOTE={receive['evidence']['remote_address']}")
print(f"TRANSFER_BYTES={send['transfer']['bytes']}")
PY

printf 'NAT_A_PACKETS=%s\n' "$nat_a_packets"
printf 'NAT_B_PACKETS=%s\n' "$nat_b_packets"
printf 'NAT_MODE=independent_static_udp_mapping\n'

rm -rf "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received"
rm -f "$EVIDENCE/source.bin" "$EVIDENCE/invite.json" "$EVIDENCE/bootstrap.json" "$EVIDENCE/join.json" "$EVIDENCE/pki/ca.key" "$EVIDENCE/pki/server.key" "$EVIDENCE/pki/server.csr" "$EVIDENCE/pki/ca.srl" "$EVIDENCE/server.toml" "$EVIDENCE/control.db" "$EVIDENCE/control.db-shm" "$EVIDENCE/control.db-wal" "$EVIDENCE/turn.db" "$EVIDENCE/turn.db-shm" "$EVIDENCE/turn.db-wal" "$EVIDENCE/turnserver.pid"
for sensitive in \
  "$EVIDENCE/profile-a" "$EVIDENCE/profile-b" "$EVIDENCE/received" \
  "$EVIDENCE/source.bin" "$EVIDENCE/invite.json" "$EVIDENCE/bootstrap.json" \
  "$EVIDENCE/join.json" "$EVIDENCE/pki/ca.key" \
  "$EVIDENCE/pki/server.key" "$EVIDENCE/pki/server.csr" "$EVIDENCE/pki/ca.srl" \
  "$EVIDENCE/server.toml" "$EVIDENCE/control.db" "$EVIDENCE/control.db-shm" \
  "$EVIDENCE/control.db-wal" "$EVIDENCE/turn.db" "$EVIDENCE/turn.db-shm" \
  "$EVIDENCE/turn.db-wal" "$EVIDENCE/turnserver.pid"; do
  [[ ! -e "$sensitive" ]] || { echo "sensitive evidence remains: $sensitive" >&2; exit 1; }
done
echo 'SENSITIVE_CLEANUP=PASS'
echo "EVIDENCE=$EVIDENCE"
