package node

import (
	"errors"
	"fmt"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/modem"
)

type RadioPacket struct {
	Data      []byte
	RSSI, SNR float32
}

type Radio interface {
	Send([]byte, time.Duration) error
	Receive(time.Duration) (RadioPacket, error)
	Close() error
}

type ModemRadio struct{ client *modem.Client }

func OpenRadio(device string, baud int) (*ModemRadio, error) {
	client, err := modem.Open(device, baud)
	if err != nil {
		return nil, err
	}
	return &ModemRadio{client: client}, nil
}

func (r *ModemRadio) Close() error                               { return r.client.Close() }
func (r *ModemRadio) Info(timeout time.Duration) (string, error) { return r.client.Info(timeout) }
func (r *ModemRadio) Configure(config modem.RadioConfig, timeout time.Duration) (string, error) {
	return r.client.SetConfig(config, timeout)
}

func (r *ModemRadio) Send(data []byte, timeout time.Duration) error {
	frame, err := r.client.Request(modem.MsgTXRequest, data, timeout, func(frame modem.Frame) bool { return frame.Type == modem.MsgTXResult || frame.Type == modem.MsgError })
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

func (r *ModemRadio) Receive(timeout time.Duration) (RadioPacket, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, err := r.client.ReadFrame(time.Until(deadline))
		if errors.Is(err, modem.ErrTimeout) {
			return RadioPacket{}, modem.ErrTimeout
		}
		if err != nil {
			return RadioPacket{}, err
		}
		if frame.Type != modem.MsgRXPacket {
			if frame.Type == modem.MsgError {
				result, decodeErr := modem.DecodeResult(frame.Payload)
				if decodeErr == nil {
					return RadioPacket{}, fmt.Errorf("modem receive error (%d): %s", result.Code, result.Message)
				}
			}
			continue
		}
		rx, err := modem.DecodeRXPacket(frame.Payload)
		if err != nil {
			continue
		}
		return RadioPacket{Data: rx.Payload, RSSI: rx.RSSI, SNR: rx.SNR}, nil
	}
	return RadioPacket{}, modem.ErrTimeout
}
