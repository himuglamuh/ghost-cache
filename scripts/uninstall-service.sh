#!/usr/bin/env bash
set -euo pipefail

remove_config=0
remove_data=0
for arg in "$@"; do case "$arg" in --remove-config) remove_config=1;; --remove-data) remove_data=1;; *) echo "unknown argument: $arg" >&2; exit 2;; esac; done
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
if systemctl list-unit-files ghost-node.service >/dev/null 2>&1; then systemctl disable --now ghost-node.service; fi
if systemctl is-active --quiet ghost-node.service; then echo "refusing to uninstall while service is active" >&2; exit 1; fi
rm -f /etc/systemd/system/ghost-node.service /usr/local/bin/ghost-node
rm -f /etc/systemd/journald@ghostcache.conf.d/volatile.conf
rmdir /etc/systemd/journald@ghostcache.conf.d 2>/dev/null || true
systemctl daemon-reload
if [[ $remove_config -eq 1 ]]; then rm -rf /etc/ghostcache; fi
if [[ $remove_data -eq 1 ]]; then rm -rf /var/lib/ghostcache; else echo "preserved /var/lib/ghostcache"; fi
