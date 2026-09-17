package publication

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const SignedManifestVersion uint16 = 2

type Signature struct {
	PublicKey [ed25519.PublicKeySize]byte
	Value     [ed25519.SignatureSize]byte
}
type Identity struct {
	Name       string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}
type identityJSON struct {
	Name      string `json:"name,omitempty"`
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
}

func KeyID(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:16])
}

func GenerateIdentity(root, name string) (Identity, error) {
	dir := filepath.Join(root, "identity")
	keyPath := filepath.Join(dir, "signing.key")
	if _, err := os.Stat(keyPath); err == nil {
		return Identity{}, errors.New("signing identity already exists")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Identity{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	if err := writeAtomic(keyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)+"\n"), 0600); err != nil {
		return Identity{}, err
	}
	if err := writeAtomic(filepath.Join(dir, "signing.pub"), []byte(base64.StdEncoding.EncodeToString(publicKey)+"\n"), 0644); err != nil {
		return Identity{}, err
	}
	metadata, _ := json.MarshalIndent(identityJSON{Name: name, Algorithm: "ed25519", KeyID: KeyID(publicKey), PublicKey: base64.StdEncoding.EncodeToString(publicKey)}, "", "  ")
	if err := writeAtomic(filepath.Join(dir, "identity.json"), metadata, 0644); err != nil {
		return Identity{}, err
	}
	return Identity{Name: name, PublicKey: publicKey, PrivateKey: privateKey}, nil
}

func LoadIdentity(root string) (Identity, error) {
	dir := filepath.Join(root, "identity")
	keyPath := filepath.Join(dir, "signing.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		return Identity{}, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return Identity{}, errors.New("signing private key permissions must be 0600")
	}
	privateText, err := os.ReadFile(keyPath)
	if err != nil {
		return Identity{}, err
	}
	privateKey, err := base64.StdEncoding.DecodeString(string(bytesTrimSpace(privateText)))
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return Identity{}, errors.New("invalid signing private key")
	}
	metadataBytes, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		return Identity{}, err
	}
	var metadata identityJSON
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return Identity{}, err
	}
	publicKey := ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey)
	if metadata.PublicKey != base64.StdEncoding.EncodeToString(publicKey) || metadata.KeyID != KeyID(publicKey) {
		return Identity{}, errors.New("identity metadata does not match private key")
	}
	return Identity{Name: metadata.Name, PublicKey: publicKey, PrivateKey: ed25519.PrivateKey(privateKey)}, nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(string(bytesTrimSpace(b)))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key")
	}
	return ed25519.PublicKey(decoded), nil
}

func CanonicalManifest(m Manifest) ([]byte, error) {
	if len([]byte(m.Filename)) > MaxFilenameBytes {
		return nil, ErrMalformed
	}
	if m.Version != SignedManifestVersion {
		return nil, errors.New("signed manifest version must be 2")
	}
	domain := []byte("GHOSTCACHE-MANIFEST\x00")
	b := make([]byte, 0, len(domain)+2+16+32+8+2+2+2+len(m.Filename))
	b = append(b, domain...)
	v := make([]byte, 2)
	binary.LittleEndian.PutUint16(v, m.Version)
	b = append(b, v...)
	b = append(b, m.ID[:]...)
	b = append(b, m.SHA256[:]...)
	n := make([]byte, 8)
	binary.LittleEndian.PutUint64(n, m.Length)
	b = append(b, n...)
	n = make([]byte, 2)
	binary.LittleEndian.PutUint16(n, m.ChunkSize)
	b = append(b, n...)
	binary.LittleEndian.PutUint16(n, m.ChunkCount)
	b = append(b, n...)
	binary.LittleEndian.PutUint16(n, uint16(len([]byte(m.Filename))))
	b = append(b, n...)
	b = append(b, []byte(m.Filename)...)
	return b, nil
}

func SignManifest(m *Manifest, identity Identity) error {
	m.Version = SignedManifestVersion
	canonical, err := CanonicalManifest(*m)
	if err != nil {
		return err
	}
	if len(identity.PrivateKey) != ed25519.PrivateKeySize {
		return errors.New("invalid private key")
	}
	sig := Signature{}
	copy(sig.PublicKey[:], identity.PublicKey)
	copy(sig.Value[:], ed25519.Sign(identity.PrivateKey, canonical))
	m.Signature = &sig
	return nil
}
func VerifyManifestSignature(m Manifest) error {
	if m.Signature == nil {
		return nil
	}
	canonical, err := CanonicalManifest(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(m.Signature.PublicKey[:], canonical, m.Signature.Value[:]) {
		return errors.New("invalid Ed25519 manifest signature")
	}
	return nil
}

type TrustStore struct{ root string }

func NewTrustStore(root string) *TrustStore { return &TrustStore{root: root} }
func (t *TrustStore) Add(path string) (string, error) {
	key, err := LoadPublicKey(path)
	if err != nil {
		return "", err
	}
	id := KeyID(key)
	dir := filepath.Join(t.root, "trust")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return id, writeAtomic(filepath.Join(dir, id+".pub"), []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0644)
}
func (t *TrustStore) Remove(id string) error {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		return errors.New("invalid key ID")
	}
	return os.Remove(filepath.Join(t.root, "trust", id+".pub"))
}
func (t *TrustStore) Trusted(key []byte) bool {
	loaded, err := LoadPublicKey(filepath.Join(t.root, "trust", KeyID(key)+".pub"))
	return err == nil && string(loaded) == string(key)
}
func (t *TrustStore) IDs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(t.root, "trust"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".pub" {
			out = append(out, entry.Name()[:len(entry.Name())-4])
		}
	}
	return out, nil
}
func bytesTrimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\n' || b[start] == '\r' || b[start] == '\t') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}
func signatureJSON(sig *Signature) any {
	if sig == nil {
		return nil
	}
	return map[string]string{"algorithm": "ed25519", "key_id": KeyID(sig.PublicKey[:]), "public_key": base64.StdEncoding.EncodeToString(sig.PublicKey[:]), "signature": base64.StdEncoding.EncodeToString(sig.Value[:])}
}
func parseSignature(raw json.RawMessage) (*Signature, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value map[string]string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value["algorithm"] != "ed25519" {
		return nil, fmt.Errorf("unsupported signature algorithm")
	}
	pub, err := base64.StdEncoding.DecodeString(value["public_key"])
	if err != nil || len(pub) != 32 {
		return nil, errors.New("invalid signature public key")
	}
	signature, err := base64.StdEncoding.DecodeString(value["signature"])
	if err != nil || len(signature) != 64 {
		return nil, errors.New("invalid signature")
	}
	if value["key_id"] != KeyID(pub) {
		return nil, errors.New("signature key ID mismatch")
	}
	out := &Signature{}
	copy(out.PublicKey[:], pub)
	copy(out.Value[:], signature)
	return out, nil
}
