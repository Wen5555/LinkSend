#!/bin/sh
set -eu
printf 'OS\n'; cat /etc/os-release
printf 'ADDRESSES\n'; ip -brief address
printf 'ROUTES\n'; ip route; ip -6 route
printf 'VIRTUALIZATION\n'; systemd-detect-virt || true
printf 'TOOLS\n'; for name in go docker podman unshare ip iptables nft turnserver tcpdump; do command -v "$name" || true; done
printf 'NAMESPACE_SUPPORT\n'; cat /proc/sys/user/max_user_namespaces; cat /proc/sys/kernel/unprivileged_userns_clone 2>/dev/null || true
printf 'ISOLATED_NAMESPACE_CHECK\n'; unshare --user --map-root-user --net sh -c 'ip -brief address; ip route'
