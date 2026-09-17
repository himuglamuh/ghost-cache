package node

import (
	"testing"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

func TestAcquisitionPolicy(t *testing.T) {
	cases := []struct {
		name                  string
		policy                AcquisitionPolicy
		size                  uint64
		partial, wanted, want bool
	}{{"unlimited", AcquisitionPolicy{}, 1 << 40, false, false, true}, {"below", AcquisitionPolicy{MaxAutoSize: 100, HasMaxSize: true}, 99, false, false, true}, {"exact", AcquisitionPolicy{MaxAutoSize: 100, HasMaxSize: true}, 100, false, false, true}, {"above", AcquisitionPolicy{MaxAutoSize: 100, HasMaxSize: true}, 101, false, false, false}, {"partial resumes", AcquisitionPolicy{MaxAutoSize: 100, HasMaxSize: true}, 1000, true, false, true}, {"manual override", AcquisitionPolicy{MaxAutoSize: 100, HasMaxSize: true}, 1000, false, true, true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.Accept(publication.Manifest{Length: tc.size}, tc.partial, tc.wanted)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSignaturePoliciesAndWantOverride(t *testing.T) {
	m, identity := signedTestPublication(t)
	if !(AcquisitionPolicy{Signature: SignatureRequired}).Accept(m, false, false) {
		t.Fatal("signed policy rejected valid signature")
	}
	trust := publication.NewTrustStore(t.TempDir())
	pubPath := writePublicKey(t, identity.PublicKey)
	if _, err := trust.Add(pubPath); err != nil {
		t.Fatal(err)
	}
	if !(AcquisitionPolicy{Signature: SignatureTrusted, Trust: trust}).Accept(m, false, false) {
		t.Fatal("trusted signature rejected")
	}
	if (AcquisitionPolicy{Signature: SignatureTrusted, Trust: publication.NewTrustStore(t.TempDir())}).Accept(m, false, false) {
		t.Fatal("untrusted signature accepted")
	}
	unsigned := m
	unsigned.Signature = nil
	unsigned.Version = 1
	for _, policy := range []SignaturePolicy{SignatureRequired, SignatureTrusted} {
		p := AcquisitionPolicy{Signature: policy, Trust: trust}
		if p.Accept(unsigned, false, false) {
			t.Fatal("unsigned accepted")
		}
		if !p.Accept(unsigned, false, true) {
			t.Fatal("want did not override policy")
		}
	}
	bad := m
	bad.Filename = "tampered"
	if (AcquisitionPolicy{}).Accept(bad, false, true) {
		t.Fatal("want bypassed invalid signature")
	}
}

func TestPartialDoesNotBypassSignaturePolicy(t *testing.T) {
	m, _ := signedTestPublication(t)
	m.Signature = nil
	m.Version = 1
	if (AcquisitionPolicy{Signature: SignatureTrusted, Trust: publication.NewTrustStore(t.TempDir())}).Accept(m, true, false) {
		t.Fatal("unsigned partial bypassed trusted policy")
	}
	if !(AcquisitionPolicy{Signature: SignatureTrusted}).Accept(m, true, true) {
		t.Fatal("want did not override partial trust policy")
	}
}
