package modem

import (
	"bytes"
	"errors"
	"testing"
)

func TestArbitraryBinaryRoundTrip(t *testing.T) {
	payload := make([]byte, MaxRadioPayload)
	for i := range payload {
		payload[i] = byte(i)
	}
	wire, err := EncodeFrame(Frame{Type: MsgTXRequest, Sequence: 0x1234, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire[:len(wire)-1], []byte{0}) {
		t.Fatal("encoded frame contains delimiter")
	}
	got, err := DecodeFrame(wire[:len(wire)-1])
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != MsgTXRequest || got.Sequence != 0x1234 || !bytes.Equal(got.Payload, payload) {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}

func TestMalformedFrameRecovery(t *testing.T) {
	good, _ := EncodeFrame(Frame{Type: MsgInfoRequest, Sequence: 9})
	stream := append([]byte{2, 1, 0}, good...)
	frames, errs := new(StreamDecoder).Push(stream)
	if len(errs) != 1 || len(frames) != 1 || frames[0].Sequence != 9 {
		t.Fatalf("got %d errors and frames %#v", len(errs), frames)
	}
}

func TestCRCRejectsCorruption(t *testing.T) {
	wire, _ := EncodeFrame(Frame{Type: MsgTXRequest, Payload: []byte("hello")})
	raw, _ := cobsDecode(wire[:len(wire)-1])
	raw[8] ^= 1
	corrupt := cobsEncode(raw)
	_, err := DecodeFrame(corrupt)
	if !errors.Is(err, ErrCRC) {
		t.Fatalf("expected CRC error, got %v", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	got, err := DecodeConfig(EncodeConfig(DefaultConfig))
	if err != nil || got != DefaultConfig {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestMessageTypeParsing(t *testing.T) {
	for _, typ := range []MessageType{MsgRXPacket, MsgTXResult, MsgStatus, MsgError} {
		wire, _ := EncodeFrame(Frame{Type: typ})
		got, err := DecodeFrame(wire[:len(wire)-1])
		if err != nil || got.Type != typ || got.Type.String() == "" {
			t.Fatalf("type %x: %#v, %v", typ, got, err)
		}
	}
}
