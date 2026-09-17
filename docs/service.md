# Headless systemd Deployment

Build and prepare a config using a stable `/dev/serial/by-id/...` path, then install:

```bash
go build -o ./ghost-node ./cmd/ghost-node
sudo ./scripts/install-service.sh --binary ./ghost-node --config ./deploy/config.toml
```

The installer validates config, installs `/usr/local/bin/ghost-node`, creates the non-root `ghostcache` system user/group, creates `/etc/ghostcache` and `/var/lib/ghostcache`, preserves an existing config unless `--replace-config` is explicit, detects the configured serial device's group and adds `ghostcache`, installs/enables the unit, and starts it unless `--no-start` is supplied.

## Requirements

- A systemd-based Linux distribution with systemd 245 or newer
- Root access for account, unit, and file installation
- A prebuilt `ghost-node` binary for the target architecture
- A validated config using `node.data_dir = "/var/lib/ghostcache"`
- A connected serial modem when automatic serial-group detection is desired

## Installer Options

| Option | Effect |
|---|---|
| `--binary PATH` | Required executable to install |
| `--config PATH` | Required TOML configuration to install or validate |
| `--no-start` | Enable the unit without starting it |
| `--replace-config` | Explicitly replace an existing installed config |

The installer validates the candidate binary with the effective config before replacing installed files, then validates the installed config as the `ghostcache` user. File and service state is restored if deployment fails. Initial account/directory provisioning is retained for a later retry. The installer dereferences `/dev/serial/by-id/...` before detecting the device group, avoiding broad serial permissions or a global udev rule.

The unit does not depend on IP networking. Content, partial state, identity, and trust persist in `/var/lib/ghostcache`. Operational stdout/stderr uses a dedicated `ghostcache` journal namespace configured with `Storage=volatile`, bounded to 16 MiB under `/run/log/journal`, and erased on reboot. This avoids inheriting the distribution's default journal persistence and creates no `/var/log` file. `LogNamespace` requires systemd 245 or newer.

The unit also enables `NoNewPrivileges`, a private temporary directory, read-only system paths, protected home directories, and a narrow writable path for `/var/lib/ghostcache`.

```bash
sudo systemctl status ghost-node
sudo systemctl restart ghost-node
sudo systemctl stop ghost-node
sudo systemctl enable --now ghost-node
sudo journalctl --namespace=ghostcache -u ghost-node -f
```

Uninstall while preserving config, content, keys, and trust:

```bash
sudo ./scripts/uninstall-service.sh
```

`--remove-config` removes `/etc/ghostcache`; `--remove-data` explicitly and destructively removes `/var/lib/ghostcache`. Neither is the default.

| Uninstall option | Removed |
|---|---|
| no options | Unit, installed binary, and Ghost Cache journal namespace configuration |
| `--remove-config` | Also `/etc/ghostcache` |
| `--remove-data` | Also all publications, partial state, identity, and trust under `/var/lib/ghostcache` |

The uninstaller refuses to continue if systemd cannot stop the active service.

Static validation on a development host:

```bash
bash -n scripts/install-service.sh scripts/uninstall-service.sh
go test ./internal/deploy
```

After installation, validate the deployed unit without modifying an existing binary:

```bash
systemd-analyze verify /etc/systemd/system/ghost-node.service
```

On the target Raspberry Pi, confirm `id ghostcache` includes the group owning the dereferenced serial device, then test restart and reinstall. Reinstall without `--replace-config` must print that it preserved `/etc/ghostcache/config.toml`; uninstall without destructive flags must preserve `/var/lib/ghostcache`, including identity and trust.
