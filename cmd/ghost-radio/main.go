package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/modem"
)

const defaultBaud = 115200

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ghost-radio:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("a command is required")
	}
	switch args[0] {
	case "info":
		return runInfo(args[1:])
	case "monitor":
		return runMonitor(args[1:])
	case "send":
		return runSend(args[1:])
	case "config":
		return runConfig(args[1:])
	case "oled":
		return runOLED(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func common(fs *flag.FlagSet) *string {
	return fs.String("device", "", "serial device, for example /dev/ttyACM0")
}

func open(device string) (*modem.Client, error) {
	if device == "" {
		return nil, errors.New("--device is required")
	}
	return modem.Open(device, defaultBaud)
}

func runInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	device := common(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := open(*device)
	if err != nil {
		return err
	}
	defer c.Close()
	frame, err := c.Request(modem.MsgInfoRequest, nil, 3*time.Second, func(f modem.Frame) bool { return f.Type == modem.MsgStatus || f.Type == modem.MsgError })
	if err != nil {
		return err
	}
	return printFrame(frame)
}

func runSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	device := common(fs)
	text := fs.String("text", "", "UTF-8 text payload")
	file := fs.String("file", "", "binary payload file")
	hexValue := fs.String("hex", "", "hexadecimal payload")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selected := 0
	if *text != "" {
		selected++
	}
	if *file != "" {
		selected++
	}
	if *hexValue != "" {
		selected++
	}
	if selected != 1 {
		return errors.New("specify exactly one of --text, --file, or --hex")
	}
	var payload []byte
	var err error
	if *text != "" {
		payload = []byte(*text)
	}
	if *file != "" {
		payload, err = os.ReadFile(*file)
	}
	if *hexValue != "" {
		payload, err = hex.DecodeString(strings.ReplaceAll(*hexValue, " ", ""))
	}
	if err != nil {
		return err
	}
	if len(payload) > modem.MaxRadioPayload {
		return fmt.Errorf("payload is %d bytes; SX1262 packet maximum is %d", len(payload), modem.MaxRadioPayload)
	}
	c, err := open(*device)
	if err != nil {
		return err
	}
	defer c.Close()
	frame, err := c.Request(modem.MsgTXRequest, payload, 10*time.Second, func(f modem.Frame) bool { return f.Type == modem.MsgTXResult || f.Type == modem.MsgError })
	if err != nil {
		return err
	}
	return printFrame(frame)
}

func runMonitor(args []string) error {
	fs := flag.NewFlagSet("monitor", flag.ContinueOnError)
	device := common(fs)
	output := fs.String("output", "", "write each RX payload to this file (requires --count=1)")
	count := fs.Int("count", 0, "exit after this many RX packets; zero means forever")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output != "" && *count != 1 {
		return errors.New("--output requires --count=1")
	}
	c, err := open(*device)
	if err != nil {
		return err
	}
	defer c.Close()
	received := 0
	err = c.ReadFrames(func(frame modem.Frame) error {
		if frame.Type != modem.MsgRXPacket {
			return printFrame(frame)
		}
		rx, err := modem.DecodeRXPacket(frame.Payload)
		if err != nil {
			return err
		}
		fmt.Printf("RX bytes=%d rssi=%.1f dBm snr=%.1f dB hex=%s\n", len(rx.Payload), rx.RSSI, rx.SNR, hex.EncodeToString(rx.Payload))
		if *output != "" {
			if err := os.WriteFile(*output, rx.Payload, 0600); err != nil {
				return err
			}
		}
		received++
		if *count > 0 && received >= *count {
			return errComplete
		}
		return nil
	})
	if errors.Is(err, errComplete) {
		return nil
	}
	return err
}

var errComplete = errors.New("complete")

func runConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	device := common(fs)
	cfg := modem.DefaultConfig
	fs.Var(float32Value{&cfg.FrequencyMHz}, "frequency", "frequency in MHz")
	fs.Var(float32Value{&cfg.BandwidthKHz}, "bandwidth", "bandwidth in kHz")
	fs.Var(uint8Value{&cfg.SpreadingFactor}, "sf", "spreading factor 5-12")
	fs.Var(uint8Value{&cfg.CodingRate}, "cr", "coding-rate denominator 5-8")
	fs.Var(int8Value{&cfg.TXPowerDBm}, "power", "TX power in dBm")
	fs.Var(uint16Value{&cfg.PreambleSymbols}, "preamble", "preamble symbols")
	syncWord := fs.String("sync", "0x12", "one-byte sync word")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sync, err := strconv.ParseUint(*syncWord, 0, 8)
	if err != nil {
		return fmt.Errorf("invalid --sync: %w", err)
	}
	cfg.SyncWord = byte(sync)
	c, err := open(*device)
	if err != nil {
		return err
	}
	defer c.Close()
	frame, err := c.Request(modem.MsgSetConfig, modem.EncodeConfig(cfg), 5*time.Second, func(f modem.Frame) bool { return f.Type == modem.MsgStatus || f.Type == modem.MsgError })
	if err != nil {
		return err
	}
	return printFrame(frame)
}

func runOLED(args []string) error {
	fs := flag.NewFlagSet("oled", flag.ContinueOnError)
	device := common(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("OLED mode is required: diagnostic, minimal, or off")
	}
	var mode modem.OLEDMode
	switch strings.ToLower(fs.Arg(0)) {
	case "diagnostic":
		mode = modem.OLEDDiagnostic
	case "minimal":
		mode = modem.OLEDMinimal
	case "off":
		mode = modem.OLEDOff
	default:
		return fmt.Errorf("unknown OLED mode %q", fs.Arg(0))
	}
	c, err := open(*device)
	if err != nil {
		return err
	}
	defer c.Close()
	frame, err := c.Request(modem.MsgSetOLED, []byte{byte(mode)}, 3*time.Second, func(f modem.Frame) bool { return f.Type == modem.MsgStatus || f.Type == modem.MsgError })
	if err != nil {
		return err
	}
	return printFrame(frame)
}

func printFrame(frame modem.Frame) error {
	switch frame.Type {
	case modem.MsgStatus:
		fmt.Println(string(frame.Payload))
		return nil
	case modem.MsgTXResult, modem.MsgError:
		result, err := modem.DecodeResult(frame.Payload)
		if err != nil {
			return err
		}
		fmt.Printf("%s code=%d message=%s\n", frame.Type, result.Code, result.Message)
		if frame.Type == modem.MsgError || result.Code != 0 {
			return fmt.Errorf("modem error %d: %s", result.Code, result.Message)
		}
		return nil
	default:
		fmt.Printf("%s sequence=%d payload=%x\n", frame.Type, frame.Sequence, frame.Payload)
		return nil
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: ghost-radio <info|monitor|send|config|oled> [options]") }

type float32Value struct{ p *float32 }

func (v float32Value) String() string { return fmt.Sprint(*v.p) }
func (v float32Value) Set(s string) error {
	n, err := strconv.ParseFloat(s, 32)
	*v.p = float32(n)
	return err
}

type uint8Value struct{ p *uint8 }

func (v uint8Value) String() string { return fmt.Sprint(*v.p) }
func (v uint8Value) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 8)
	*v.p = uint8(n)
	return err
}

type int8Value struct{ p *int8 }

func (v int8Value) String() string { return fmt.Sprint(*v.p) }
func (v int8Value) Set(s string) error {
	n, err := strconv.ParseInt(s, 10, 8)
	*v.p = int8(n)
	return err
}

type uint16Value struct{ p *uint16 }

func (v uint16Value) String() string { return fmt.Sprint(*v.p) }
func (v uint16Value) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 16)
	*v.p = uint16(n)
	return err
}
