# Node Operations

## Data Directory

```text
data-dir/
├── node-id
├── identity/
├── trust/
├── known/<publication-id>/
├── partial/<publication-id>/
└── objects/<publication-id>/
    ├── content
    ├── manifest.json
    └── complete
```

`known` stores inspected metadata without content. `partial` stores resumable chunks. `objects` contains complete publications that passed integrity and signature validation. Locally added and remotely acquired objects use the same representation.

## Add Content

```bash
ghost-node add --data-dir /var/lib/ghostcache ./document.md
```

For a systemd installation, stop the service before adding content and run the command as the service account so ownership remains correct:

```bash
sudo systemctl stop ghost-node
sudo -u ghostcache ghost-node add --data-dir /var/lib/ghostcache ./document.md
sudo systemctl start ghost-node
```

`ghost-node` does not currently lock the data directory across processes. Stop the service before `add`, `keygen`, trust-store changes, backup, restore, or manual filesystem maintenance. Read-only `library` and `inspect` commands are safe while the service runs, and `want` is a supported live policy-control operation. Service-account permissions still apply.

The command computes SHA-256, derives the 128-bit publication ID, creates the manifest, chunks and commits the content, and signs it when an identity exists. Input preparation and format conversion are external to Ghost Cache.

To deliberately add unsigned content despite a configured identity:

```bash
ghost-node add --data-dir /var/lib/ghostcache --unsigned ./document.md
```

## Browse the Library

```bash
ghost-node library --data-dir /var/lib/ghostcache
ghost-node inspect --data-dir /var/lib/ghostcache "id-or-unique-prefix"
```

Installed service example:

```bash
sudo -u ghostcache ghost-node library --data-dir /var/lib/ghostcache
```

The catalog reports filename, publication ID, size, state, persistent want status, and signature/trust state. Filenames are not unique identifiers.

Representative output:

```text
STATE         SIZE       SIGNATURE                                   FILENAME                 ID
complete      16.0 KiB   trusted:8c9eaf0f11e8478a2c67b76af631da32    bulletin.md              af3995b457b01234567890abcdef1234
deferred      64.0 KiB   unsigned                                    report.pdf               5cb313a4b9f01234567890abcdef1234
partial+want  22.0 KiB   signed/untrusted:117ac9306f840521a33c4127    update.bin               9e183a62c4d01234567890abcdef1234
```

- `deferred` means the manifest is known but content is not being acquired automatically.
- `partial` means some chunks are durable and acquisition can resume.
- `complete` means content passed integrity and applicable signature validation.
- `+want` means a persistent manual acquisition override exists.

## Approve Deferred Content

```bash
ghost-node want --data-dir /var/lib/ghostcache "id-or-unique-prefix"
```

`want` is safe while `ghost-node run` is active. It writes an atomic marker separate from object and chunk files. The running node notices the marker during subsequent reconciliation and begins acquisition when a source is available:

```bash
sudo -u ghostcache ghost-node want --data-dir /var/lib/ghostcache "id-or-unique-prefix"
```

The marker survives restart. It overrides automatic size and signed/trusted admission policy, but never permits a SHA-256 mismatch or invalid signature.

## Run a Node

```bash
ghost-node run --config /etc/ghostcache/config.toml
```

Use one process per modem and data directory. Stable serial paths are strongly preferred. The node remains in receive mode by default, advertises with jitter, reconciles one branch at a time, and transfers chunks in adaptive batches of at most four.

## Verify Replication

```bash
ID="paste-publication-id-here"
sha256sum "/var/lib/ghostcache/objects/$ID/content"
ghost-node inspect --data-dir /var/lib/ghostcache "$ID"
```

The `complete` marker is written only after full content and any present signature verify. `ghost-node` revalidates complete content and signatures before advertising it.

## Observe a Service

```bash
sudo systemctl status ghost-node
sudo journalctl --namespace=ghostcache -u ghost-node -f
```

Periodic diagnostics include traffic pressure, decoded signal values, inventory size, reconciliation cost, manifest traffic, policy decisions, and content-transfer bytes.

## Storage Lifecycle

Ghost Cache currently has no automatic retention policy or garbage collector. Complete objects, known metadata, partial transfers, and want markers remain until an operator removes them. Monitor free space with normal operating-system tools:

```bash
du -sh /var/lib/ghostcache
df -h /var/lib/ghostcache
```

Publication removal is currently a manual administrative operation. Before deletion, stop the service and back up the data directory. Remove only a complete publication's entire `objects/<id>` directory, or the entire matching `known/<id>` or `partial/<id>` directory; never remove individual chunk or manifest files from active state. Manual deletion is local and is not propagated to peers.

## Backup and Restore

Stop the service to obtain a consistent snapshot:

```bash
sudo systemctl stop ghost-node
sudo sh -c 'umask 077; tar -C /var/lib -czf /root/ghostcache-backup.tgz ghostcache'
sudo systemctl start ghost-node
```

The backup includes complete content, partial recovery state, known metadata, wants, node ID, signing identity, and trust store. Protect it as sensitive because it contains `identity/signing.key` when signing is configured.

Restore while stopped, preserve ownership, and verify the private-key mode:

```bash
sudo systemctl stop ghost-node
sudo tar -C /var/lib -xzf /root/ghostcache-backup.tgz
sudo chown -R ghostcache:ghostcache /var/lib/ghostcache
sudo chmod 600 /var/lib/ghostcache/identity/signing.key
sudo systemctl start ghost-node
```

Skip the `chmod` command when no signing identity exists.

## Upgrades

Ghost Node RF versions are not negotiated. Upgrade all nodes in one RF domain together when the GN protocol version changes. Stop the service, back up `/var/lib/ghostcache`, replace the binary/config as needed, run `ghost-node config check`, and restart. Modem firmware changes are independent unless release notes explicitly say otherwise.
