# Troubleshooting

## Serial Device Is Missing

```bash
pio device list
ls -l /dev/serial/by-id/
dmesg --follow
```

Use a known data-capable cable and another USB port. Stable `/dev/serial/by-id/...` paths may appear only after the adapter is connected.

## Permission Denied

Inspect the dereferenced serial-device group:

```bash
stat -Lc '%G %n' "/dev/serial/by-id/your-modem"
groups
```

Add the user to that group (`uucp` on many Arch systems, `dialout` on many Debian systems), then log out and back in. The service installer performs this step for `ghostcache` when the configured device exists.

## Port Is Busy

Only one process may own a modem:

```bash
fuser /dev/ttyUSB0
```

Close PlatformIO monitor, terminal programs, `ghost-radio`, and duplicate node processes before retrying.

## Upload Cannot Connect

Hold **BOOT**, tap **RESET**, start the upload command, and release **BOOT** when writing begins. Recheck the device path after reset because enumeration can change.

## Modem INFO Times Out

```bash
./ghost-radio info --device /dev/ttyUSB0
```

The client tolerates CP2102 reset pulses and boot text for a bounded startup period. If it still fails, open a 115200 baud monitor, reset the board, and verify the boot diagnostics in [Hardware](hardware.md). Close the monitor before retrying INFO.

## Node Fails During Radio Configuration

Ghost Cache fails closed if SET_CONFIG or readback does not match. Check the startup error and compare every node's frequency, SF, bandwidth, coding rate, preamble, and sync word. Validate TOML first:

```bash
ghost-node config check --config /etc/ghostcache/config.toml
```

## Nodes Do Not Discover Each Other

- Confirm all nodes run the same current GN protocol version.
- Confirm matching RF settings and attached antennas.
- Use a short `advertise_interval` such as `10s` while testing.
- Keep radios separated enough to avoid overload.
- Check volatile logs for root summaries, timeouts, and traffic pressure.

## Radios Are Extremely Close or RSSI Is Very Strong

Closer is not infinitely better. Touching or nearly touching antennas do not represent a normal deployment, and very strong nearby transmitters, near-field coupling, and antenna interaction can produce confusing results. Give radios some separation when practical and keep antennas in a normal orientation.

RSSI values are usually negative dBm; values closer to zero are stronger. A very strong RSSI is not itself proof that every packet should arrive.

## Packet Loss Despite Strong Signal

Strong signal strength does not guarantee delivery. The radios are half duplex, so a node cannot receive while it transmits. Two nodes may transmit at the same time, and unrelated interference can overlap otherwise strong packets.

Occasional missing, duplicate, and out-of-order chunks are expected. Ghost Cache persists accepted chunks, requests missing chunks again, backs off after failures, and repeats reconciliation. Judge success by completed SHA-256-verified publications rather than by perfect packet delivery.

## Publication Is Deferred

Inspect local policy and metadata:

```bash
ghost-node library --data-dir /var/lib/ghostcache
ghost-node inspect --data-dir /var/lib/ghostcache "id-prefix"
```

A publication may exceed `max_auto_size`, be unsigned under `signed`, or use an unknown key under `trusted`. Approve it explicitly with `ghost-node want` if appropriate.

## Signing Identity Is Rejected

The private key must be mode `0600`:

```bash
stat -c '%a %n' /var/lib/ghostcache/identity/signing.key
chmod 600 /var/lib/ghostcache/identity/signing.key
```

Never replace identity metadata independently of its key files.

## Service Does Not Start

```bash
sudo systemctl status ghost-node
sudo journalctl --namespace=ghostcache -u ghost-node -n 100
id ghostcache
```

Confirm the config uses `/var/lib/ghostcache`, the serial path exists, `ghostcache` belongs to its group, and the system runs systemd 245 or newer.
