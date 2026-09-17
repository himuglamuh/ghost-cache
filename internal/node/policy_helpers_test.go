package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

func signedTestPublication(t *testing.T) (publication.Manifest, publication.Identity) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("x"))
	var id publication.ID
	copy(id[:], hash[:16])
	m := publication.Manifest{ID: id, Filename: "x", Length: 1, SHA256: hash, ChunkSize: publication.ChunkDataSize, ChunkCount: 1}
	identity := publication.Identity{PublicKey: pub, PrivateKey: priv}
	if err := publication.SignManifest(&m, identity); err != nil {
		t.Fatal(err)
	}
	return m, identity
}
func writePublicKey(t *testing.T, key ed25519.PublicKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "publisher.pub")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
