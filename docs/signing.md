# Publication Signing and Local Trust

Signing is optional. SHA-256 remains content integrity; Ed25519 provides manifest authenticity; the trust store is local admission policy. Relays preserve the origin signature and never re-sign acquired publications.

Create an identity:

```bash
ghost-node keygen --data-dir /var/lib/ghostcache --name "Example Publisher"
```

This creates `identity/signing.key` (base64 raw 64-byte Ed25519 private key, mode `0600`), `identity/signing.pub` (base64 raw 32-byte public key), and `identity/identity.json`. Key ID is the lowercase hex first 16 bytes of `SHA-256(raw_public_key)`. Existing private keys are never overwritten.

The friendly name is local metadata and is not signed or sent over RF. Copy `signing.pub` to operators who should trust this publisher. Never distribute `signing.key`.

`ghost-node add` signs by default when an identity exists. Use `--unsigned` for a specific publication.

Relay acceptance example:

```bash
./ghost-node keygen --data-dir ./data/node-a --name "Ghost Cache Demo Publisher"
./ghost-node trust --data-dir ./data/node-c ./data/node-a/identity/signing.pub
./ghost-node add --data-dir ./data/node-a ./demo-content.md
./ghost-node inspect --data-dir ./data/node-a "publication-id-prefix"
```

Run A and B, wait for B to commit, stop A, then run C with `signature_policy = "trusted"`. C must acquire from B while validating A's preserved signature. Compare all completed content with `sha256sum` and `cmp` as in the node relay acceptance procedure.

For an isolated negative test, copy a signed object's manifest and content into a temporary test data directory, alter signed metadata or signature bytes, and confirm `ghost-node library` reports invalid or acquisition refuses commit. Do not alter a verified production store.

## Canonical Signed Bytes

The exact signed byte string is concatenation in this order. Integers are little-endian and strings are raw UTF-8 bytes without terminators:

1. ASCII `GHOSTCACHE-MANIFEST` followed by `00`.
2. uint16 manifest version, currently `2`.
3. 16-byte publication ID.
4. 32-byte full content SHA-256.
5. uint64 content length.
6. uint16 chunk size.
7. uint16 chunk count.
8. uint16 filename byte length.
9. filename bytes.

Disk JSON includes algorithm, key ID, base64 public key, and base64 64-byte signature. RF uses a separate addressed signature response: one status byte, 32-byte public key, and 64-byte signature. Including the 14-byte GN envelope, signed metadata costs 111 bytes; unsigned status costs 15 bytes. Chunk size remains 178 bytes.

The public key is included in each signed manifest so any relay can preserve a self-contained verifiable publication. Trust is established separately by matching its derived key ID and exact public-key bytes against the local trust store.

## Trust

```bash
ghost-node trust --data-dir ./data/node-c ./data/node-a/identity/signing.pub
ghost-node trust list --data-dir ./data/node-c
ghost-node trust remove --data-dir ./data/node-c "key-id"
```

Policies:

- `permissive` (default): unsigned accepted; present signatures must verify.
- `signed`: a valid signature is required, but the key need not be trusted.
- `trusted`: a valid signature from the local trust store is required.

Persistent `want` overrides signed/trusted admission and size policy, including for unsigned/untrusted content. It never overrides SHA-256 or a cryptographically invalid signature.

Inspect the local catalog:

```bash
ghost-node library --data-dir ./data/node-c
ghost-node inspect --data-dir ./data/node-c "id-or-unique-prefix"
ghost-node want --data-dir ./data/node-c "id-or-unique-prefix"
```
