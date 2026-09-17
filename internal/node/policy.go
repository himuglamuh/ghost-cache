package node

import (
	"fmt"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

type SignaturePolicy uint8

const (
	SignaturePermissive SignaturePolicy = iota
	SignatureRequired
	SignatureTrusted
)

func ParseSignaturePolicy(value string) (SignaturePolicy, error) {
	switch value {
	case "", "permissive":
		return SignaturePermissive, nil
	case "signed":
		return SignatureRequired, nil
	case "trusted":
		return SignatureTrusted, nil
	default:
		return 0, fmt.Errorf("invalid signature policy %q", value)
	}
}

type AcquisitionPolicy struct {
	MaxAutoSize uint64
	HasMaxSize  bool
	Signature   SignaturePolicy
	Trust       *publication.TrustStore
}

func (p AcquisitionPolicy) Accept(manifest publication.Manifest, partial, wanted bool) bool {
	if manifest.Signature != nil && publication.VerifyManifestSignature(manifest) != nil {
		return false
	}
	if wanted {
		return true
	}
	if !partial && p.HasMaxSize && manifest.Length > p.MaxAutoSize {
		return false
	}
	switch p.Signature {
	case SignatureRequired:
		return manifest.Signature != nil
	case SignatureTrusted:
		return manifest.Signature != nil && p.Trust != nil && p.Trust.Trusted(manifest.Signature.PublicKey[:])
	default:
		return true
	}
}
