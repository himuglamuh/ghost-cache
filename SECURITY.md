# Security Policy

## Reporting a Vulnerability

Do not open a public issue for vulnerabilities involving signature validation, key handling, path traversal, unsafe persistence, or remotely triggered crashes.

Use [GitHub private vulnerability reporting](https://github.com/himuglamuh/ghost-cache/security/advisories/new). Do not include vulnerability or exploit details in a public issue.

Include affected versions, reproduction conditions, impact, and any proposed mitigation. Reports will be acknowledged and triaged as availability permits.

## Scope

Security-sensitive components include:

- serial and RF packet decoders;
- publication integrity and Ed25519 verification;
- signing identity and trust-store handling;
- filesystem path and permission handling;
- service installation and privilege boundaries.

Ghost Cache does not currently promise confidentiality, anonymity, resistance to RF jamming, authenticated node identities, or protection from a compromised host. See [Security Model](docs/security.md).
