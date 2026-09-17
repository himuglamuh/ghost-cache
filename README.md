# Ghost Cache

Ghost Cache lets small Linux computers exchange files directly over LoRa, a slow, long-range-oriented packet radio technology, without Wi-Fi, cellular service, or the Internet. Add a file to one node, and nearby nodes can discover it, download it, verify it, and then pass it onward to other nodes even after the original source disappears.

```text
Linux computer       Linux computer       Linux computer
     |                     |                    |
   USB                   USB                  USB
     |                     |                    |
 LoRa radio  ~~~~~~~>  LoRa radio  ~~~~~~~> LoRa radio
     A                     B                    C

Add file to A
B downloads and verifies it
A disappears
C can now download it from B
```

More formally, Ghost Cache is an offline-first replication system that identifies content by a cryptographic hash and moves it over low-bandwidth LoRa links. It distributes verified files between nearby Linux nodes without infrastructure, central coordination, or an original source that must remain online.

Each node can add local files, discover files held by peers, decide locally what to download, resume interrupted transfers, verify received data, and share every file it has accepted. The radio firmware remains a deliberately simple USB-to-LoRa adapter; replication, storage, policy, and trust live on the host.

## What a Node Is

A physical Ghost Cache node consists of a Linux computer running `ghost-node`, a compatible USB LoRa modem, and an antenna designed for the configured frequency band. The reference node uses a Raspberry Pi-class or other Linux host connected to a Heltec WiFi LoRa 32 V3.

LoRa provides a slow but infrastructure-independent radio link. Ghost Cache is engineered around that constraint: it discovers differences compactly, transfers only approved files, recovers missing pieces, and turns every verified copy into another source.

> **Current publication limit:** one publication may contain at most 253,472 bytes, about 247.5 KiB. Ghost Cache is intended for compact documents, bulletins, small data files, and similar content, not large media.

## Highlights

- Content-addressed publications with full SHA-256 verification
- Resumable, out-of-order, duplicate-safe chunk transfer
- Source-independent partial state and continuation from another peer
- Radix digest reconciliation whose cost follows inventory differences
- Receiver-controlled acquisition by object size
- Optional Ed25519 signatures that authenticate a signing key, plus local trust policy
- Runtime RF configuration with modem readback verification
- Headless systemd deployment with bounded volatile logs
- Identical, application-agnostic firmware on every supported radio

Ghost Cache is for replicated content objects, not chat or general routed messaging. It does not provide confidentiality, anonymity, LoRaWAN integration, multi-hop packet routing, or regulatory policy enforcement.

Traffic is plaintext. Anyone able to receive the configured LoRa channel may observe publication metadata and content. Signatures prove that a key signed the publication; associating that key with a person or organization requires an external trusted exchange. Signatures do not hide content, node activity, or metadata.

New to LoRa? Read the [RF Primer for Ghost Cache Users](docs/rf-primer.md) before changing radio settings.

By default, a node uses permissive signature policy and no additional auto-acquisition size ceiling beyond the current 247.5 KiB publication limit. For unattended deployment, consider setting `max_auto_size` and using `signed` or `trusted` policy. Receiving nodes decide what to acquire; locally added complete content is always included in the shared inventory.

## Glossary

- **Node:** a Linux host running `ghost-node` plus a compatible LoRa modem.
- **Modem:** the dumb USB-to-LoRa adapter; it transmits raw packets but does not understand publications.
- **Publication:** one content file, its manifest, SHA-256, chunk layout, and optional origin signature.
- **Manifest:** compact metadata describing a publication.
- **Known:** metadata is stored locally, but complete content has not been downloaded.
- **Partial:** some content chunks are present, but the publication is incomplete.
- **Complete:** all bytes passed SHA-256 and any required signature checks.
- **Acquisition:** downloading a publication from a peer.
- **Reconciliation:** efficiently discovering how two nodes' complete publication sets differ.
- **Want:** a persistent local approval to acquire a publication that automatic policy deferred.

## Reference Platform

Reference hardware:

- Heltec WiFi LoRa 32 V3
- ESP32-S3 and SX1262
- Linux host connected through USB serial

Default example RF profile:

- US 915 MHz
- SF7, BW125, CR4/5, 5 dBm

The modem boundary is portable to other raw-LoRa hardware. See [Porting the firmware](firmware/PORTING.md).

## Quick Start

Install Go and PlatformIO, then build the host tools and firmware:

```bash
go mod download
go test ./...
go build -o ./ghost-node ./cmd/ghost-node
go build -o ./ghost-radio ./cmd/ghost-radio
pio run --project-dir firmware/heltec-modem
```

Flash each Heltec with the same firmware, replacing the device path:

```bash
pio run --project-dir firmware/heltec-modem \
  --target upload \
  --upload-port /dev/ttyUSB0
```

Copy the sample configuration for Node A and Node B. Set a different data directory and stable serial path in each file:

```bash
cp deploy/config.toml node-a.toml
cp deploy/config.toml node-b.toml
./ghost-node config check --config node-a.toml
./ghost-node config check --config node-b.toml
```

Add a small file to Node A before starting either process. If Node A has a signing identity, the file is signed automatically:

```bash
./ghost-node add --data-dir ./data/node-a ./document.md
```

Start both nodes in separate terminals:

```bash
./ghost-node run --config node-a.toml
./ghost-node run --config node-b.toml
```

Node B will discover the publication, inspect its manifest, apply its own acquisition policy, transfer accepted content, verify it, and begin advertising it as another source.

For a complete first deployment, follow the [Quick Start guide](docs/quickstart.md).

## Core Workflow

```text
DISCOVER -> INSPECT -> DECIDE -> TRANSFER -> VERIFY -> REPLICATE
```

1. Nodes compare compact radix summaries of complete publication IDs.
2. A node requests metadata only for content it does not know.
3. Local size and signature policy determine whether transfer begins.
4. Missing chunks are requested in bounded batches.
5. SHA-256 and any present Ed25519 signature are verified.
6. A committed object becomes indistinguishable from locally originated content.

## Commands

| Command | Purpose |
|---|---|
| `ghost-node run` | Run the replication service |
| `ghost-node add` | Add a local file to the verified object store |
| `ghost-node library` | List complete, partial, and known publications |
| `ghost-node inspect` | Show one publication by ID or unique prefix |
| `ghost-node want` | Persistently approve a deferred publication |
| `ghost-node keygen` | Create an Ed25519 signing identity |
| `ghost-node trust` | Add, list, or remove trusted publisher keys |
| `ghost-node config` | Check or display TOML configuration |
| `ghost-node version` | Display application and GN protocol versions |
| `ghost-radio` | Diagnose and operate a modem directly |

`ghost-publish` and `ghost-broadcast` remain available as legacy two-party diagnostic tools. New deployments should use `ghost-node`.

## Documentation

- [Documentation index](docs/README.md)
- [Quick Start](docs/quickstart.md)
- [RF primer](docs/rf-primer.md)
- [Installation and building](docs/installation.md)
- [Hardware and firmware](docs/hardware.md)
- [Node operations](docs/operations.md)
- [Architecture](docs/architecture.md)
- [Security and trust](docs/security.md)
- [Configuration reference](docs/configuration.md)
- [systemd deployment](docs/service.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Protocol references](docs/protocols.md)

## Project Status

The reference hardware has been physically verified for bidirectional binary transport, multi-packet publication transfer, SHA-256 verification, interrupted resume, source switching, receiver-driven acquisition, live manual wants, radix reconciliation, signed publication relay, trusted-signature verification after the original signer disappears, trust-policy overrides through explicit wants, and host-applied runtime RF configuration. Headless service tooling is covered by automated static tests.

Current constraints and compatibility expectations are documented in [Support and limitations](docs/support.md). RF operation remains the operator's responsibility: attach an appropriate antenna before transmission and comply with local frequency, power, and duty-cycle requirements.

## License

Licensed under the [Apache License 2.0](LICENSE).
