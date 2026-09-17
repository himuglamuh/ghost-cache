#!/usr/bin/env bash
set -euo pipefail

binary=""
config=""
start=1
replace_config=0
while (($#)); do
  case "$1" in
    --binary) binary="$2"; shift 2 ;;
    --config) config="$2"; shift 2 ;;
    --no-start) start=0; shift ;;
    --replace-config) replace_config=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ -n "$binary" && -x "$binary" ]] || { echo "--binary must name an executable ghost-node" >&2; exit 2; }
[[ -n "$config" && -f "$config" ]] || { echo "--config must name a TOML config" >&2; exit 2; }

candidate_binary=$(mktemp)
candidate_config=$(mktemp)
trap 'rm -f "$candidate_binary" "$candidate_config"' EXIT
install -m 0755 "$binary" "$candidate_binary"
if [[ -e /etc/ghostcache/config.toml && $replace_config -eq 0 ]]; then
  install -m 0640 /etc/ghostcache/config.toml "$candidate_config"
else
  install -m 0640 "$config" "$candidate_config"
fi
"$candidate_binary" config check --config "$candidate_config"
candidate_data_dir=$("$candidate_binary" config data-dir --config "$candidate_config")
[[ "$candidate_data_dir" == "/var/lib/ghostcache" ]] || { echo "service requires node.data_dir=/var/lib/ghostcache" >&2; exit 2; }

backup_dir=$(mktemp -d)
for path in usr/local/bin/ghost-node etc/ghostcache/config.toml etc/systemd/system/ghost-node.service etc/systemd/journald@ghostcache.conf.d/volatile.conf; do
  if [[ -e "/$path" ]]; then
    install -d "$backup_dir/$(dirname "$path")"
    cp -a "/$path" "$backup_dir/$path"
  fi
done
committed=0
was_enabled=0
was_active=0
old_groups=""
if id ghostcache >/dev/null 2>&1; then
  old_groups=$(id -nG ghostcache | tr ' ' ',')
fi
was_enabled=$(systemctl is-enabled ghost-node.service 2>/dev/null || true)
if systemctl is-active --quiet ghost-node.service 2>/dev/null; then was_active=1; fi
rollback() {
  if [[ $committed -eq 0 ]]; then
    for path in usr/local/bin/ghost-node etc/ghostcache/config.toml etc/systemd/system/ghost-node.service etc/systemd/journald@ghostcache.conf.d/volatile.conf; do
      if [[ -e "$backup_dir/$path" ]]; then
        install -d "/$(dirname "$path")"
        cp -a "$backup_dir/$path" "/$path"
      elif [[ -e "/$path" ]]; then
        rm -f "/$path"
      fi
    done
    if id ghostcache >/dev/null 2>&1; then usermod -G "$old_groups" ghostcache || true; fi
    systemctl daemon-reload || true
    case "$was_enabled" in
      enabled) systemctl enable ghost-node.service || true ;;
      enabled-runtime) systemctl enable --runtime ghost-node.service || true ;;
      *) systemctl disable ghost-node.service 2>/dev/null || true ;;
    esac
    if [[ $was_active -eq 1 ]]; then systemctl restart ghost-node.service || true; else systemctl stop ghost-node.service 2>/dev/null || true; fi
  fi
  rm -f "$candidate_binary" "$candidate_config"
  rm -rf "$backup_dir"
}
trap rollback EXIT

getent group ghostcache >/dev/null || groupadd --system ghostcache
nologin=$(command -v nologin || true)
[[ -n "$nologin" ]] || nologin=/bin/false
id ghostcache >/dev/null 2>&1 || useradd --system --gid ghostcache --home-dir /var/lib/ghostcache --shell "$nologin" ghostcache
install -d -m 0750 -o ghostcache -g ghostcache /var/lib/ghostcache
install -d -m 0750 -o root -g ghostcache /etc/ghostcache
install -m 0755 "$candidate_binary" /usr/local/bin/ghost-node
if [[ ! -e /etc/ghostcache/config.toml || $replace_config -eq 1 ]]; then
  install -m 0640 -o root -g ghostcache "$candidate_config" /etc/ghostcache/config.toml
else
  echo "preserving existing /etc/ghostcache/config.toml"
fi
runuser -u ghostcache -- /usr/local/bin/ghost-node config check --config /etc/ghostcache/config.toml
data_dir=$(/usr/local/bin/ghost-node config data-dir --config /etc/ghostcache/config.toml)
[[ "$data_dir" == "/var/lib/ghostcache" ]] || { echo "service requires node.data_dir=/var/lib/ghostcache" >&2; exit 2; }
device=$(/usr/local/bin/ghost-node config device --config /etc/ghostcache/config.toml)
if [[ -n "$device" && -c "$device" ]]; then
  serial_group=$(stat -Lc '%G' "$device")
  case "$serial_group" in
    dialout|uucp) ;;
    *) echo "refusing automatic access to unrecognized serial group: $serial_group" >&2; exit 2 ;;
  esac
elif [[ -n "$device" && -e "$device" ]]; then
  echo "configured device is not a character device: $device" >&2
  exit 2
else
  echo "warning: configured serial device does not exist yet; add ghostcache to its owning group after connecting it" >&2
fi
install -m 0644 "$(dirname "$0")/../deploy/ghost-node.service" /etc/systemd/system/ghost-node.service
install -d -m 0755 /etc/systemd/journald@ghostcache.conf.d
install -m 0644 "$(dirname "$0")/../deploy/journald-ghostcache.conf" /etc/systemd/journald@ghostcache.conf.d/volatile.conf
systemctl daemon-reload
systemctl enable ghost-node.service
if [[ $start -eq 1 ]]; then systemctl restart ghost-node.service; fi
if [[ -n "${serial_group:-}" ]]; then
  usermod -aG "$serial_group" ghostcache
  echo "added ghostcache to serial device group: $serial_group"
  if [[ $start -eq 1 ]]; then systemctl restart ghost-node.service; fi
fi
committed=1
