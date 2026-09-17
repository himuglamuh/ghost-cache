package modem

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"go.bug.st/serial"
)

var ErrTimeout = errors.New("modem timeout")

const (
	startupTimeout       = 4 * time.Second
	requestRetryInterval = 400 * time.Millisecond
)

type serialPort interface {
	io.ReadWriteCloser
	SetReadTimeout(time.Duration) error
}

type Client struct {
	port    serialPort
	mu      sync.Mutex
	read    sync.Mutex
	seq     uint16
	decoder StreamDecoder
	pending []Frame
}

func Open(device string, baud int) (*Client, error) {
	port, err := serial.Open(device, &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
		// CP210x adapters commonly connect these lines to ESP32 EN/BOOT.
		// Unix may still pulse them during open, but this prevents leaving
		// either line asserted afterward. The library disables RTS/CTS flow.
		InitialStatusBits: &serial.ModemOutputBits{DTR: false, RTS: false},
	})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	client := &Client{port: port}
	_, err = client.requestWithRetry(MsgInfoRequest, nil, startupTimeout, requestRetryInterval, func(frame Frame) bool {
		return frame.Type == MsgStatus || frame.Type == MsgError
	})
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("synchronize with modem after opening %s: %w", device, err)
	}
	return client, nil
}

func (c *Client) Close() error { return c.port.Close() }

func (c *Client) Info(timeout time.Duration) (string, error) {
	frame, err := c.Request(MsgInfoRequest, nil, timeout, func(f Frame) bool { return f.Type == MsgStatus || f.Type == MsgError })
	if err != nil {
		return "", err
	}
	if frame.Type == MsgError {
		result, e := DecodeResult(frame.Payload)
		if e != nil {
			return "", e
		}
		return "", fmt.Errorf("modem error %d: %s", result.Code, result.Message)
	}
	return string(frame.Payload), nil
}

func (c *Client) SetConfig(config RadioConfig, timeout time.Duration) (string, error) {
	frame, err := c.Request(MsgSetConfig, EncodeConfig(config), timeout, func(f Frame) bool { return f.Type == MsgStatus || f.Type == MsgError })
	if err != nil {
		return "", err
	}
	if frame.Type == MsgError {
		result, e := DecodeResult(frame.Payload)
		if e != nil {
			return "", e
		}
		return "", fmt.Errorf("modem config error %d: %s", result.Code, result.Message)
	}
	return string(frame.Payload), nil
}

func (c *Client) Send(t MessageType, payload []byte) (uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	encoded, err := EncodeFrame(Frame{Type: t, Sequence: c.seq, Payload: payload})
	if err != nil {
		return 0, err
	}
	if err := c.writeAll(encoded); err != nil {
		return 0, err
	}
	return c.seq, nil
}

func (c *Client) ReadFrames(handle func(Frame) error) error {
	for {
		frame, err := c.ReadFrame(serial.NoTimeout)
		if err != nil {
			return err
		}
		if err := handle(frame); err != nil {
			return err
		}
	}
}

// ReadFrame returns the next valid frame while discarding serial noise and
// malformed COBS records. A negative timeout waits indefinitely.
func (c *Client) ReadFrame(timeout time.Duration) (Frame, error) {
	c.read.Lock()
	defer c.read.Unlock()
	if len(c.pending) > 0 {
		frame := c.pending[0]
		c.pending = c.pending[1:]
		return frame, nil
	}
	deadline := time.Time{}
	if timeout >= 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		frames, err := c.readAvailable(deadline)
		if err != nil {
			return Frame{}, err
		}
		if len(frames) > 0 {
			c.pending = append(c.pending, frames[1:]...)
			return frames[0], nil
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return Frame{}, ErrTimeout
		}
	}
}

func (c *Client) Request(t MessageType, payload []byte, timeout time.Duration, accept func(Frame) bool) (Frame, error) {
	// Open has already synchronized through any reset. Do not automatically
	// retry commands here: retransmitting TX could send a packet twice.
	return c.requestWithRetry(t, payload, timeout, timeout, accept)
}

func (c *Client) requestWithRetry(t MessageType, payload []byte, timeout, retryInterval time.Duration, accept func(Frame) bool) (Frame, error) {
	c.read.Lock()
	defer c.read.Unlock()
	if timeout <= 0 || retryInterval <= 0 {
		return Frame{}, fmt.Errorf("timeout and retry interval must be positive")
	}

	seq, err := c.Send(t, payload)
	if err != nil {
		return Frame{}, err
	}
	defer c.port.SetReadTimeout(serial.NoTimeout)
	deadline := time.Now().Add(timeout)
	nextRetry := time.Now().Add(retryInterval)
	for time.Now().Before(deadline) {
		for i, frame := range c.pending {
			if frame.Sequence == seq && (accept == nil || accept(frame)) {
				c.pending = append(c.pending[:i], c.pending[i+1:]...)
				return frame, nil
			}
		}
		readUntil := nextRetry
		if deadline.Before(readUntil) {
			readUntil = deadline
		}
		frames, err := c.readAvailable(readUntil)
		if err != nil {
			return Frame{}, err
		}
		for i, frame := range frames {
			if frame.Sequence == seq && (accept == nil || accept(frame)) {
				c.pending = append(c.pending, frames[:i]...)
				c.pending = append(c.pending, frames[i+1:]...)
				return frame, nil
			}
		}
		c.pending = append(c.pending, frames...)
		if !time.Now().Before(nextRetry) && time.Now().Before(deadline) {
			encoded, err := EncodeFrame(Frame{Type: t, Sequence: seq, Payload: payload})
			if err != nil {
				return Frame{}, err
			}
			if err := c.writeAll(encoded); err != nil {
				return Frame{}, err
			}
			nextRetry = time.Now().Add(retryInterval)
		}
	}
	return Frame{}, ErrTimeout
}

func (c *Client) readAvailable(deadline time.Time) ([]Frame, error) {
	timeout := serial.NoTimeout
	if !deadline.IsZero() {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, ErrTimeout
		}
	}
	if err := c.port.SetReadTimeout(timeout); err != nil {
		return nil, err
	}
	buf := make([]byte, 256)
	n, err := c.port.Read(buf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read serial: %w", err)
	}
	frames, _ := c.decoder.Push(buf[:n])
	return frames, nil
}

func (c *Client) writeAll(data []byte) error {
	for len(data) > 0 {
		n, err := c.port.Write(data)
		if err != nil {
			return fmt.Errorf("write frame: %w", err)
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
