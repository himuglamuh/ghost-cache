package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type Store struct{ root string }

type manifestJSON struct {
	Version    uint16 `json:"version"`
	ID         string `json:"publication_id"`
	Filename   string `json:"filename"`
	Length     uint64 `json:"content_length"`
	SHA256     string `json:"sha256"`
	ChunkSize  uint16 `json:"chunk_size"`
	ChunkCount uint16 `json:"chunk_count"`
	Signature  any    `json:"signature,omitempty"`
}

func NewStore(root string) *Store        { return &Store{root: root} }
func (s *Store) objectDir(id ID) string  { return filepath.Join(s.root, "objects", id.String()) }
func (s *Store) partialDir(id ID) string { return filepath.Join(s.root, "partial", id.String()) }
func (s *Store) knownDir(id ID) string   { return filepath.Join(s.root, "known", id.String()) }
func (s *Store) Complete(id ID) bool {
	info, err := os.Stat(filepath.Join(s.objectDir(id), "complete"))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	manifest, err := s.LoadManifest(id)
	if err != nil {
		return false
	}
	if err := VerifyManifestSignature(manifest); err != nil {
		return false
	}
	content, err := os.ReadFile(filepath.Join(s.objectDir(id), "content"))
	return err == nil && uint64(len(content)) == manifest.Length && sha256.Sum256(content) == manifest.SHA256
}

func (s *Store) AddFile(path string) (Manifest, string, error) {
	return s.AddFileSigned(path, nil)
}

func (s *Store) AddFileSigned(path string, identity *Identity) (Manifest, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, "", err
	}
	if len(content) > MaxChunks*ChunkDataSize {
		return Manifest{}, "", fmt.Errorf("file exceeds maximum size of %d bytes", MaxChunks*ChunkDataSize)
	}
	hash := sha256.Sum256(content)
	var id ID
	copy(id[:], hash[:len(id)])
	count := 0
	if len(content) > 0 {
		count = (len(content) + ChunkDataSize - 1) / ChunkDataSize
	}
	manifest := Manifest{Version: 1, ID: id, Filename: filepath.Base(path), Length: uint64(len(content)), SHA256: hash, ChunkSize: ChunkDataSize, ChunkCount: uint16(count)}
	if identity != nil {
		if err := SignManifest(&manifest, *identity); err != nil {
			return Manifest{}, "", err
		}
	}
	if _, err := EncodeManifest(manifest); err != nil {
		return Manifest{}, "", fmt.Errorf("file metadata cannot be advertised: %w", err)
	}
	if s.Complete(id) {
		existing, err := s.LoadManifest(id)
		if err != nil {
			return Manifest{}, "", err
		}
		if existing.SHA256 != manifest.SHA256 || existing.Length != manifest.Length || existing.Filename != manifest.Filename {
			return Manifest{}, "", errors.New("publication ID conflicts with existing object")
		}
		return existing, filepath.Join(s.objectDir(id), "content"), nil
	}
	assembly, err := s.Restart(manifest)
	if err != nil {
		return Manifest{}, "", err
	}
	for i, start := 0, 0; start < len(content); i, start = i+1, start+ChunkDataSize {
		end := min(start+ChunkDataSize, len(content))
		if err := s.Add(assembly, uint16(i), content[start:end]); err != nil {
			return Manifest{}, "", err
		}
	}
	objectPath, err := s.Commit(assembly)
	return manifest, objectPath, err
}

func (s *Store) PartialIDs() ([]ID, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "partial"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []ID
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := ParseID(entry.Name())
		if err == nil && s.HasPartial(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return string(ids[i][:]) < string(ids[j][:]) })
	return ids, nil
}

func (s *Store) IDs() ([]ID, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "objects"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]ID, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := ParseID(entry.Name())
		if err != nil || !s.Complete(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return string(ids[i][:]) < string(ids[j][:]) })
	return ids, nil
}

func ParseID(value string) (ID, error) {
	var id ID
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != len(id) {
		return id, ErrMalformed
	}
	copy(id[:], b)
	return id, nil
}

func (s *Store) LoadManifest(id ID) (Manifest, error) {
	return readManifest(filepath.Join(s.objectDir(id), "manifest.json"))
}

func (s *Store) SaveKnown(manifest Manifest) error {
	if _, err := EncodeManifest(manifest); err != nil {
		return err
	}
	dir := s.knownDir(manifest.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(toJSON(manifest), "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "manifest.json")
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == string(encoded) {
			return nil
		}
		// Known metadata is discovery state, not verified content. A different
		// relay may provide a valid signed form after an unsigned/untrusted peer.
		return writeAtomic(path, encoded, 0600)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAtomic(path, encoded, 0600)
}

func (s *Store) LoadKnown(id ID) (Manifest, error) {
	return readManifest(filepath.Join(s.knownDir(id), "manifest.json"))
}
func (s *Store) HasKnown(id ID) bool {
	manifest, err := s.LoadKnown(id)
	return err == nil && manifest.ID == id
}

func (s *Store) KnownIDs() ([]ID, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "known"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []ID
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := ParseID(entry.Name())
		if err == nil && s.HasKnown(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return string(ids[i][:]) < string(ids[j][:]) })
	return ids, nil
}

func (s *Store) Want(id ID) error {
	if !s.HasKnown(id) && !s.HasPartial(id) {
		return errors.New("publication manifest is not known")
	}
	dir := s.knownDir(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "want"), []byte("wanted\n"), 0600)
}

func (s *Store) Wanted(id ID) bool {
	_, err := os.Stat(filepath.Join(s.knownDir(id), "want"))
	return err == nil
}

func (s *Store) WantedIDs() ([]ID, error) {
	ids, err := s.KnownIDs()
	if err != nil {
		return nil, err
	}
	out := ids[:0]
	for _, id := range ids {
		if s.Wanted(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Store) LoadPartial(id ID) (*Assembly, error) {
	manifest, err := readManifest(filepath.Join(s.partialDir(id), "manifest.json"))
	if err != nil {
		return nil, err
	}
	return s.Begin(manifest)
}

func (s *Store) HasPartial(id ID) bool {
	_, err := os.Stat(filepath.Join(s.partialDir(id), "manifest.json"))
	return err == nil
}

func (s *Store) ReadChunk(id ID, index uint16) ([]byte, error) {
	manifest, err := s.LoadManifest(id)
	if err != nil {
		return nil, err
	}
	if index >= manifest.ChunkCount {
		return nil, ErrMalformed
	}
	content, err := os.Open(filepath.Join(s.objectDir(id), "content"))
	if err != nil {
		return nil, err
	}
	defer content.Close()
	length := int(manifest.ChunkSize)
	if index == manifest.ChunkCount-1 {
		length = int(manifest.Length) - int(index)*int(manifest.ChunkSize)
	}
	b := make([]byte, length)
	_, err = content.ReadAt(b, int64(index)*int64(manifest.ChunkSize))
	return b, err
}

func (s *Store) Begin(manifest Manifest) (*Assembly, error) {
	if filepath.Base(manifest.Filename) != manifest.Filename || manifest.Filename == "." || manifest.Filename == "" {
		return nil, fmt.Errorf("unsafe filename: %w", ErrMalformed)
	}
	dir := s.partialDir(manifest.ID)
	if err := os.MkdirAll(filepath.Join(dir, "chunks"), 0700); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(toJSON(manifest), "", "  ")
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if existing, err := os.ReadFile(manifestPath); err == nil {
		if string(existing) != string(encoded) {
			return nil, errors.New("publication ID conflicts with existing partial manifest")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if err := writeAtomic(manifestPath, encoded, 0600); err != nil {
		return nil, err
	}
	assembly := NewAssembly(manifest)
	for i := uint16(0); i < manifest.ChunkCount; i++ {
		data, err := os.ReadFile(filepath.Join(dir, "chunks", strconv.Itoa(int(i))))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := assembly.Add(i, data); err != nil {
			return nil, fmt.Errorf("invalid persisted chunk %d: %w", i, err)
		}
	}
	return assembly, nil
}

func (s *Store) Add(assembly *Assembly, index uint16, data []byte) error {
	if int(index) < len(assembly.received) && assembly.received[index] {
		return assembly.Add(index, data)
	}
	if err := assembly.Add(index, data); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(s.partialDir(assembly.Manifest.ID), "chunks", strconv.Itoa(int(index))), data, 0600); err != nil {
		assembly.chunks[index] = nil
		assembly.received[index] = false
		return err
	}
	return nil
}

func (s *Store) Commit(assembly *Assembly) (string, error) {
	content, err := assembly.Content()
	if err != nil {
		return "", err
	}
	if err := VerifyManifestSignature(assembly.Manifest); err != nil {
		return "", err
	}
	dir := s.objectDir(assembly.Manifest.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	encoded, err := json.MarshalIndent(toJSON(assembly.Manifest), "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeAtomic(filepath.Join(dir, "content"), content, 0600); err != nil {
		return "", err
	}
	if err := writeAtomic(filepath.Join(dir, "manifest.json"), encoded, 0600); err != nil {
		return "", err
	}
	if err := writeAtomic(filepath.Join(dir, "complete"), []byte("verified\n"), 0600); err != nil {
		return "", err
	}
	if err := os.RemoveAll(s.partialDir(assembly.Manifest.ID)); err != nil {
		return "", err
	}
	return filepath.Join(dir, "content"), nil
}

func (s *Store) Restart(manifest Manifest) (*Assembly, error) {
	if err := os.RemoveAll(s.partialDir(manifest.ID)); err != nil {
		return nil, err
	}
	return s.Begin(manifest)
}

func (s *Store) DiscardPartial(id ID) error { return os.RemoveAll(s.partialDir(id)) }

func toJSON(m Manifest) manifestJSON {
	version := m.Version
	if version == 0 {
		version = 1
	}
	return manifestJSON{Version: version, ID: m.ID.String(), Filename: m.Filename, Length: m.Length, SHA256: fmt.Sprintf("%x", m.SHA256), ChunkSize: m.ChunkSize, ChunkCount: m.ChunkCount, Signature: signatureJSON(m.Signature)}
}

func readManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var value manifestJSON
	if err := json.Unmarshal(b, &value); err != nil {
		return Manifest{}, err
	}
	if value.Version != 1 && value.Version != SignedManifestVersion {
		return Manifest{}, ErrMalformed
	}
	id, err := ParseID(value.ID)
	if err != nil {
		return Manifest{}, err
	}
	hashBytes, err := hex.DecodeString(value.SHA256)
	if err != nil || len(hashBytes) != sha256.Size {
		return Manifest{}, ErrMalformed
	}
	var hash [sha256.Size]byte
	copy(hash[:], hashBytes)
	encodedJSON, _ := json.Marshal(value.Signature)
	signature, err := parseSignature(encodedJSON)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Version: value.Version, ID: id, Filename: value.Filename, Length: value.Length, SHA256: hash, ChunkSize: value.ChunkSize, ChunkCount: value.ChunkCount, Signature: signature}
	packet, err := EncodeManifest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	decoded, _ := DecodePacket(packet)
	validated, err := DecodeManifest(decoded)
	if err != nil {
		return Manifest{}, err
	}
	validated.Version = value.Version
	validated.Signature = signature
	return validated, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
