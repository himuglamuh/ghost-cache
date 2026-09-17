package publication

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func signedTestManifest(t *testing.T) (Manifest, Identity) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifest([]byte("signed content"))
	identity := Identity{Name: "publisher", PublicKey: pub, PrivateKey: priv}
	if err := SignManifest(&m, identity); err != nil {
		t.Fatal(err)
	}
	return m, identity
}

func TestSignVerifyAndMutation(t *testing.T) {
	m, _ := signedTestManifest(t)
	if err := VerifyManifestSignature(m); err != nil {
		t.Fatal(err)
	}
	changed := m
	changed.Filename = "changed"
	if VerifyManifestSignature(changed) == nil {
		t.Fatal("metadata mutation verified")
	}
	changed = m
	changed.SHA256[31] ^= 1
	if VerifyManifestSignature(changed) == nil {
		t.Fatal("content hash mutation verified")
	}
}

func TestCanonicalManifestStable(t *testing.T) {
	m, _ := signedTestManifest(t)
	a, err := CanonicalManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("canonical bytes differ")
	}
}

func TestIdentityGenerationAndPermissions(t *testing.T) {
	root := t.TempDir()
	identity, err := GenerateIdentity(root, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(identity.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatal("bad key")
	}
	if _, err := GenerateIdentity(root, "Replace"); err == nil {
		t.Fatal("duplicate keygen overwrote identity")
	}
	info, err := os.Stat(filepath.Join(root, "identity", "signing.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private mode=%o", info.Mode().Perm())
	}
	loaded, err := LoadIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if KeyID(loaded.PublicKey) != KeyID(identity.PublicKey) {
		t.Fatal("loaded identity differs")
	}
}

func TestTrustStore(t *testing.T) {
	publisher := t.TempDir()
	identity, err := GenerateIdentity(publisher, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	trust := NewTrustStore(root)
	id, err := trust.Add(filepath.Join(publisher, "identity", "signing.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if id != KeyID(identity.PublicKey) || !trust.Trusted(identity.PublicKey) {
		t.Fatal("trusted key not found")
	}
	ids, err := trust.IDs()
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if err := trust.Remove(id); err != nil {
		t.Fatal(err)
	}
	if trust.Trusted(identity.PublicKey) {
		t.Fatal("removed key remains trusted")
	}
	if err := trust.Remove("../../../../tmp/not-allowed-value"); err == nil {
		t.Fatal("path traversal key ID accepted")
	}
}

func TestLoadIdentityRejectsLoosePermissions(t *testing.T) {
	root := t.TempDir()
	if _, err := GenerateIdentity(root, "Demo"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "identity", "signing.key")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIdentity(root); err == nil {
		t.Fatal("loaded world-readable private key")
	}
}

func TestSignedStoreRoundTripAndRelay(t *testing.T) {
	root := t.TempDir()
	identity, err := GenerateIdentity(root, "Origin")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "signed.bin")
	if err := os.WriteFile(input, []byte("signed content"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := NewStore(root).AddFileSigned(input, &identity)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(root).LoadManifest(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Signature == nil || VerifyManifestSignature(loaded) != nil {
		t.Fatal("signature not preserved")
	}
	if *loaded.Signature != *manifest.Signature {
		t.Fatal("relay metadata changed")
	}
}

func TestCatalogShowsStatesAndSignatures(t *testing.T) {
	root := t.TempDir()
	identity, err := GenerateIdentity(root, "Origin")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "catalog.bin")
	os.WriteFile(input, []byte("catalog"), 0600)
	manifest, _, err := NewStore(root).AddFileSigned(input, &identity)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := NewStore(root).Catalog()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	if entries[0].State != "complete" || entries[0].Manifest.Filename != "catalog.bin" || entries[0].Manifest.Signature == nil || entries[0].Manifest.ID != manifest.ID {
		t.Fatalf("entry=%+v", entries[0])
	}
	resolved, err := NewStore(root).Resolve(manifest.ID.String()[:8])
	if err != nil || resolved != manifest.ID {
		t.Fatalf("resolved=%s err=%v", resolved, err)
	}
}

func TestKnownMetadataMayUpgradeToSigned(t *testing.T) {
	root := t.TempDir()
	unsigned := testManifest([]byte("upgrade"))
	store := NewStore(root)
	if err := store.SaveKnown(unsigned); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed := unsigned
	if err := SignManifest(&signed, Identity{PublicKey: pub, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveKnown(signed); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadKnown(signed.ID)
	if err != nil || got.Signature == nil || VerifyManifestSignature(got) != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
