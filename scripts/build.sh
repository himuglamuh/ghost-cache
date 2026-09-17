#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pushd "$root" >/dev/null
go test ./...
go build -o ./ghost-radio ./cmd/ghost-radio
go build -o ./ghost-publish ./cmd/ghost-publish
go build -o ./ghost-broadcast ./cmd/ghost-broadcast
go build -o ./ghost-node ./cmd/ghost-node
pio run --project-dir firmware/heltec-modem
popd >/dev/null
