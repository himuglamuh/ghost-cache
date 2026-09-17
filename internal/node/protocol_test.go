package node

import (
	"bytes"
	"testing"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

func makeIDs(count int) []publication.ID {
	ids := make([]publication.ID, count)
	for i := range ids {
		ids[i][0] = byte(i)
		ids[i][15] = byte(255 - i)
	}
	return ids
}

func TestInventoryPaginationAndBoundary(t *testing.T) {
	ids := makeIDs(IDsPerInventoryPage*2 + 1)
	pages := InventoryPages(ids)
	if len(pages) != 3 {
		t.Fatalf("pages=%d", len(pages))
	}
	seen := map[publication.ID]bool{}
	for i, page := range pages {
		wire, err := EncodeInventory(7, page)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && len(wire) != 199 {
			t.Fatalf("full page bytes=%d", len(wire))
		}
		packet, err := Decode(wire)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeInventory(packet)
		if err != nil {
			t.Fatal(err)
		}
		if got.Page != uint16(i) || got.Pages != 3 {
			t.Fatalf("page=%+v", got)
		}
		for _, id := range got.IDs {
			if seen[id] {
				t.Fatal("duplicate ID")
			}
			seen[id] = true
		}
	}
	if len(seen) != len(ids) {
		t.Fatalf("seen=%d", len(seen))
	}
}

func TestMalformedInventory(t *testing.T) {
	wire, _ := EncodeInventory(1, Inventory{Pages: 1, IDs: makeIDs(1)})
	wire[HeaderSize+8] = 12
	packet, _ := Decode(wire)
	if _, err := DecodeInventory(packet); err == nil {
		t.Fatal("accepted invalid count")
	}
	if _, err := Decode(make([]byte, 201)); err == nil {
		t.Fatal("accepted oversized packet")
	}
}

func TestPacketSizeBoundaries(t *testing.T) {
	for _, size := range []int{199, 200} {
		body := make([]byte, size-HeaderSize)
		wire, err := Encode(Packet{Type: TypeError, Source: 1, Destination: 2, Body: body})
		if err != nil || len(wire) != size {
			t.Fatalf("size %d: bytes=%d err=%v", size, len(wire), err)
		}
		if _, err := Decode(wire); err != nil {
			t.Fatalf("decode size %d: %v", size, err)
		}
	}
	if _, err := Encode(Packet{Body: make([]byte, 201-HeaderSize)}); err == nil {
		t.Fatal("encoded 201-byte packet")
	}
}

func TestRadixPacketSizes(t *testing.T) {
	ids := makeIDs(LeafThreshold)
	root, _ := EncodeSummary(1, Broadcast, 0, BuildSummary(ids, nil))
	if len(root) != 179 {
		t.Fatalf("root bytes=%d", len(root))
	}
	deep := make([]byte, 31)
	summary, _ := EncodeSummary(1, 2, 1, BuildSummary(ids, deep))
	if len(summary) != 195 {
		t.Fatalf("deep summary bytes=%d", len(summary))
	}
	leaf, _ := EncodeLeaf(1, 2, 1, BuildLeaf(ids, nil))
	if len(leaf) != 180 {
		t.Fatalf("leaf bytes=%d", len(leaf))
	}
}

func TestMalformedRadixPackets(t *testing.T) {
	wire, _ := EncodeSummary(1, 2, 1, BuildSummary(nil, nil))
	wire[HeaderSize+4] = 32
	packet, _ := Decode(wire)
	if _, err := DecodeSummary(packet); err == nil {
		t.Fatal("accepted invalid prefix depth")
	}
	leaf, _ := EncodeLeaf(1, 2, 1, BuildLeaf(makeIDs(1), nil))
	leaf[HeaderSize+5] = 11
	packet, _ = Decode(leaf)
	if _, err := DecodeLeaf(packet); err == nil {
		t.Fatal("accepted invalid leaf count")
	}
}

func TestTruncatedPrefixNeverPanics(t *testing.T) {
	for depth := byte(1); depth <= 32; depth++ {
		packet := Packet{Type: TypeSummaryRequest, Body: []byte{0, 0, 0, 0, depth}}
		if _, _, err := DecodeSummaryRequest(packet); err == nil {
			t.Fatalf("accepted truncated depth %d", depth)
		}
	}
}

func TestDepth32SummaryRejectedButLeafAllowed(t *testing.T) {
	prefix := make([]byte, 32)
	if _, err := EncodeSummary(1, 2, 3, BuildSummary(nil, prefix)); err == nil {
		t.Fatal("encoded depth-32 summary")
	}
	leaf, err := EncodeLeaf(1, 2, 3, BuildLeaf(nil, prefix))
	if err != nil {
		t.Fatal(err)
	}
	packet, _ := Decode(leaf)
	if _, err := DecodeLeaf(packet); err != nil {
		t.Fatal(err)
	}
}

func TestChunkPacketPreservesApplicationChunkSize(t *testing.T) {
	data := bytes.Repeat([]byte{0xa5}, publication.ChunkDataSize)
	wire, err := EncodeChunk(1, 2, 3, 9, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != HeaderSize+2+publication.ChunkDataSize {
		t.Fatalf("bytes=%d", len(wire))
	}
	packet, _ := Decode(wire)
	index, got, err := DecodeChunk(packet)
	if err != nil || index != 9 || !bytes.Equal(got, data) {
		t.Fatalf("index=%d err=%v", index, err)
	}
}

func TestSignaturePacketCompact(t *testing.T) {
	m, _ := signedTestPublication(t)
	wire, err := EncodeSignature(1, 2, 3, m.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 111 {
		t.Fatalf("signed bytes=%d", len(wire))
	}
	packet, _ := Decode(wire)
	signature, err := DecodeSignature(packet)
	if err != nil || signature == nil || *signature != *m.Signature {
		t.Fatalf("signature=%v err=%v", signature, err)
	}
	unsigned, _ := EncodeSignature(1, 2, 3, nil)
	if len(unsigned) != 15 {
		t.Fatalf("unsigned bytes=%d", len(unsigned))
	}
}

func TestChunkRequestBounded(t *testing.T) {
	_, err := EncodeChunkRequest(1, 2, 3, ChunkRequest{Bitmap: 0x1f})
	if err == nil {
		t.Fatal("accepted five-chunk burst")
	}
	base, bitmap, indexes := MissingBatch([]bool{true, false, false, false, false, false, false, false, false}, 0)
	if base != 0 || bitmap != 0x1e || len(indexes) != MaxChunkBatch {
		t.Fatalf("base=%d bitmap=%x indexes=%v", base, bitmap, indexes)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte("GN"))
	f.Add(make([]byte, MaxPacketSize))
	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := Decode(data)
		if err == nil {
			_, _ = Encode(packet)
			if packet.Type == TypeInventory {
				_, _ = DecodeInventory(packet)
			}
			if packet.Type == TypeGetChunks {
				_, _ = DecodeChunkRequest(packet)
			}
			if packet.Type == TypeChunk {
				_, _, _ = DecodeChunk(packet)
			}
		}
	})
}

func FuzzInventoryDecoder(f *testing.F) {
	valid, _ := EncodeInventory(1, InventoryPages(makeIDs(IDsPerInventoryPage))[0])
	f.Add(valid)
	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := Decode(data)
		if err == nil {
			_, _ = DecodeInventory(packet)
		}
	})
}
func FuzzChunkRequestDecoder(f *testing.F) {
	valid, _ := EncodeChunkRequest(1, 2, 3, ChunkRequest{Bitmap: 0x0f})
	f.Add(valid)
	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := Decode(data)
		if err == nil {
			_, _ = DecodeChunkRequest(packet)
		}
	})
}
func FuzzChunkDecoder(f *testing.F) {
	valid, _ := EncodeChunk(1, 2, 3, 0, []byte("data"))
	f.Add(valid)
	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := Decode(data)
		if err == nil {
			_, _, _ = DecodeChunk(packet)
		}
	})
}
