package publication

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testManifest(content []byte) Manifest {
	hash := sha256.Sum256(content)
	var id ID
	copy(id[:], hash[:16])
	count := 0
	if len(content) > 0 {
		count = (len(content) + ChunkDataSize - 1) / ChunkDataSize
	}
	return Manifest{Version: 1, ID: id, Filename: "payload.bin", Length: uint64(len(content)), SHA256: hash, ChunkSize: ChunkDataSize, ChunkCount: uint16(count)}
}

func TestManifestRoundTrip(t *testing.T) {
	want := testManifest(make([]byte, 4096))
	wire, err := EncodeManifest(want)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := DecodePacket(wire)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeManifest(packet)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestChunkRoundTripBinary(t *testing.T) {
	data := make([]byte, ChunkDataSize)
	for i := range data {
		data[i] = byte(i)
	}
	id := testManifest(data).ID
	wire, err := EncodeChunk(id, 7, data)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := DecodePacket(wire)
	if err != nil {
		t.Fatal(err)
	}
	index, got, err := DecodeChunk(packet)
	if err != nil {
		t.Fatal(err)
	}
	if index != 7 || !bytes.Equal(got, data) {
		t.Fatal("chunk round trip mismatch")
	}
}

func TestReceiptMissing(t *testing.T) {
	want := []bool{true, false, true, true, false, false, false, true, true}
	wire, err := EncodeReceipt(ID{}, uint16(len(want)), want)
	if err != nil {
		t.Fatal(err)
	}
	packet, _ := DecodePacket(wire)
	got, err := DecodeReceipt(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(boolBytes(got), boolBytes(want)) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOutOfOrderAndDuplicateAssembly(t *testing.T) {
	content := make([]byte, ChunkDataSize*2+13)
	for i := range content {
		content[i] = byte(i)
	}
	m := testManifest(content)
	a := NewAssembly(m)
	chunks := split(content)
	if err := a.Add(2, chunks[2]); err != nil {
		t.Fatal(err)
	}
	if err := a.Add(0, chunks[0]); err != nil {
		t.Fatal(err)
	}
	if err := a.Add(0, chunks[0]); err != nil {
		t.Fatal("identical duplicate must be harmless:", err)
	}
	if len(a.Missing()) != 1 || a.Missing()[0] != 1 {
		t.Fatalf("missing=%v", a.Missing())
	}
	if err := a.Add(1, chunks[1]); err != nil {
		t.Fatal(err)
	}
	got, err := a.Content()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("reassembled content differs")
	}
}

func TestConflictingDuplicateRejected(t *testing.T) {
	content := []byte("hello")
	a := NewAssembly(testManifest(content))
	if err := a.Add(0, content); err != nil {
		t.Fatal(err)
	}
	if err := a.Add(0, []byte("HELLO")); !errors.Is(err, ErrMalformed) {
		t.Fatalf("got %v", err)
	}
}

func TestSHA256Mismatch(t *testing.T) {
	content := []byte("correct")
	a := NewAssembly(testManifest(content))
	if err := a.Add(0, []byte("corrupt")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Content(); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestEmptyAndBoundaryFiles(t *testing.T) {
	for _, size := range []int{0, 1, ChunkDataSize, ChunkDataSize + 1, ChunkDataSize * 2} {
		content := make([]byte, size)
		m := testManifest(content)
		wire, err := EncodeManifest(m)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		packet, _ := DecodePacket(wire)
		if _, err := DecodeManifest(packet); err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		a := NewAssembly(m)
		for i, chunk := range split(content) {
			if err := a.Add(uint16(i), chunk); err != nil {
				t.Fatal(err)
			}
		}
		got, err := a.Content()
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("size %d differs", size)
		}
	}
}

func TestMalformedPackets(t *testing.T) {
	cases := [][]byte{nil, []byte("GP"), append([]byte{'G', 'P', 2, 1}, make([]byte, 16)...), make([]byte, MaxPacketSize+1)}
	for _, data := range cases {
		if _, err := DecodePacket(data); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted malformed packet of length %d", len(data))
		}
	}
	m := testManifest([]byte("x"))
	wire, _ := EncodeManifest(m)
	wire[20+42] = 2
	packet, _ := DecodePacket(wire)
	if _, err := DecodeManifest(packet); !errors.Is(err, ErrMalformed) {
		t.Fatal("accepted inconsistent chunk count")
	}
	packet.Body[40], packet.Body[41] = 0, 0
	if _, err := DecodeManifest(packet); !errors.Is(err, ErrMalformed) {
		t.Fatal("accepted zero chunk size")
	}
	packet.Body[40], packet.Body[41] = 2, 0
	for i := 0; i < 8; i++ {
		packet.Body[i] = 0xff
	}
	if _, err := DecodeManifest(packet); !errors.Is(err, ErrMalformed) {
		t.Fatal("accepted overflowing content length")
	}
}

func TestManifestRejectsIDHashMismatch(t *testing.T) {
	m := testManifest([]byte("data"))
	m.ID[0] ^= 1
	if _, err := EncodeManifest(m); !errors.Is(err, ErrMalformed) {
		t.Fatalf("got %v", err)
	}
}

func TestStoreResumeAndCommit(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	content := make([]byte, ChunkDataSize+9)
	for i := range content {
		content[i] = byte(i)
	}
	m := testManifest(content)
	chunks := split(content)
	a, err := store.Begin(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(a, 1, chunks[1]); err != nil {
		t.Fatal(err)
	}
	a, err = store.Begin(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Missing()) != 1 || a.Missing()[0] != 0 {
		t.Fatalf("resume missing=%v", a.Missing())
	}
	if err := store.Add(a, 0, chunks[0]); err != nil {
		t.Fatal(err)
	}
	path, err := store.Commit(a)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("committed content differs")
	}
	if _, err := os.Stat(filepath.Join(root, "partial", m.ID.String())); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial state remains")
	}
}

func TestLocalAddAndDuplicate(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "object.bin")
	content := bytes.Repeat([]byte("binary\x00"), 100)
	if err := os.WriteFile(input, content, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(root)
	first, path, err := store.AddFile(input)
	if err != nil {
		t.Fatal(err)
	}
	second, path2, err := store.AddFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || path != path2 {
		t.Fatal("duplicate add changed publication")
	}
	ids, err := store.IDs()
	if err != nil || len(ids) != 1 || ids[0] != first.ID {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("stored content differs")
	}
}

func TestPartialIDsSurviveRestart(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	content := bytes.Repeat([]byte{1}, ChunkDataSize+1)
	m := testManifest(content)
	a, err := store.Begin(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(a, 0, content[:ChunkDataSize]); err != nil {
		t.Fatal(err)
	}
	ids, err := NewStore(root).PartialIDs()
	if err != nil || len(ids) != 1 || ids[0] != m.ID {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	resumed, err := NewStore(root).LoadPartial(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := resumed.Missing(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("missing=%v", got)
	}
}

func TestKnownManifestAndWantSurviveRestart(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	manifest := testManifest([]byte("deferred"))
	if err := store.SaveKnown(manifest); err != nil {
		t.Fatal(err)
	}
	if err := store.Want(manifest.ID); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	got, err := reopened.LoadKnown(manifest.ID)
	if err != nil || got != manifest {
		t.Fatalf("manifest=%+v err=%v", got, err)
	}
	if !reopened.Wanted(manifest.ID) {
		t.Fatal("manual want did not persist")
	}
}

func TestCorruptKnownDoesNotSuppressDiscovery(t *testing.T) {
	root := t.TempDir()
	manifest := testManifest([]byte("known"))
	store := NewStore(root)
	if err := store.SaveKnown(manifest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "known", manifest.ID.String(), "manifest.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if store.HasKnown(manifest.ID) {
		t.Fatal("corrupt known metadata reported valid")
	}
}

func split(content []byte) [][]byte {
	var chunks [][]byte
	for start := 0; start < len(content); start += ChunkDataSize {
		chunks = append(chunks, content[start:min(start+ChunkDataSize, len(content))])
	}
	return chunks
}

func boolBytes(values []bool) []byte {
	out := make([]byte, len(values))
	for i, value := range values {
		if value {
			out[i] = 1
		}
	}
	return out
}

func FuzzPublicationPacketDecoder(f *testing.F) {
	f.Add([]byte("GP"))
	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := DecodePacket(data)
		if err == nil {
			switch packet.Type {
			case TypeManifest:
				_, _ = DecodeManifest(packet)
			case TypeChunk:
				_, _, _ = DecodeChunk(packet)
			case TypeReceipt:
				_, _ = DecodeReceipt(packet)
			}
		}
	})
}
