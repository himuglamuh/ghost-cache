# Legacy Publication Ingestion Protocol v1

This protocol is retained for the `ghost-publish` and `ghost-broadcast` diagnostic applications. New deployments use the [Ghost Node protocol](node-protocol.md), which provides symmetric peer replication, radix reconciliation, receiver-driven policy, and signed-manifest inspection.

The legacy protocol provides reliable Publisher-to-Broadcaster ingestion over raw Ghost Modem packets. It does not define peer discovery, symmetric replication, routing, encryption, or signatures.

## Packet Envelope

Every application packet is at most 200 bytes:

| Offset | Size | Field |
|---:|---:|---|
| 0 | 2 | magic `GP` |
| 2 | 1 | version (`1`) |
| 3 | 1 | type |
| 4 | 16 | publication ID, first 128 bits of SHA-256 |
| 20 | variable | type-specific body |

The 200-byte limit leaves 55 bytes below the SX1262 maximum for modem and future application evolution. CHUNK uses a two-byte little-endian index, leaving 178 content bytes.

## Messages

- `MANIFEST` (`1`): uint64 content length, 32-byte SHA-256, uint16 chunk size, uint16 chunk count, uint8 filename length, filename.
- `CHUNK` (`2`): uint16 zero-based index followed by binary content.
- `QUERY` (`3`): requests current receipt/completion state.
- `RECEIPT` (`4`): uint16 chunk count followed by a bitmap where one means received.
- `COMPLETE` (`5`): the object has passed SHA-256 verification and its commit marker is durable.
- `ERROR` (`6`): bounded UTF-8 diagnostic text.

## Transfer

The publisher repeatedly announces the manifest until it receives a receipt bitmap. It sends only chunks absent from that bitmap, then sends QUERY. A missing response starts another bounded round: the manifest resumes any persistent partial state and returns a fresh bitmap. Duplicate identical chunks are harmless, chunks may arrive out of order, and conflicting duplicates are rejected.

The broadcaster writes each chunk atomically under partial state. Once all chunks exist it reconstructs and hashes the content. A mismatch clears all chunks and returns an empty receipt bitmap. Successful content and metadata are written under the object directory before a final `complete` marker. COMPLETE is sent in response to QUERY, avoiding simultaneous LoRa transmissions after the last chunk.
