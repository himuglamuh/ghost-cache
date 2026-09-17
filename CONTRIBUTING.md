# Contributing to Ghost Cache

Contributions should preserve the project's core boundaries: the modem is an opaque transport adapter, content correctness is publication-based, acquisition policy is local, and scarce RF airtime is treated as a primary resource.

## Development Setup

```bash
go mod download
go test -race ./...
go vet ./...
pio run --project-dir firmware/heltec-modem
```

Build all host applications with `./scripts/build.sh`.

## Pull Requests

- Keep changes focused and include tests for protocol, persistence, or policy behavior.
- Document wire-format changes exactly and increment the relevant protocol version when compatibility changes.
- Do not add application semantics to modem firmware.
- Keep operational logs on stdout/stderr and avoid persistent logging by default.
- Never include private keys, generated data stores, binaries, PlatformIO build output, or device-specific secrets.
- Describe hardware tests separately from software tests.

Protocol changes should include malformed-input, packet-size boundary, and compatibility tests. Firmware changes should compile under PlatformIO and be verified on target hardware before release claims are updated.

By contributing, you agree that your contribution is licensed under Apache License 2.0.
