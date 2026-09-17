package publication

import (
	"errors"
	"fmt"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/modem"
)

type Link struct{ client *modem.Client }

func OpenLink(device string, baud int) (*Link, error) {
	client, err := modem.Open(device, baud)
	if err != nil {
		return nil, err
	}
	return &Link{client: client}, nil
}

func (l *Link) Close() error { return l.client.Close() }

func (l *Link) Send(packet []byte, timeout time.Duration) error {
	frame, err := l.client.Request(modem.MsgTXRequest, packet, timeout, func(frame modem.Frame) bool {
		return frame.Type == modem.MsgTXResult || frame.Type == modem.MsgError
	})
	if err != nil {
		return err
	}
	result, err := modem.DecodeResult(frame.Payload)
	if err != nil {
		return err
	}
	if frame.Type == modem.MsgError || result.Code != 0 {
		return fmt.Errorf("modem TX failed (%d): %s", result.Code, result.Message)
	}
	return nil
}

func (l *Link) Receive(timeout time.Duration) (Packet, error) {
	deadline := time.Time{}
	if timeout >= 0 {
		deadline = time.Now().Add(timeout)
	}
	for deadline.IsZero() || time.Now().Before(deadline) {
		remaining := timeout
		if !deadline.IsZero() {
			remaining = time.Until(deadline)
		}
		frame, err := l.client.ReadFrame(remaining)
		if err != nil {
			return Packet{}, err
		}
		if frame.Type == modem.MsgError {
			result, decodeErr := modem.DecodeResult(frame.Payload)
			if decodeErr == nil {
				return Packet{}, fmt.Errorf("modem error (%d): %s", result.Code, result.Message)
			}
			continue
		}
		if frame.Type != modem.MsgRXPacket {
			continue
		}
		rx, err := modem.DecodeRXPacket(frame.Payload)
		if err != nil {
			continue
		}
		packet, err := DecodePacket(rx.Payload)
		if errors.Is(err, ErrMalformed) {
			continue
		}
		if err != nil {
			return Packet{}, err
		}
		return packet, nil
	}
	return Packet{}, fmt.Errorf("timed out waiting for publication packet")
}
