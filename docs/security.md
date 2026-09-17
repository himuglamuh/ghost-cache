# Security Model

> **Ghost Cache traffic is plaintext.** Anyone capable of receiving the configured LoRa channel may observe publication metadata and content. Signatures prove that metadata was signed by a key; they do not hide content, identify radio users, or provide anonymity.

Ghost Cache separates three properties:

- **Integrity:** full SHA-256 proves that reconstructed bytes match the manifest.
- **Authenticity:** an optional Ed25519 signature binds immutable manifest metadata to a signing key.
- **Trust:** each receiver independently decides which signing keys it accepts.

## What Ghost Cache Protects

- Corrupted or incomplete content is not committed.
- A signature cannot be moved to different content, filename, size, or chunk geometry.
- Relays preserve the original signature and do not need the private key.
- Invalid present signatures are rejected under every policy.
- Private signing keys remain local and are never transmitted.

## What Ghost Cache Does Not Protect

- RF traffic and stored content are not encrypted.
- Publication IDs, metadata, and content may be observed by anyone receiving the channel.
- There is no transport encryption, metadata privacy, or anonymity guarantee.
- Traffic analysis, jamming, replay, denial of service, and malicious inventory claims are not prevented.
- Node IDs are random coordination values, not authenticated identities.
- The trust store has no automatic exchange, revocation protocol, certificate chain, or trust-on-first-use behavior.
- Host compromise can expose content and signing keys.

## Signing Identity

```bash
ghost-node keygen \
  --data-dir /var/lib/ghostcache \
  --name "Example Publisher"
```

The private key is created with mode `0600`; Ghost Cache refuses to use it if group or world permissions are present. Key generation never overwrites an existing identity. Back up `identity/` securely if signed publications must remain reproducible.

## Trust Policies

Configure one local policy:

| Policy | Unsigned | Valid unknown signer | Valid trusted signer | Invalid signature |
|---|---:|---:|---:|---:|
| `permissive` | accept | accept | accept | reject |
| `signed` | defer | accept | accept | reject |
| `trusted` | defer | defer | accept | reject |

`signed` does not mean "signed by someone you trust." It means only that the publication carries a cryptographically valid signature. Use `trusted` when signer identity matters.

Ghost Cache cannot determine whether a public key actually belongs to the person or organization you expect. Obtain trusted keys through an external trusted channel, such as an in-person exchange or an authenticated organizational system. The RF network does not bootstrap publisher identity.

Manage trusted raw Ed25519 public keys:

```bash
ghost-node trust --data-dir /var/lib/ghostcache publisher.pub
ghost-node trust list --data-dir /var/lib/ghostcache
ghost-node trust remove --data-dir /var/lib/ghostcache "key-id"
```

Removing a key changes future automatic admission only. It does not revoke, quarantine, delete, or stop advertising publications already committed under that key. Ghost Cache currently has no network revocation protocol. To withdraw an existing local object, stop the node and follow the manual lifecycle guidance in [Node operations](operations.md); other peers may continue to hold and serve their copies.

Manual `want` is an explicit local admission override for unsigned or untrusted publications. It does not override invalid signatures or content integrity.

For a service installation, run key and trust commands as `ghostcache` while the service is stopped. This prevents conflicting writers and preserves data ownership.

## Canonical Signature Format

The complete interoperable byte format and key-ID derivation are specified in [Publication signing](signing.md).
