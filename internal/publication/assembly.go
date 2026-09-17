package publication

import (
	"bytes"
	"crypto/sha256"
	"errors"
)

var ErrHashMismatch = errors.New("publication SHA-256 mismatch")

type Assembly struct {
	Manifest Manifest
	chunks   [][]byte
	received []bool
}

func NewAssembly(manifest Manifest) *Assembly {
	return &Assembly{Manifest: manifest, chunks: make([][]byte, manifest.ChunkCount), received: make([]bool, manifest.ChunkCount)}
}

func (a *Assembly) Add(index uint16, data []byte) error {
	if int(index) >= len(a.chunks) {
		return ErrMalformed
	}
	expected := int(a.Manifest.ChunkSize)
	if int(index) == len(a.chunks)-1 {
		expected = int(a.Manifest.Length) - int(index)*int(a.Manifest.ChunkSize)
	}
	if len(data) != expected {
		return ErrMalformed
	}
	if a.received[index] {
		if !bytes.Equal(a.chunks[index], data) {
			return ErrMalformed
		}
		return nil
	}
	a.chunks[index] = append([]byte(nil), data...)
	a.received[index] = true
	return nil
}

func (a *Assembly) Received() []bool { return append([]bool(nil), a.received...) }
func (a *Assembly) Missing() []uint16 {
	var out []uint16
	for i, ok := range a.received {
		if !ok {
			out = append(out, uint16(i))
		}
	}
	return out
}
func (a *Assembly) Complete() bool { return len(a.Missing()) == 0 }

func (a *Assembly) Content() ([]byte, error) {
	if !a.Complete() {
		return nil, errors.New("publication is incomplete")
	}
	content := make([]byte, 0, a.Manifest.Length)
	for _, chunk := range a.chunks {
		content = append(content, chunk...)
	}
	if sha256.Sum256(content) != a.Manifest.SHA256 {
		return nil, ErrHashMismatch
	}
	return content, nil
}
