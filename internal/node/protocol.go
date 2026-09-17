package node

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"sort"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

const (
	Version                    = 3
	MaxPacketSize              = publication.MaxPacketSize
	HeaderSize                 = 14
	InventoryFixedSize         = 9
	IDsPerInventoryPage        = (MaxPacketSize - HeaderSize - InventoryFixedSize) / 16
	MaxChunkBatch              = 4
	Broadcast           NodeID = 0xffffffff
)

type NodeID uint32
type Type byte

const (
	TypeInventory      Type = 1
	TypeGetManifest    Type = 2
	TypeManifest       Type = 3
	TypeGetChunks      Type = 4
	TypeChunk          Type = 5
	TypeError          Type = 6
	TypeSummaryRequest Type = 7
	TypeSummary        Type = 8
	TypeLeaf           Type = 9
	TypeGetSignature   Type = 10
	TypeSignature      Type = 11
)

type Packet struct {
	Type                Type
	Source, Destination NodeID
	Transaction         uint16
	Body                []byte
}

var ErrMalformed = errors.New("malformed node packet")

func Encode(packet Packet) ([]byte, error) {
	if len(packet.Body)+HeaderSize > MaxPacketSize {
		return nil, ErrMalformed
	}
	b := make([]byte, HeaderSize+len(packet.Body))
	b[0], b[1], b[2], b[3] = 'G', 'N', Version, byte(packet.Type)
	binary.LittleEndian.PutUint32(b[4:8], uint32(packet.Source))
	binary.LittleEndian.PutUint32(b[8:12], uint32(packet.Destination))
	binary.LittleEndian.PutUint16(b[12:14], packet.Transaction)
	copy(b[14:], packet.Body)
	return b, nil
}

func Decode(b []byte) (Packet, error) {
	if len(b) < HeaderSize || len(b) > MaxPacketSize || b[0] != 'G' || b[1] != 'N' || b[2] != Version {
		return Packet{}, ErrMalformed
	}
	return Packet{Type: Type(b[3]), Source: NodeID(binary.LittleEndian.Uint32(b[4:8])), Destination: NodeID(binary.LittleEndian.Uint32(b[8:12])), Transaction: binary.LittleEndian.Uint16(b[12:14]), Body: append([]byte(nil), b[14:]...)}, nil
}

type Inventory struct {
	Generation  uint32
	Page, Pages uint16
	IDs         []publication.ID
}

func InventoryPages(ids []publication.ID) []Inventory {
	ids = append([]publication.ID(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return string(ids[i][:]) < string(ids[j][:]) })
	hash := crc32.NewIEEE()
	for _, id := range ids {
		hash.Write(id[:])
	}
	generation := hash.Sum32()
	pages := (len(ids) + IDsPerInventoryPage - 1) / IDsPerInventoryPage
	if pages == 0 {
		pages = 1
	}
	out := make([]Inventory, pages)
	for page := 0; page < pages; page++ {
		start := page * IDsPerInventoryPage
		end := min(start+IDsPerInventoryPage, len(ids))
		out[page] = Inventory{Generation: generation, Page: uint16(page), Pages: uint16(pages), IDs: append([]publication.ID(nil), ids[start:end]...)}
	}
	return out
}

func EncodeInventory(source NodeID, inventory Inventory) ([]byte, error) {
	if inventory.Pages == 0 || inventory.Page >= inventory.Pages || len(inventory.IDs) > IDsPerInventoryPage {
		return nil, ErrMalformed
	}
	body := make([]byte, InventoryFixedSize+16*len(inventory.IDs))
	binary.LittleEndian.PutUint32(body[0:4], inventory.Generation)
	binary.LittleEndian.PutUint16(body[4:6], inventory.Page)
	binary.LittleEndian.PutUint16(body[6:8], inventory.Pages)
	body[8] = byte(len(inventory.IDs))
	for i, id := range inventory.IDs {
		copy(body[9+i*16:], id[:])
	}
	return Encode(Packet{Type: TypeInventory, Source: source, Destination: Broadcast, Body: body})
}

func DecodeInventory(packet Packet) (Inventory, error) {
	if packet.Type != TypeInventory || packet.Destination != Broadcast || len(packet.Body) < InventoryFixedSize || int(packet.Body[8]) > IDsPerInventoryPage || len(packet.Body) != InventoryFixedSize+int(packet.Body[8])*16 {
		return Inventory{}, ErrMalformed
	}
	inv := Inventory{Generation: binary.LittleEndian.Uint32(packet.Body[:4]), Page: binary.LittleEndian.Uint16(packet.Body[4:6]), Pages: binary.LittleEndian.Uint16(packet.Body[6:8])}
	if inv.Pages == 0 || inv.Page >= inv.Pages {
		return Inventory{}, ErrMalformed
	}
	inv.IDs = make([]publication.ID, int(packet.Body[8]))
	for i := range inv.IDs {
		copy(inv.IDs[i][:], packet.Body[9+i*16:])
	}
	return inv, nil
}

func EncodeIDRequest(kind Type, source, destination NodeID, tx uint16, id publication.ID) ([]byte, error) {
	return Encode(Packet{Type: kind, Source: source, Destination: destination, Transaction: tx, Body: id[:]})
}
func DecodeIDBody(packet Packet) (publication.ID, error) {
	var id publication.ID
	if len(packet.Body) != len(id) {
		return id, ErrMalformed
	}
	copy(id[:], packet.Body)
	return id, nil
}

func EncodeManifest(source, destination NodeID, tx uint16, m publication.Manifest) ([]byte, error) {
	legacy, err := publication.EncodeManifest(m)
	if err != nil {
		return nil, err
	}
	p, _ := publication.DecodePacket(legacy)
	return Encode(Packet{Type: TypeManifest, Source: source, Destination: destination, Transaction: tx, Body: p.Body})
}
func DecodeManifest(packet Packet, id publication.ID) (publication.Manifest, error) {
	if packet.Type != TypeManifest {
		return publication.Manifest{}, ErrMalformed
	}
	return publication.DecodeManifest(publication.Packet{Type: publication.TypeManifest, ID: id, Body: packet.Body})
}

func EncodeSignatureRequest(source, destination NodeID, tx uint16, id publication.ID) ([]byte, error) {
	return EncodeIDRequest(TypeGetSignature, source, destination, tx, id)
}
func EncodeSignature(source, destination NodeID, tx uint16, signature *publication.Signature) ([]byte, error) {
	body := []byte{0}
	if signature != nil {
		body = make([]byte, 97)
		body[0] = 1
		copy(body[1:33], signature.PublicKey[:])
		copy(body[33:], signature.Value[:])
	}
	return Encode(Packet{Type: TypeSignature, Source: source, Destination: destination, Transaction: tx, Body: body})
}
func DecodeSignature(packet Packet) (*publication.Signature, error) {
	if packet.Type != TypeSignature || len(packet.Body) < 1 {
		return nil, ErrMalformed
	}
	if packet.Body[0] == 0 {
		if len(packet.Body) != 1 {
			return nil, ErrMalformed
		}
		return nil, nil
	}
	if packet.Body[0] != 1 || len(packet.Body) != 97 {
		return nil, ErrMalformed
	}
	sig := &publication.Signature{}
	copy(sig.PublicKey[:], packet.Body[1:33])
	copy(sig.Value[:], packet.Body[33:97])
	return sig, nil
}

type ChunkRequest struct {
	ID     publication.ID
	Base   uint16
	Bitmap byte
}

func EncodeChunkRequest(source, destination NodeID, tx uint16, r ChunkRequest) ([]byte, error) {
	if bits(r.Bitmap) > MaxChunkBatch {
		return nil, ErrMalformed
	}
	body := make([]byte, 19)
	copy(body, r.ID[:])
	binary.LittleEndian.PutUint16(body[16:18], r.Base)
	body[18] = r.Bitmap
	return Encode(Packet{Type: TypeGetChunks, Source: source, Destination: destination, Transaction: tx, Body: body})
}
func DecodeChunkRequest(packet Packet) (ChunkRequest, error) {
	var r ChunkRequest
	if packet.Type != TypeGetChunks || len(packet.Body) != 19 || bits(packet.Body[18]) > MaxChunkBatch {
		return r, ErrMalformed
	}
	copy(r.ID[:], packet.Body[:16])
	r.Base = binary.LittleEndian.Uint16(packet.Body[16:18])
	r.Bitmap = packet.Body[18]
	return r, nil
}
func EncodeChunk(source, destination NodeID, tx, index uint16, data []byte) ([]byte, error) {
	if len(data) > publication.ChunkDataSize {
		return nil, ErrMalformed
	}
	body := make([]byte, 2+len(data))
	binary.LittleEndian.PutUint16(body, index)
	copy(body[2:], data)
	return Encode(Packet{Type: TypeChunk, Source: source, Destination: destination, Transaction: tx, Body: body})
}
func DecodeChunk(packet Packet) (uint16, []byte, error) {
	if packet.Type != TypeChunk || len(packet.Body) < 2 || len(packet.Body)-2 > publication.ChunkDataSize {
		return 0, nil, ErrMalformed
	}
	return binary.LittleEndian.Uint16(packet.Body[:2]), append([]byte(nil), packet.Body[2:]...), nil
}
func bits(v byte) int {
	n := 0
	for v > 0 {
		n += int(v & 1)
		v >>= 1
	}
	return n
}

func EncodeSummaryRequest(source, destination NodeID, tx uint16, generation uint32, prefix []byte) ([]byte, error) {
	body, err := encodePrefix(generation, prefix)
	if err != nil {
		return nil, err
	}
	return Encode(Packet{Type: TypeSummaryRequest, Source: source, Destination: destination, Transaction: tx, Body: body})
}

func DecodeSummaryRequest(packet Packet) (uint32, []byte, error) {
	if packet.Type != TypeSummaryRequest {
		return 0, nil, ErrMalformed
	}
	return decodePrefix(packet.Body)
}

func EncodeSummary(source, destination NodeID, tx uint16, summary RadixSummary) ([]byte, error) {
	if len(summary.Prefix) >= 32 {
		return nil, ErrMalformed
	}
	body, err := encodePrefix(summary.Generation, summary.Prefix)
	if err != nil {
		return nil, err
	}
	for _, child := range summary.Children {
		entry := make([]byte, 10)
		binary.LittleEndian.PutUint16(entry[:2], child.Count)
		binary.LittleEndian.PutUint64(entry[2:], child.Digest)
		body = append(body, entry...)
	}
	return Encode(Packet{Type: TypeSummary, Source: source, Destination: destination, Transaction: tx, Body: body})
}

func DecodeSummary(packet Packet) (RadixSummary, error) {
	if packet.Type != TypeSummary {
		return RadixSummary{}, ErrMalformed
	}
	generation, prefix, err := decodePrefixWithTail(packet.Body, 160)
	if err != nil {
		return RadixSummary{}, err
	}
	if len(prefix) >= 32 {
		return RadixSummary{}, ErrMalformed
	}
	offset := 5 + (len(prefix)+1)/2
	summary := RadixSummary{Generation: generation, Prefix: prefix}
	for i := 0; i < 16; i++ {
		summary.Children[i] = ChildDigest{Count: binary.LittleEndian.Uint16(packet.Body[offset : offset+2]), Digest: binary.LittleEndian.Uint64(packet.Body[offset+2 : offset+10])}
		offset += 10
	}
	return summary, nil
}

func EncodeLeaf(source, destination NodeID, tx uint16, leaf RadixLeaf) ([]byte, error) {
	if len(leaf.IDs) > LeafThreshold {
		return nil, ErrMalformed
	}
	body, err := encodePrefix(leaf.Generation, leaf.Prefix)
	if err != nil {
		return nil, err
	}
	body = append(body, byte(len(leaf.IDs)))
	for _, id := range leaf.IDs {
		body = append(body, id[:]...)
	}
	return Encode(Packet{Type: TypeLeaf, Source: source, Destination: destination, Transaction: tx, Body: body})
}

func DecodeLeaf(packet Packet) (RadixLeaf, error) {
	if packet.Type != TypeLeaf || len(packet.Body) < 6 {
		return RadixLeaf{}, ErrMalformed
	}
	generation, prefix, err := decodePrefix(packet.Body)
	if err != nil {
		return RadixLeaf{}, err
	}
	offset := 5 + (len(prefix)+1)/2
	if offset >= len(packet.Body) {
		return RadixLeaf{}, ErrMalformed
	}
	count := int(packet.Body[offset])
	offset++
	if count > LeafThreshold || len(packet.Body) != offset+count*16 {
		return RadixLeaf{}, ErrMalformed
	}
	leaf := RadixLeaf{Generation: generation, Prefix: prefix, IDs: make([]publication.ID, count)}
	for i := range leaf.IDs {
		copy(leaf.IDs[i][:], packet.Body[offset+i*16:])
	}
	return leaf, nil
}

func encodePrefix(generation uint32, prefix []byte) ([]byte, error) {
	if len(prefix) > 32 {
		return nil, ErrMalformed
	}
	body := make([]byte, 5+(len(prefix)+1)/2)
	binary.LittleEndian.PutUint32(body[:4], generation)
	body[4] = byte(len(prefix))
	for i, nibble := range prefix {
		if nibble > 15 {
			return nil, ErrMalformed
		}
		if i%2 == 0 {
			body[5+i/2] = nibble << 4
		} else {
			body[5+i/2] |= nibble
		}
	}
	return body, nil
}
func decodePrefix(body []byte) (uint32, []byte, error) {
	if len(body) < 5 {
		return 0, nil, ErrMalformed
	}
	required := 5 + (int(body[4])+1)/2
	if required > len(body) {
		return 0, nil, ErrMalformed
	}
	return decodePrefixWithTail(body, len(body)-required)
}
func decodePrefixWithTail(body []byte, tail int) (uint32, []byte, error) {
	if len(body) < 5 {
		return 0, nil, ErrMalformed
	}
	depth := int(body[4])
	if depth > 32 || len(body) != 5+(depth+1)/2+tail {
		return 0, nil, ErrMalformed
	}
	prefix := make([]byte, depth)
	for i := range prefix {
		packed := body[5+i/2]
		if i%2 == 0 {
			prefix[i] = packed >> 4
		} else {
			prefix[i] = packed & 15
		}
	}
	if depth%2 == 1 && body[5+depth/2]&15 != 0 {
		return 0, nil, ErrMalformed
	}
	return binary.LittleEndian.Uint32(body[:4]), prefix, nil
}
