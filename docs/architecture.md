# Architecture

Ghost Cache separates radio transport from content replication.

```text
+---------------- Linux host ----------------+
| ghost-node                                  |
|                                             |
|  discovery -> policy -> transfer -> verify  |
|       |                      |          |    |
|  radix tree            partial store  SHA   |
|       |                      |       Ed25519 |
|       +---------- GN protocol ----------+   |
|                       |                     |
|                 modem client                |
+-----------------------|---------------------+
                        | COBS serial
+-----------------------|---------------------+
| dumb modem firmware                         |
| USB serial <-> raw SX1262 TX/RX + RSSI/SNR  |
+---------------------------------------------+
```

## Content Model

A publication binds filename, byte length, full SHA-256, chunk geometry, and optional Ed25519 origin signature. Its compact ID is the first 128 bits of the content SHA-256. Full SHA-256 remains authoritative for acceptance.

Origin is not part of replication state. After B verifies an object from A, B can serve it to C with the original signature unchanged. Partial chunks are also source-independent, allowing an interrupted acquisition to continue from another peer.

## Discovery and Reconciliation

Consider two small libraries:

```text
Node A has: A B C D E
Node B has: A B C D
```

Ghost Cache does not repeatedly send all five IDs. The nodes compare compact summaries, narrow the mismatch, and eventually let Node B learn: "I am missing E."

Radix reconciliation applies this idea to large libraries. IDs are grouped by successive hexadecimal prefixes, and equal groups require no further RF traffic. Reconciliation cost can therefore depend mostly on what differs rather than on retransmitting every publication ID.

Nodes periodically advertise one compact root summary of their complete IDs. Each of sixteen radix children carries a count and truncated digest. A receiver descends only through differing branches and requests concrete IDs when a branch contains ten or fewer publications.

This matters because full inventory paging grows with every stored object even when two nodes are nearly synchronized. Software tests measured:

| Complete inventory | Difference | Reconciliation cost |
|---:|---:|---:|
| 1,000 objects | 1 object | 5 packets / 484 RF bytes |
| 10,000 objects | 1 object | 7 packets / 686 RF bytes |

A truncated reconciliation digest can delay discovery in the unlikely event of collision; it cannot validate content or bypass full SHA-256.

## Receiver-Driven Acquisition

Discovery does not imply transfer:

```text
DISCOVER -> INSPECT -> DECIDE -> TRANSFER -> VERIFY -> REPLICATE
```

The receiver obtains compact manifest and signature metadata, persists known metadata, then applies local size and trust policy. Deferred content consumes no chunk airtime. A persistent manual want can approve it later.

## Transfer and Recovery

Content is divided into 178-byte chunks inside the 200-byte application packet budget. Requests select at most four missing chunks from an eight-chunk window. Chunks may arrive out of order or more than once. Each accepted chunk is written atomically to publication-keyed partial state.

On restart, the node reconstructs its receipt state from disk. If another advertising source is available, subsequent requests may use it. After all chunks arrive, the node reconstructs content, verifies SHA-256, verifies any signature, commits metadata/content, and publishes the final marker.

## Half-Duplex Scheduling

An SX1262 cannot transmit and receive simultaneously. Ghost Cache listens by default and permits only one self-initiated addressed exchange at a time. Inventory, reconciliation, and acquisition work yield between bounded units. Recent traffic increases quiet time and reduces chunk batch size. Independent jitter and bounded exponential backoff reduce persistent collisions without requiring synchronized clocks.

## Trust Boundary

Node IDs coordinate short-lived RF replies; they are neither identities nor authorization. Ed25519 keys identify signing identities. Trust policy and trusted keys remain local and are never advertised. See [Security](security.md).
