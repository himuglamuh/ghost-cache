# Ghost Cache Documentation

Use this index to move from evaluation to deployment and protocol development.

## Get Started

- [Quick Start](quickstart.md): bring up two nodes and replicate a file
- [RF primer](rf-primer.md): LoRa concepts, radio settings, airtime, and beginner safety
- [Installation](installation.md): prerequisites, builds, and host binaries
- [Hardware](hardware.md): reference board, firmware, flashing, and radio safety
- [Configuration](configuration.md): TOML schema, precedence, and RF settings
- [systemd deployment](service.md): install a persistent headless node

## Glossary

- **Node:** a Linux host running `ghost-node` plus a compatible LoRa modem.
- **Modem:** the USB-to-LoRa transport adapter; it does not understand publications.
- **Publication:** one content file, its metadata, full SHA-256, chunk layout, and optional signature.
- **Manifest:** metadata describing a publication.
- **Known:** metadata is local, but complete content is not.
- **Partial:** some durable chunks are present.
- **Complete:** all content passed integrity and applicable signature checks.
- **Acquisition:** downloading a publication from a peer.
- **Reconciliation:** discovering set differences without sending every publication ID.
- **Want:** a persistent local approval for content automatic policy deferred.

## Operate

- [Node operations](operations.md): add, discover, inspect, approve, sign, and verify content
- [Security](security.md): integrity, signatures, trust policy, and threat boundaries
- [Troubleshooting](troubleshooting.md): serial, radio, replication, signing, and service failures
- [Support and limitations](support.md): tested platform, limits, compatibility, and non-goals

## Understand and Extend

- [Architecture](architecture.md): components, data flow, storage, reconciliation, and scheduling
- [Protocol index](protocols.md): current and legacy wire specifications
- [Ghost Node protocol v3](node-protocol.md): peer discovery and replication
- [Ghost Modem protocol v1](protocol.md): host-to-firmware serial interface
- [Legacy ingestion protocol v1](publication-protocol.md): two-party diagnostic protocol
- [Firmware porting contract](../firmware/PORTING.md)

## Project

- [Contributing](../CONTRIBUTING.md)
- [Security policy](../SECURITY.md)
- [Changelog](../CHANGELOG.md)
- [Apache License 2.0](../LICENSE)
