# Quick Start

This guide creates two Ghost Cache nodes using Heltec WiFi LoRa 32 V3 modems and verifies a replicated file byte for byte.

If LoRa terminology is unfamiliar, read the [RF primer](rf-primer.md) first.

## Requirements

- Two Heltec WiFi LoRa 32 V3 boards with suitable antennas
- Two data-capable USB cables
- A Linux host, or two Linux hosts on the same RF channel
- Go 1.24 or newer
- PlatformIO Core 6

The examples use `/dev/ttyUSB0` and `/dev/ttyUSB1`. Prefer stable `/dev/serial/by-id/...` paths for permanent installations.

## 1. Build

```bash
go mod download
go test ./...
go build -o ./ghost-node ./cmd/ghost-node
go build -o ./ghost-radio ./cmd/ghost-radio
pio run --project-dir firmware/heltec-modem
```

## 2. Flash Both Modems

Connect one board at a time and identify it with `pio device list`.

```bash
pio run --project-dir firmware/heltec-modem \
  --target upload \
  --upload-port /dev/ttyUSB0
```

Flash the same image onto both boards. Keep antennas attached whenever the radio may transmit.

Verify each modem after closing any serial monitor:

```bash
./ghost-radio info --device /dev/ttyUSB0
./ghost-radio info --device /dev/ttyUSB1
```

Both status lines must report the same frequency, spreading factor, bandwidth, coding rate, preamble, and sync word.

## 3. Configure Two Nodes

Create `node-a.toml`:

```toml
[node]
data_dir = "./data/node-a"

[radio]
device = "/dev/ttyUSB0"
frequency_mhz = 915.0
spreading_factor = 7
bandwidth_khz = 125.0
coding_rate = 5
tx_power_dbm = 5
preamble = 8
sync_word = 0x12

[discovery]
advertise_interval = "10s"
startup_backoff = "5s"

[acquisition]
signature_policy = "permissive"
```

> This example uses US 915 MHz settings. Use settings legal for your region.

The 10-second advertisement interval is intentionally aggressive so discovery is visible quickly during evaluation. Real deployments commonly use minutes or longer to reduce airtime.

Create `node-b.toml` with `data_dir = "./data/node-b"` and `device = "/dev/ttyUSB1"`. Keep every radio setting identical.

Ghost Cache generates and persists a random operational node ID automatically. Node IDs coordinate short-lived RF exchanges; they are not user identities, signing keys, or trust relationships.

```bash
./ghost-node config check --config node-a.toml
./ghost-node config check --config node-b.toml
```

## 4. Add a Small File to Node A

```bash
dd if=/dev/urandom of=/tmp/ghost-example.bin bs=16384 count=1 status=none
sha256sum /tmp/ghost-example.bin
./ghost-node add --data-dir ./data/node-a /tmp/ghost-example.bin
```

Keep the publication ID printed by `add`.

## 5. Start Both Nodes

Terminal A:

```bash
./ghost-node run --config node-a.toml
```

Terminal B:

```bash
./ghost-node run --config node-b.toml
```

Startup fails if the modem does not accept and report the configured RF settings.

## 6. Watch Node B Acquire It

Node B should eventually report:

```text
publication discovered: ...
manifest received: ...
received ... chunk ...
publication verified and committed: ...
```

Missing, repeated, or out-of-order chunks are normal on radio links. The final committed line is the success condition.

## 7. Verify the Copy

Set `ID` to the publication ID printed by `add`:

```bash
ID="paste-publication-id-here"
sha256sum "./data/node-a/objects/$ID/content" \
          "./data/node-b/objects/$ID/content"
cmp "./data/node-a/objects/$ID/content" \
    "./data/node-b/objects/$ID/content"
```

`cmp` produces no output on success.

## Next Steps

- Add signing and local trust with the [Security guide](security.md).
- Set acquisition limits and production RF timing in [Configuration](configuration.md).
- Install a headless service using the [systemd guide](service.md).
- Learn how discovery avoids full inventory transfer in [Architecture](architecture.md).
