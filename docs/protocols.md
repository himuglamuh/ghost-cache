# Protocol References

Ghost Cache has separate protocol layers. Keep their version numbers and responsibilities distinct.

| Layer | Version | Specification | Purpose |
|---|---:|---|---|
| Ghost Modem serial | 1 | [protocol.md](protocol.md) | Host control and opaque raw-LoRa packets |
| Ghost Node RF | 3 | [node-protocol.md](node-protocol.md) | Reconciliation, inspection, and peer replication |
| Signed manifest | 2 | [signing.md](signing.md) | Canonical Ed25519 authenticity metadata |
| Legacy ingestion RF | 1 | [publication-protocol.md](publication-protocol.md) | Two-party Publisher/Broadcaster diagnostics |

The firmware implements only the Ghost Modem serial layer. It never parses Ghost Node or publication packets.

The node RF protocol is compact, binary, and limited to 200 bytes per LoRa packet. Current hosts do not negotiate GN versions; all peers in one RF domain should run compatible software.
