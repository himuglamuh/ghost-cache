# Ghost Node Configuration

## Initial Local Evaluation

For a first test, change only the paths and regional frequency unless you know why another value is needed:

| Setting | Beginner guidance |
|---|---|
| `radio.device` | Set this to the modem serial path. Prefer `/dev/serial/by-id/...`. |
| `node.data_dir` | Set a different directory for each node. |
| `radio.frequency_mhz` | Use a legal regional band and exactly match every peer. |
| `radio.spreading_factor` | Leave at 7 initially. Lower SF is faster; higher SF is slower but can improve sensitivity. |
| `radio.bandwidth_khz` | Leave at 125 kHz for the initial evaluation. |
| `radio.coding_rate` | Leave at 5, which means LoRa coding rate 4/5. |
| `radio.tx_power_dbm` | 5 dBm is deliberately low for nearby evaluation radios. |
| `radio.preamble` | Leave at 8 unless you have an interoperability reason to change it. |
| `radio.sync_word` | Leave at `0x12` and match peers. It is not a password or security key. |

The example uses US 915 MHz settings. Use settings legal for your region. Read the [RF primer](rf-primer.md) before tuning radio parameters.

Run with TOML:

```bash
ghost-node config check --config /etc/ghostcache/config.toml
ghost-node config show --config /etc/ghostcache/config.toml
ghost-node run --config /etc/ghostcache/config.toml
```

## Precedence

```text
firmware settings
       |
       v
configuration file
       |
       v
explicit CLI overrides
```

See `deploy/config.toml` for the schema. When no RF field is configured, firmware settings remain active. When any RF field is supplied, the host constructs and sends a complete configuration from its defaults plus TOML and CLI overrides.

Configurable RF values are frequency MHz, spreading factor, bandwidth kHz, coding-rate denominator, TX power dBm, preamble symbols, and sync word. On startup the node opens/synchronizes the modem, reads INFO, sends SET_CONFIG when configured, validates the returned STATUS, reads INFO again, validates every effective value, logs the result, and only then begins Ghost Cache traffic. Failure is fatal.

Example:

```toml
[node]
data_dir = "/var/lib/ghostcache"

[radio]
device = "/dev/serial/by-id/usb-Silicon_Labs_CP2102..."
frequency_mhz = 915.0
spreading_factor = 7
bandwidth_khz = 125.0
coding_rate = 5
tx_power_dbm = 5
preamble = 8
sync_word = 0x12

[discovery]
advertise_interval = "10m"
startup_backoff = "5s"

[acquisition]
max_auto_size = "128KiB"
signature_policy = "permissive"
```

## Acquisition Defaults

By default, `signature_policy = "permissive"` allows unsigned content and verifies signatures when present. If `max_auto_size` is omitted, the node may automatically acquire any unknown publication that fits the current protocol limit of about 247.5 KiB.

For an unattended deployment, consider setting `max_auto_size = "128KiB"` and choosing `signed` or `trusted` signature policy. Locally added complete content is always shared; acquisition settings control only what this node receives.

CLI equivalents include `--frequency`, `--sf`, `--bandwidth`, `--cr`, `--power`, `--preamble`, `--sync`, `--advertise-interval`, `--max-auto-size`, and `--signature-policy`.

## Reference

| TOML key | Type | Required | Description |
|---|---|---:|---|
| `node.data_dir` | path | yes | Persistent node state and object store |
| `node.id` | hex string | no | Operational RF node ID; generated and persisted when omitted |
| `radio.device` | path | yes | Modem serial device; prefer `/dev/serial/by-id/...` |
| `radio.frequency_mhz` | float | no | LoRa carrier frequency in MHz |
| `radio.spreading_factor` | integer | no | Spreading factor 5 through 12 |
| `radio.bandwidth_khz` | float | no | Positive LoRa bandwidth in kHz |
| `radio.coding_rate` | integer | no | Denominator 5 through 8 for coding rates 4/5 through 4/8 |
| `radio.tx_power_dbm` | integer | no | Requested transmit power, validated within -9 through 22 dBm |
| `radio.preamble` | integer | no | Positive preamble length in symbols |
| `radio.sync_word` | integer | no | One-byte private LoRa sync word |
| `discovery.advertise_interval` | duration | no | Base interval between jittered radix root summaries |
| `discovery.startup_backoff` | duration | no | Maximum randomized listen-only startup delay |
| `acquisition.max_auto_size` | byte size | no | Inclusive automatic-acquisition ceiling; omitted means unlimited |
| `acquisition.signature_policy` | enum | no | `permissive`, `signed`, or `trusted`; default `permissive` |

Durations use Go syntax such as `30s`, `10m`, or `1h`. Byte sizes accept values such as `32768`, `32KiB`, and `128KiB`. The publication protocol itself currently stops at about 247.5 KiB. Unknown TOML keys are rejected so misspellings cannot silently change RF behavior.

## Precedence Example

```bash
ghost-node run --config /etc/ghostcache/config.toml --power 4
```

This loads every value from TOML and replaces only TX power with 4 dBm. When at least one RF value is supplied by TOML or CLI, Ghost Cache sends a complete radio configuration assembled from the host binary's `modem.DefaultConfig` plus all overrides. The current host defaults match the reference firmware, but ports with different firmware defaults should configure every RF field explicitly.

## Validation and Readback

`config check` validates syntax, required paths, known keys, durations, sizes, signature policy, and obvious radio bounds without opening the modem.

At runtime, Ghost Cache requires exact values in the modem status response after configuration. A mismatch is fatal; the node never begins discovery with uncertain RF settings. This check verifies configuration application, not regional legality or actual RF output calibration.

Lab acceptance:

```bash
cp deploy/config.toml /tmp/ghost-config.toml
# Set device to the actual stable path.
./ghost-node config check --config /tmp/ghost-config.toml
./ghost-node run --config /tmp/ghost-config.toml
```

The startup log must confirm 915 MHz, SF7, BW125, CR4/5, 5 dBm, preamble 8, and sync `0x12`. Verify precedence by overriding one compatible value, for example `--power 4`, and confirm the readback log changes. A deliberately incompatible peer setting should apply and be logged clearly but prevent RF communication; restore matching values afterward.
