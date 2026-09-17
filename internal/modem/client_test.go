package modem

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

type fakePort struct {
	mu          sync.Mutex
	readTimeout time.Duration
	readData    []byte
	writes      int
}

func (p *fakePort) Read(dst []byte) (int, error) {
	p.mu.Lock()
	if len(p.readData) > 0 {
		n := copy(dst, p.readData)
		p.readData = p.readData[n:]
		p.mu.Unlock()
		return n, nil
	}
	timeout := p.readTimeout
	p.mu.Unlock()
	if timeout > 0 {
		time.Sleep(timeout)
	}
	return 0, nil
}

func (p *fakePort) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	if p.writes == 2 {
		request, err := DecodeFrame(data[:len(data)-1])
		if err != nil {
			return 0, err
		}
		response, _ := EncodeFrame(Frame{Type: MsgStatus, Sequence: request.Sequence, Payload: []byte("ready")})
		rx, _ := EncodeFrame(Frame{Type: MsgRXPacket, Sequence: 0, Payload: []byte("queued")})
		p.readData = append(p.readData, []byte("ESP-ROM boot\nghost-modem boot\n")...)
		p.readData = append(p.readData, 2, 1, 0)
		p.readData = append(p.readData, rx...)
		p.readData = append(p.readData, response...)
	}
	return len(data), nil
}

func (p *fakePort) Close() error { return nil }

func (p *fakePort) SetReadTimeout(timeout time.Duration) error {
	p.mu.Lock()
	p.readTimeout = timeout
	p.mu.Unlock()
	return nil
}

func TestRequestRetriesThroughBootNoise(t *testing.T) {
	port := &fakePort{}
	client := &Client{port: port}
	frame, err := client.requestWithRetry(MsgInfoRequest, nil, 200*time.Millisecond, 10*time.Millisecond, func(frame Frame) bool {
		return frame.Type == MsgStatus
	})
	if err != nil {
		t.Fatal(err)
	}
	if port.writes != 2 {
		t.Fatalf("got %d writes, want 2", port.writes)
	}
	if !bytes.Equal(frame.Payload, []byte("ready")) {
		t.Fatalf("got payload %q", frame.Payload)
	}
	queued, err := client.ReadFrame(time.Millisecond)
	if err != nil || queued.Type != MsgRXPacket || !bytes.Equal(queued.Payload, []byte("queued")) {
		t.Fatalf("queued frame was lost: %#v, %v", queued, err)
	}
}

type partialWritePort struct {
	fakePort
	written []byte
}

func (p *partialWritePort) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	n := min(3, len(data))
	p.written = append(p.written, data[:n]...)
	return n, nil
}

func TestSendCompletesPartialWrites(t *testing.T) {
	port := &partialWritePort{}
	client := &Client{port: port}
	if _, err := client.Send(MsgTXRequest, []byte{0, 1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	frame, err := DecodeFrame(port.written[:len(port.written)-1])
	if err != nil {
		t.Fatal(err)
	}
	if frame.Type != MsgTXRequest || !bytes.Equal(frame.Payload, []byte{0, 1, 2, 3}) {
		t.Fatalf("unexpected frame: %#v", frame)
	}
}

var _ serialPort = (*fakePort)(nil)
var _ io.ReadWriteCloser = (*fakePort)(nil)
