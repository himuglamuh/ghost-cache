package modem

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
)

const (
	ProtocolVersion byte = 1
	MaxRadioPayload      = 255
	MaxFramePayload      = 512
)

var magic = [2]byte{'G', 'C'}

type MessageType byte

const (
	MsgInfoRequest MessageType = 0x01
	MsgTXRequest   MessageType = 0x02
	MsgSetConfig   MessageType = 0x03
	MsgSetOLED     MessageType = 0x04

	MsgRXPacket MessageType = 0x81
	MsgTXResult MessageType = 0x82
	MsgStatus   MessageType = 0x83
	MsgError    MessageType = 0x84
)

func (t MessageType) String() string {
	switch t {
	case MsgInfoRequest:
		return "INFO_REQUEST"
	case MsgTXRequest:
		return "TX_REQUEST"
	case MsgSetConfig:
		return "SET_CONFIG"
	case MsgSetOLED:
		return "SET_OLED"
	case MsgRXPacket:
		return "RX_PACKET"
	case MsgTXResult:
		return "TX_RESULT"
	case MsgStatus:
		return "STATUS"
	case MsgError:
		return "ERROR"
	default:
		return fmt.Sprintf("TYPE_0x%02x", byte(t))
	}
}

type Frame struct {
	Type     MessageType
	Sequence uint16
	Payload  []byte
}

var (
	ErrMalformed = errors.New("malformed frame")
	ErrCRC       = errors.New("frame CRC mismatch")
	ErrVersion   = errors.New("unsupported protocol version")
)

// EncodeFrame serializes, protects, and COBS-frames a message. The returned
// bytes include the terminating zero delimiter.
func EncodeFrame(frame Frame) ([]byte, error) {
	if len(frame.Payload) > MaxFramePayload {
		return nil, fmt.Errorf("payload is %d bytes; maximum is %d", len(frame.Payload), MaxFramePayload)
	}
	raw := make([]byte, 8+len(frame.Payload)+4)
	copy(raw[0:2], magic[:])
	raw[2] = ProtocolVersion
	raw[3] = byte(frame.Type)
	binary.LittleEndian.PutUint16(raw[4:6], frame.Sequence)
	binary.LittleEndian.PutUint16(raw[6:8], uint16(len(frame.Payload)))
	copy(raw[8:], frame.Payload)
	binary.LittleEndian.PutUint32(raw[len(raw)-4:], crc32.ChecksumIEEE(raw[:len(raw)-4]))
	encoded := cobsEncode(raw)
	return append(encoded, 0), nil
}

// DecodeFrame accepts one COBS frame without its zero delimiter.
func DecodeFrame(encoded []byte) (Frame, error) {
	raw, err := cobsDecode(encoded)
	if err != nil {
		return Frame{}, err
	}
	if len(raw) < 12 || raw[0] != magic[0] || raw[1] != magic[1] {
		return Frame{}, ErrMalformed
	}
	if raw[2] != ProtocolVersion {
		return Frame{}, fmt.Errorf("%w: %d", ErrVersion, raw[2])
	}
	payloadLen := int(binary.LittleEndian.Uint16(raw[6:8]))
	if payloadLen > MaxFramePayload || len(raw) != 8+payloadLen+4 {
		return Frame{}, ErrMalformed
	}
	wantCRC := binary.LittleEndian.Uint32(raw[len(raw)-4:])
	if crc32.ChecksumIEEE(raw[:len(raw)-4]) != wantCRC {
		return Frame{}, ErrCRC
	}
	payload := append([]byte(nil), raw[8:8+payloadLen]...)
	return Frame{Type: MessageType(raw[3]), Sequence: binary.LittleEndian.Uint16(raw[4:6]), Payload: payload}, nil
}

func cobsEncode(src []byte) []byte {
	dst := make([]byte, 1, len(src)+len(src)/254+1)
	codeIndex, code := 0, byte(1)
	for _, b := range src {
		if b == 0 {
			dst[codeIndex] = code
			codeIndex = len(dst)
			dst = append(dst, 0)
			code = 1
			continue
		}
		dst = append(dst, b)
		code++
		if code == 0xff {
			dst[codeIndex] = code
			codeIndex = len(dst)
			dst = append(dst, 0)
			code = 1
		}
	}
	dst[codeIndex] = code
	return dst
}

func cobsDecode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, ErrMalformed
	}
	dst := make([]byte, 0, len(src))
	for index := 0; index < len(src); {
		code := int(src[index])
		if code == 0 || index+code > len(src)+1 {
			return nil, ErrMalformed
		}
		index++
		end := index + code - 1
		if end > len(src) {
			return nil, ErrMalformed
		}
		dst = append(dst, src[index:end]...)
		index = end
		if code != 0xff && index < len(src) {
			dst = append(dst, 0)
		}
	}
	return dst, nil
}

type OLEDMode byte

const (
	OLEDDiagnostic OLEDMode = iota
	OLEDMinimal
	OLEDOff
)

type RadioConfig struct {
	FrequencyMHz    float32
	BandwidthKHz    float32
	SpreadingFactor uint8
	CodingRate      uint8
	TXPowerDBm      int8
	PreambleSymbols uint16
	SyncWord        uint8
}

var DefaultConfig = RadioConfig{915.0, 125.0, 7, 5, 5, 8, 0x12}

func EncodeConfig(c RadioConfig) []byte {
	b := make([]byte, 15)
	binary.LittleEndian.PutUint32(b[0:4], math.Float32bits(c.FrequencyMHz))
	binary.LittleEndian.PutUint32(b[4:8], math.Float32bits(c.BandwidthKHz))
	b[8] = c.SpreadingFactor
	b[9] = c.CodingRate
	b[10] = byte(c.TXPowerDBm)
	binary.LittleEndian.PutUint16(b[11:13], c.PreambleSymbols)
	b[13] = c.SyncWord
	b[14] = 0
	return b
}

func DecodeConfig(b []byte) (RadioConfig, error) {
	if len(b) != 15 {
		return RadioConfig{}, ErrMalformed
	}
	return RadioConfig{
		FrequencyMHz:    math.Float32frombits(binary.LittleEndian.Uint32(b[0:4])),
		BandwidthKHz:    math.Float32frombits(binary.LittleEndian.Uint32(b[4:8])),
		SpreadingFactor: b[8], CodingRate: b[9], TXPowerDBm: int8(b[10]),
		PreambleSymbols: binary.LittleEndian.Uint16(b[11:13]), SyncWord: b[13],
	}, nil
}

type RXPacket struct {
	RSSI    float32
	SNR     float32
	Payload []byte
}

func DecodeRXPacket(b []byte) (RXPacket, error) {
	if len(b) < 8 {
		return RXPacket{}, ErrMalformed
	}
	return RXPacket{
		RSSI:    math.Float32frombits(binary.LittleEndian.Uint32(b[0:4])),
		SNR:     math.Float32frombits(binary.LittleEndian.Uint32(b[4:8])),
		Payload: append([]byte(nil), b[8:]...),
	}, nil
}

type Result struct {
	Code    int16
	Message string
}

func DecodeResult(b []byte) (Result, error) {
	if len(b) < 2 {
		return Result{}, ErrMalformed
	}
	return Result{Code: int16(binary.LittleEndian.Uint16(b[:2])), Message: string(b[2:])}, nil
}
