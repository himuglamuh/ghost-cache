package publication

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Version          = 1
	MaxPacketSize    = 200
	HeaderSize       = 20
	ChunkDataSize    = MaxPacketSize - HeaderSize - 2
	MaxFilenameBytes = MaxPacketSize - HeaderSize - 45
	MaxChunks        = (MaxPacketSize - HeaderSize - 2) * 8
)

type Type byte

const (
	TypeManifest Type = 1
	TypeChunk    Type = 2
	TypeQuery    Type = 3
	TypeReceipt  Type = 4
	TypeComplete Type = 5
	TypeError    Type = 6
)

type ID [16]byte

func (id ID) String() string { return fmt.Sprintf("%x", id[:]) }

type Packet struct {
	Type Type
	ID   ID
	Body []byte
}

var ErrMalformed = errors.New("malformed publication packet")

func EncodePacket(packet Packet) ([]byte, error) {
	if len(packet.Body)+HeaderSize > MaxPacketSize {
		return nil, ErrMalformed
	}
	b := make([]byte, HeaderSize+len(packet.Body))
	b[0], b[1], b[2], b[3] = 'G', 'P', Version, byte(packet.Type)
	copy(b[4:20], packet.ID[:])
	copy(b[20:], packet.Body)
	return b, nil
}

func DecodePacket(b []byte) (Packet, error) {
	if len(b) < HeaderSize || len(b) > MaxPacketSize || b[0] != 'G' || b[1] != 'P' || b[2] != Version {
		return Packet{}, ErrMalformed
	}
	var id ID
	copy(id[:], b[4:20])
	return Packet{Type: Type(b[3]), ID: id, Body: append([]byte(nil), b[20:]...)}, nil
}

type Manifest struct {
	Version    uint16
	ID         ID
	Filename   string
	Length     uint64
	SHA256     [32]byte
	ChunkSize  uint16
	ChunkCount uint16
	Signature  *Signature
}

func EncodeManifest(m Manifest) ([]byte, error) {
	name := []byte(m.Filename)
	if len(name) > MaxFilenameBytes || m.ChunkSize == 0 || m.ChunkSize > ChunkDataSize || m.ChunkCount > MaxChunks || !IDMatchesHash(m.ID, m.SHA256) {
		return nil, ErrMalformed
	}
	body := make([]byte, 45+len(name))
	binary.LittleEndian.PutUint64(body[0:8], m.Length)
	copy(body[8:40], m.SHA256[:])
	binary.LittleEndian.PutUint16(body[40:42], m.ChunkSize)
	binary.LittleEndian.PutUint16(body[42:44], m.ChunkCount)
	body[44] = byte(len(name))
	copy(body[45:], name)
	return EncodePacket(Packet{Type: TypeManifest, ID: m.ID, Body: body})
}

func NormalizeManifest(m Manifest) Manifest {
	if m.Version == 0 {
		m.Version = 1
	}
	return m
}

func DecodeManifest(packet Packet) (Manifest, error) {
	if packet.Type != TypeManifest || len(packet.Body) < 45 || int(packet.Body[44]) != len(packet.Body)-45 {
		return Manifest{}, ErrMalformed
	}
	var hash [32]byte
	copy(hash[:], packet.Body[8:40])
	m := Manifest{Version: 1, ID: packet.ID, Filename: string(packet.Body[45:]), Length: binary.LittleEndian.Uint64(packet.Body[0:8]), SHA256: hash, ChunkSize: binary.LittleEndian.Uint16(packet.Body[40:42]), ChunkCount: binary.LittleEndian.Uint16(packet.Body[42:44])}
	if m.ChunkSize == 0 || m.ChunkSize > ChunkDataSize || m.ChunkCount > MaxChunks || m.Length > uint64(MaxChunks)*uint64(m.ChunkSize) || !IDMatchesHash(m.ID, m.SHA256) {
		return Manifest{}, ErrMalformed
	}
	expected := uint64(0)
	if m.Length > 0 {
		expected = (m.Length-1)/uint64(m.ChunkSize) + 1
	}
	if uint64(m.ChunkCount) != expected {
		return Manifest{}, ErrMalformed
	}
	return m, nil
}

func IDMatchesHash(id ID, hash [32]byte) bool {
	for i := range id {
		if id[i] != hash[i] {
			return false
		}
	}
	return true
}

func EncodeChunk(id ID, index uint16, data []byte) ([]byte, error) {
	if len(data) > ChunkDataSize {
		return nil, ErrMalformed
	}
	body := make([]byte, 2+len(data))
	binary.LittleEndian.PutUint16(body, index)
	copy(body[2:], data)
	return EncodePacket(Packet{Type: TypeChunk, ID: id, Body: body})
}

func DecodeChunk(packet Packet) (uint16, []byte, error) {
	if packet.Type != TypeChunk || len(packet.Body) < 2 || len(packet.Body)-2 > ChunkDataSize {
		return 0, nil, ErrMalformed
	}
	return binary.LittleEndian.Uint16(packet.Body[:2]), append([]byte(nil), packet.Body[2:]...), nil
}

func EncodeSimple(t Type, id ID) ([]byte, error) { return EncodePacket(Packet{Type: t, ID: id}) }

func EncodeReceipt(id ID, chunkCount uint16, received []bool) ([]byte, error) {
	if int(chunkCount) != len(received) || chunkCount > MaxChunks {
		return nil, ErrMalformed
	}
	body := make([]byte, 2+(len(received)+7)/8)
	binary.LittleEndian.PutUint16(body, chunkCount)
	for i, ok := range received {
		if ok {
			body[2+i/8] |= 1 << (uint(i) % 8)
		}
	}
	return EncodePacket(Packet{Type: TypeReceipt, ID: id, Body: body})
}

func DecodeReceipt(packet Packet) ([]bool, error) {
	if packet.Type != TypeReceipt || len(packet.Body) < 2 {
		return nil, ErrMalformed
	}
	count := int(binary.LittleEndian.Uint16(packet.Body[:2]))
	if count > MaxChunks || len(packet.Body) != 2+(count+7)/8 {
		return nil, ErrMalformed
	}
	received := make([]bool, count)
	for i := range received {
		received[i] = packet.Body[2+i/8]&(1<<(uint(i)%8)) != 0
	}
	return received, nil
}

func EncodeError(id ID, message string) ([]byte, error) {
	if len(message) > MaxPacketSize-HeaderSize {
		message = message[:MaxPacketSize-HeaderSize]
	}
	return EncodePacket(Packet{Type: TypeError, ID: id, Body: []byte(message)})
}
