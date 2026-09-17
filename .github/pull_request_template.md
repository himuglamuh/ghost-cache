## Summary

Describe the problem and the smallest implemented solution.

## Verification

- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] Firmware builds when firmware changed
- [ ] Wire-format changes include versioning and malformed-input tests
- [ ] Hardware verification is described separately from software tests
- [ ] Documentation reflects operator-visible behavior

## RF and Compatibility

Describe airtime, half-duplex scheduling, storage migration, and protocol compatibility impact. Write `None` when not applicable.

## Security

Describe effects on integrity, signing keys, trust policy, filesystem permissions, or privilege boundaries. Write `None` when not applicable.
