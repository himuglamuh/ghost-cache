# Ghost Node Replication Protocol v3

Ghost nodes replicate verified content objects. This is not a routed message protocol and node IDs are not identities or ownership records.

## Envelope

Every RF packet is binary and at most 200 bytes. Integers are little-endian.

| Offset | Size | Field |
|---:|---:|---|
| 0 | 2 | magic `GN` |
| 2 | 1 | version `3` |
| 3 | 1 | message type |
| 4 | 4 | temporary operational source node ID |
| 8 | 4 | destination node ID, or `ffffffff` for broadcast |
| 12 | 2 | transaction number |
| 14 | variable | message body |

The persisted random node ID coordinates half-duplex replies and rejects unrelated responses. It is not cryptographic, is not attached to stored publications, and conveys no provenance.

## Radix Reconciliation

The legacy full-ID `INVENTORY` packet remains represented in the implementation for tests/history but `ghost-node` no longer sends it. Because discovery and signed-manifest inspection changed incompatibly, current nodes use GN version 3 and do not claim rolling RF compatibility with older GN versions. The modem serial protocol remains v1. Nodes periodically broadcast a radix root `SUMMARY` and exchange only differing branches.

Publication IDs are traversed as 32 hexadecimal nibbles. For each of the sixteen immediate children of a prefix, a summary contains:

| Size | Field |
|---:|---|
| 2 | publication count, saturated at 65535 |
| 8 | first 64 bits of SHA-256 over sorted complete publication IDs |

The digest is reconciliation evidence, not content integrity. A 64-bit collision could delay discovery by making a branch appear equal; it cannot cause corrupt content acceptance because manifests bind the 128-bit publication ID to the full SHA-256 and completed content is verified with the full hash.

`SUMMARY_REQUEST` (`7`) body:

| Size | Field |
|---:|---|
| 4 | source-local inventory generation |
| 1 | prefix depth in nibbles, 0-32 |
| ceil(depth/2) | packed prefix nibbles |

`SUMMARY` (`8`) uses the same generation/prefix header followed by sixteen 10-byte child summaries. Packet sizes are:

- root: 179 bytes;
- deepest prefix: 195 bytes;
- always within the 200-byte application limit.

`LEAF` (`9`) uses the generation/prefix header, a one-byte count, and concrete 16-byte publication IDs. A branch becomes a leaf at ten or fewer IDs. The worst-case deepest leaf is 196 bytes.

Generation is the first 32 bits of SHA-256 over the sorted complete inventory. It is local snapshot bookkeeping, not synchronized time. If a response generation differs from the root that began reconciliation, the receiver drops that RAM-only walk and waits for a later root.

Reconciliation starts by comparing root child summaries. Equal branches stop immediately. Differing branches are requested one at a time through the normal scheduler until a summary matches or a leaf identifies concrete differences. No durable peer synchronization state is required.

Deterministic software measurements for inventories differing by one object:

| Inventory size | Reconciliation packets | RF bytes | Concrete IDs |
|---:|---:|---:|---:|
| 1,000 | 5 | 484 | 4 |
| 10,000 | 7 | 686 | 4 |

Counts include the broadcast root and each addressed request/response needed to identify the missing ID. They exclude the subsequent manifest and any accepted bulk transfer.

The old `INVENTORY` (`1`) body was:

| Size | Field |
|---:|---|
| 4 | CRC-32 generation over the sorted complete IDs |
| 2 | zero-based page |
| 2 | page count |
| 1 | ID count |
| 16 x count | publication IDs |

It carried eleven IDs per 199-byte page and has been superseded because its airtime scaled with total inventory size.

## Acquisition

- `GET_MANIFEST` (`2`): addressed request containing a 16-byte publication ID.
- `MANIFEST` (`3`): addressed response containing the compact base manifest body below.
- `GET_CHUNKS` (`4`): 16-byte publication ID, uint16 window base, and an 8-bit wanted bitmap. At most four bits may be set.
- `CHUNK` (`5`): uint16 index followed by up to 178 content bytes. The active addressed transaction supplies publication identity.
- `ERROR` (`6`) is reserved for bounded diagnostics.
- `GET_SIGNATURE` (`10`): addressed request containing a 16-byte publication ID.
- `SIGNATURE` (`11`): one status byte; signed responses add a 32-byte Ed25519 public key and 64-byte signature.

`MANIFEST` body:

| Offset | Size | Field |
|---:|---:|---|
| 0 | 8 | content length, uint64 little-endian |
| 8 | 32 | full content SHA-256 |
| 40 | 2 | chunk size, uint16 little-endian |
| 42 | 2 | chunk count, uint16 little-endian |
| 44 | 1 | filename byte length |
| 45 | variable | UTF-8 filename bytes |

Publication identity is supplied by the active request. The receiver requires that the first 16 bytes of the full SHA-256 equal the requested publication ID.

`SIGNATURE` body:

| Offset | Size | Field |
|---:|---:|---|
| 0 | 1 | `0` unsigned, `1` Ed25519 signed |
| 1 | 32 | raw Ed25519 public key when signed |
| 33 | 64 | Ed25519 signature when signed |

Base manifest and signature inspection are requested from the same source. After validation and persistence, chunk acquisition may rotate across any source advertising the publication.

The requester chooses one advertising source for a transaction. Persistent partial state is keyed only by publication ID and immutable manifest, so later batches may use another source. A source sends only the requested batch, at most four chunks, then both nodes return to listening. Missing chunks are requested again after bounded exponential backoff. Full SHA-256 is authoritative before commit.

Discovery follows `DISCOVER -> INSPECT -> DECIDE -> TRANSFER -> VERIFY -> REPLICATE`. A concrete unknown ID triggers a base manifest request followed by signature metadata from the same source. The combined manifest is stored under `known/<id>/manifest.json` before local admission policy runs. With no size limit, behavior remains acquire-all. With `--max-auto-size`, objects at or below the inclusive limit transfer automatically; larger objects remain known without chunk traffic. `ghost-node want` creates a persistent local override. Existing partial objects remain resumable subject to current signature policy or an explicit want. Policy is never sent over RF, and every complete locally-added object is always included in reconciliation.

## Cooperative Scheduling

Nodes listen by default. Addressed requests are served immediately with bounded replies. New inventory and acquisition requests pass through one traffic controller.

The controller uses decoded packet activity, traffic addressed between other nodes, expected replies, response timeouts, and TX failures. It does not use RSSI/SNR as carrier sense because the current modem reports those only for successfully decoded packets. RSSI/SNR remain diagnostics.

Recent activity decays over roughly twenty seconds. Rising pressure:

- lengthens the required quiet interval before initiating traffic;
- reduces acquisition batches from four chunks toward one;
- adds randomized deferral after failures;
- combines with exponential per-object retry backoff.

Root-summary deadlines take precedence over continuous acquisition work, and only one root is sent per interval. Reconciliation alternates with acquisition, and acquisition work rotates across objects and sources. This prevents one large object from monopolizing the radio while avoiding any promise of perfect fairness without carrier detection or synchronized clocks.

At startup each node actively listens and serves requests for a random configurable interval before initiating its own traffic. Advertisement intervals use independent +/-20 percent jitter. Transaction numbers start at a randomized value on every process start to reduce correlation with delayed packets from an earlier process.
