#!/bin/sh
# Fresh, scoped deployment only. Existing/partial installs require inspection.
# Usage: install-fresh-jp.sh BINARY SOURCE_COMMIT BINARY_SHA256
set -eu
umask 077
test "$(id -u)" -eq 0
test "$#" -eq 3
binary=$1
source_commit=$2
binary_sha256=$3
assets=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
printf '%s' "$source_commit" | grep -Eq '^[a-f0-9]{40}$'
printf '%s' "$binary_sha256" | grep -Eq '^[a-f0-9]{64}$'
test -f "$binary"
test "$(sha256sum "$binary" | awk '{print $1}')" = "$binary_sha256"
for path in /opt/linksend /var/lib/linksend /etc/linksend /etc/systemd/system/linksend.service /etc/systemd/system/linksend-backup.service /etc/systemd/system/linksend-backup.timer /etc/systemd/journald@linksend.conf /etc/nginx/sites-available/linksend.conf /etc/nginx/sites-enabled/linksend.conf /etc/logrotate.d/linksend-nginx; do
    if test -e "$path" || test -L "$path"; then printf 'INSPECT_EXISTING=%s\n' "$path"; exit 2; fi
done
if getent passwd linksend >/dev/null; then printf 'INSPECT_EXISTING_USER=linksend\n'; exit 2; fi
if ss -lntH 'sport = :8787' | grep -q .; then printf 'PORT_8787_OCCUPIED\n'; exit 2; fi
test -r /etc/nginx/ssl/cloudflare/oooai.de/cert.pem
test -r /etc/nginx/ssl/cloudflare/oooai.de/privkey.pem
test -r /etc/nginx/ssl/cloudflare/origin-ca-rsa-root.pem
nginx -t
printf 'PHASE=backup\n'
backup=/var/backups/linksend/deploy-$(date -u +%Y%m%dT%H%M%SZ)
install -d -m 700 "$backup" /var/backups/linksend/database
cp -p /etc/nginx/nginx.conf "$backup/nginx.conf.before"
cp -p /etc/nginx/sites-enabled/cloudflare-origin.conf "$backup/cloudflare-origin.conf.before"
test -s "$backup/nginx.conf.before"
test -r "$backup/cloudflare-origin.conf.before"
sha256sum "$backup/"*.before > "$backup/before.sha256"
sha256sum -c "$backup/before.sha256"
printf 'BACKUP_PATH=%s\n' "$backup"
changing=false
rollback() {
    code=$?
    trap - EXIT HUP INT TERM
    set +e
    if "$changing"; then
        printf 'PHASE=rollback PRESERVE_DATABASE=true\n'
        systemctl disable --now linksend-backup.timer linksend.service
        systemctl stop linksend-backup.service
        if test "$(readlink /etc/nginx/sites-enabled/linksend.conf)" = /etc/nginx/sites-available/linksend.conf; then
            rm /etc/nginx/sites-enabled/linksend.conf
        fi
        if nginx -t; then systemctl reload nginx; fi
        printf 'ROLLBACK=project_stopped_database_retained\n'
    fi
    exit "$code"
}
trap rollback EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
changing=true
printf 'PHASE=change\n'
useradd --system --home-dir /var/lib/linksend --shell /usr/sbin/nologin linksend
install -d -m 755 /opt/linksend /opt/linksend/releases /opt/linksend/tools
install -d -m 755 "/opt/linksend/releases/$source_commit"
install -m 755 "$binary" "/opt/linksend/releases/$source_commit/rendezvous"
printf '%s  %s\n' "$binary_sha256" "/opt/linksend/releases/$source_commit/rendezvous" | sha256sum -c -
printf 'source_commit=%s\nbinary_sha256=%s\narchive_build=true\n' "$source_commit" "$binary_sha256" > "/opt/linksend/releases/$source_commit/BUILD-INFO.txt"
ln -s "/opt/linksend/releases/$source_commit" /opt/linksend/current
install -d -m 700 -o linksend -g linksend /var/lib/linksend
install -d -m 750 -o root -g linksend /etc/linksend /etc/linksend/tls
openssl req -x509 -newkey rsa:3072 -nodes -sha256 -days 3650 -subj '/CN=LinkSend backend local CA' -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' -keyout /etc/linksend/tls/backend-ca.key -out /etc/linksend/tls/backend-ca.crt 2> "$backup/openssl-ca.log"
openssl req -new -newkey rsa:3072 -nodes -sha256 -subj '/CN=linksend-backend.internal' -keyout /etc/linksend/tls/server.key -out /etc/linksend/tls/server.csr 2> "$backup/openssl-leaf.log"
cat > /etc/linksend/tls/server.ext <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:linksend-backend.internal,IP:127.0.0.1
EOF
openssl x509 -req -in /etc/linksend/tls/server.csr -CA /etc/linksend/tls/backend-ca.crt -CAkey /etc/linksend/tls/backend-ca.key -CAcreateserial -out /etc/linksend/tls/server.crt -days 397 -sha256 -extfile /etc/linksend/tls/server.ext 2>> "$backup/openssl-leaf.log"
chmod 600 /etc/linksend/tls/backend-ca.key /etc/linksend/tls/backend-ca.srl
chown root:linksend /etc/linksend/tls/server.key /etc/linksend/tls/server.crt
chmod 640 /etc/linksend/tls/server.key /etc/linksend/tls/server.crt
chmod 644 /etc/linksend/tls/backend-ca.crt
openssl verify -CAfile /etc/linksend/tls/backend-ca.crt -verify_hostname linksend-backend.internal /etc/linksend/tls/server.crt
cat > /etc/linksend/server.toml <<'EOF'
listen = "127.0.0.1:8787"
database = "/var/lib/linksend/control.db"
allow_insecure_loopback = false
allow_loopback_candidates = false
tls_cert = "/etc/linksend/tls/server.crt"
tls_key = "/etc/linksend/tls/server.key"
EOF
chown root:linksend /etc/linksend/server.toml
chmod 640 /etc/linksend/server.toml
bootstrap=$(openssl rand -hex 32)
printf 'LINKSEND_BOOTSTRAP_TOKEN=%s\n' "$bootstrap" > /etc/linksend/bootstrap.env
unset bootstrap
chmod 600 /etc/linksend/bootstrap.env
install -m 755 "$assets/linksend-backup.py" /opt/linksend/tools/linksend-backup.py
install -m 644 "$assets/linksend.service" /etc/systemd/system/linksend.service
install -m 644 "$assets/linksend-backup.service" /etc/systemd/system/linksend-backup.service
install -m 644 "$assets/linksend-backup.timer" /etc/systemd/system/linksend-backup.timer
install -m 644 "$assets/journald-linksend.conf" /etc/systemd/journald@linksend.conf
install -m 644 "$assets/nginx-linksend.conf" /etc/nginx/sites-available/linksend.conf
install -m 644 "$assets/linksend-nginx.logrotate" /etc/logrotate.d/linksend-nginx
systemd-analyze verify /etc/systemd/system/linksend.service /etc/systemd/system/linksend-backup.service /etc/systemd/system/linksend-backup.timer
systemctl daemon-reload
systemctl enable --now linksend.service
printf 'PHASE=verify_backend\n'
ready=false
for attempt in 1 2 3 4 5 6 7 8 9 10; do
    if curl --noproxy '*' --silent --show-error --fail --max-time 3 --cacert /etc/linksend/tls/backend-ca.crt --resolve linksend-backend.internal:8787:127.0.0.1 https://linksend-backend.internal:8787/healthz > "$backup/backend-health.json" 2> "$backup/backend-health.err"; then ready=true; break; fi
    sleep 1
done
"$ready"
python3 - "$backup/backend-health.json" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
assert data['status'] == 'ok' and data['capabilities']['relay'] is False
print('BACKEND_HEALTH_RELAY=false')
PY
ln -s /etc/nginx/sites-available/linksend.conf /etc/nginx/sites-enabled/linksend.conf
nginx -t
systemctl reload nginx
curl --noproxy '*' --silent --show-error --fail --max-time 10 --cacert /etc/nginx/ssl/cloudflare/origin-ca-rsa-root.pem --resolve linksend.oooai.de:443:127.0.0.1 https://linksend.oooai.de/healthz > "$backup/origin-health.json"
cmp "$backup/backend-health.json" "$backup/origin-health.json"
systemctl start linksend-backup.service
systemctl enable --now linksend-backup.timer
systemctl is-active --quiet linksend.service linksend-backup.timer
tar -C /etc -czf "$backup/linksend-config.tar.gz" linksend systemd/system/linksend.service systemd/system/linksend-backup.service systemd/system/linksend-backup.timer systemd/journald@linksend.conf nginx/sites-available/linksend.conf logrotate.d/linksend-nginx
chmod 600 "$backup/linksend-config.tar.gz"
sha256sum "$backup/linksend-config.tar.gz" > "$backup/linksend-config.tar.gz.sha256"
sha256sum -c "$backup/linksend-config.tar.gz.sha256"
sha256sum -c "$backup/before.sha256"
cmp /etc/nginx/nginx.conf "$backup/nginx.conf.before"
cmp /etc/nginx/sites-enabled/cloudflare-origin.conf "$backup/cloudflare-origin.conf.before"
printf 'SOURCE_COMMIT=%s\nBINARY_SHA256=%s\n' "$source_commit" "$binary_sha256"
printf 'VERIFY=backend_tls_origin_tls_persistent_db_backup_timer\nDNS_CHANGED=false\nDATABASE_PRESERVED=true\n'
changing=false
trap - EXIT HUP INT TERM
