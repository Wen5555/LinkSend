#!/bin/sh
set -eu
printf 'SERVICE\n'; systemctl show linksend-rendezvous.service -p ActiveState -p SubState -p MainPID
printf 'BINARY\n'; sha256sum /opt/linksend-lan-test/rendezvous
printf 'LISTENERS\n'; ss -lntup | awk 'NR==1 || /:443 |:3478 /'
printf 'PUBLIC_HEALTH\n'; curl --fail --silent --show-error --max-time 15 https://linksend.oooai.de/healthz
