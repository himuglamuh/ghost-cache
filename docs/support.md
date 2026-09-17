# Support and Limitations

## Hardware Support

The maintained reference target is Heltec WiFi LoRa 32 V3 with ESP32-S3 and SX1262. Other boards require a modem firmware port that preserves the host serial contract. Display hardware is optional.

## Verified Behavior

Physical radios have verified:

- arbitrary binary packet transport in both directions;
- 4 KiB and 16 KiB object transfer with byte-for-byte integrity;
- interrupted transfer resume;
- continuation from another source;
- relay after the original source disappears;
- receiver-driven size and trust policy;
- radix reconciliation;
- Ed25519 signed publication relay;
- trusted-signature verification after the original signer disappears;
- live manual `want`, including local admission-policy overrides;
- host-applied runtime RF configuration.

Software tests additionally cover malformed inputs, policy combinations, runtime RF readback behavior, and systemd deployment contracts. Validate service installation on the target Linux distribution before operational use.

## Current Limits

| Property | Limit |
|---|---:|
| Raw SX1262 packet | 255 bytes |
| Ghost Node RF packet | 200 bytes |
| Content per chunk | 178 bytes |
| Chunks per publication | 1,424 |
| Maximum publication size | 253,472 bytes |
| Chunks per requested batch | 4 |
| Publication ID | 128-bit SHA-256 prefix |
| Integrity hash | Full SHA-256 |

The 128-bit ID and 64-bit reconciliation digests are compact lookup aids. Full SHA-256 is authoritative for content acceptance.

There is no aggregate storage quota, automatic expiration, or garbage collection. Operators must monitor disk capacity and manage local state as described in [Node operations](operations.md).

## Compatibility

- Ghost Modem serial protocol: v1
- Ghost Node RF protocol: v3
- Unsigned disk manifest: v1
- Signed disk manifest: v2
- Legacy Publisher/Broadcaster ingestion protocol: v1

GN protocol versions are not negotiated. Peers must run compatible host software. The modem firmware is independent of GN versions because LoRa payloads are opaque to it.

## Non-Goals

Ghost Cache currently does not implement encryption, anonymous communication, chat, routed messaging, topics, subscriptions, priorities, FEC, fountain codes, USB export, web UI, LoRaWAN, automatic key exchange, certificate authorities, or regulatory enforcement.

## Operational Expectations

LoRa is low bandwidth and half duplex. Transfer time depends on PHY settings, contention, loss, discovery interval, and object size. Ghost Cache favors correctness, bounded airtime units, and eventual retry rather than interactive latency.
