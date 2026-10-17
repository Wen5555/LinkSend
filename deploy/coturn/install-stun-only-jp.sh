#!/bin/sh
# Fresh scoped deployment; install the distribution's coturn separately with
# coturn.service masked. Never changes a firewall, route, proxy, or other unit.
# Usage: install-stun-only-jp.sh LOCAL_IPV4 PROBE PROBE_SHA256 SOURCE_COMMIT
set -eu
umask 077
test "$(id -u)" -eq 0
test "$#" -eq 4
listen_ip=$1
probe=$2
probe_sha=$3
source_commit=$4
assets=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
printf '%s' "$probe_sha" | grep -Eq '^[a-f0-9]{64}$'
printf '%s' "$source_commit" | grep -Eq '^[a-f0-9]{40}$'
python3 - "$listen_ip" <<'PY'
import ipaddress, sys
ip = ipaddress.IPv4Address(sys.argv[1])
assert ip.is_global, 'require a concrete global IPv4'
PY
ip -4 -o addr show scope global | awk '{split($4,a,"/");print a[1]}' | grep -Fxq "$listen_ip"
test -x /usr/bin/turnserver
test -x "$probe"
test "$(sha256sum "$probe" | awk '{print $1}')" = "$probe_sha"
test "$(systemctl is-enabled coturn.service)" = masked
if systemctl is-active --quiet coturn.service; then exit 2; fi
if ss -luntH 'sport = :3478' | grep -q .; then printf 'PORT_3478_OCCUPIED\n'; exit 2; fi
for path in /etc/linksend-stun /etc/systemd/system/linksend-stun.service; do
    if test -e "$path" || test -L "$path"; then printf 'INSPECT_EXISTING=%s\n' "$path"; exit 2; fi
done
id turnserver
# The STUN user must not acquire access to the signaling backend's TLS keys.
if id -Gn turnserver | tr ' ' '\n' | grep -Fxq linksend; then exit 2; fi
test -r /etc/systemd/journald@linksend.conf
printf 'PHASE=backup\n'
backup=/var/backups/linksend/stun-$(date -u +%Y%m%dT%H%M%SZ)
install -d -m 700 "$backup"
printf 'source_commit=%s\nprobe_sha256=%s\nlisten_ipv4=%s\nprevious_service=absent\n' "$source_commit" "$probe_sha" "$listen_ip" > "$backup/before.txt"
dpkg-query -W -f='${Version}\n' coturn > "$backup/package-version.txt"
cp -p "$assets/linksend-stun.service" "$assets/stun-only-jp.conf.in" "$assets/install-stun-only-jp.sh" "$backup/"
sha256sum "$backup/"* > "$backup/before.sha256"
sha256sum -c "$backup/before.sha256"
printf 'BACKUP_PATH=%s\n' "$backup"
changing=false
rollback() {
    code=$?
    trap - EXIT HUP INT TERM
    set +e
    if "$changing"; then
        printf 'PHASE=rollback\n'
        systemctl disable --now linksend-stun.service
        # Retain scoped files and backup for inspection; no existing files existed.
        printf 'ROLLBACK=stun_stopped_files_retained\n'
        ss -luntpH 'sport = :3478'
    fi
    exit "$code"
}
trap rollback EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
changing=true
printf 'PHASE=change\n'
install -d -m 750 -o root -g turnserver /etc/linksend-stun
sed "s/@LISTEN_IPV4@/$listen_ip/" "$assets/stun-only-jp.conf.in" > "$backup/stun-only.conf"
install -m 640 -o root -g turnserver "$backup/stun-only.conf" /etc/linksend-stun/stun-only.conf
install -m 644 "$assets/linksend-stun.service" /etc/systemd/system/linksend-stun.service
systemd-analyze verify /etc/systemd/system/linksend-stun.service
systemctl daemon-reload
systemctl enable --now linksend-stun.service
printf 'PHASE=verify\n'
ready=false
for attempt in 1 2 3 4 5; do
    if systemctl is-active --quiet linksend-stun.service && ss -lunH 'sport = :3478' | grep -Fq "$listen_ip:3478"; then ready=true; break; fi
    sleep 1
done
"$ready"
if ss -lntH 'sport = :3478' | grep -q .; then printf 'UNEXPECTED_TCP_LISTENER\n'; exit 1; fi
"$probe" -bind "$listen_ip" -stun "stun:$listen_ip:3478" -require-srflx=false -timeout 10s
systemctl is-active linksend.service linksend-backup.timer linksend-stun.service
test "$(systemctl is-enabled coturn.service)" = masked
ss -luntpH 'sport = :3478'
sha256sum /etc/linksend-stun/stun-only.conf /etc/systemd/system/linksend-stun.service
printf 'VERIFY=local_binding_and_turn_negative_pass EXTERNAL_UDP=not_tested\n'
changing=false
trap - EXIT HUP INT TERM
