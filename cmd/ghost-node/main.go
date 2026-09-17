package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	mathrand "math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	appconfig "github.com/himuglamuh/ghost-cache/internal/config"
	"github.com/himuglamuh/ghost-cache/internal/modem"
	"github.com/himuglamuh/ghost-cache/internal/node"
	"github.com/himuglamuh/ghost-cache/internal/publication"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ghost-node:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ghost-node <add|run|want|keygen|trust|library|inspect|config>")
	}
	switch args[0] {
	case "add":
		return add(args[1:])
	case "run":
		return runNode(args[1:])
	case "want":
		return want(args[1:])
	case "keygen":
		return keygen(args[1:])
	case "trust":
		return trust(args[1:])
	case "library":
		return library(args[1:])
	case "inspect":
		return inspect(args[1:])
	case "config":
		return configCommand(args[1:])
	case "version", "--version":
		fmt.Printf("ghost-node %s (GN protocol v%d)\n", version, node.Version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func add(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "node data directory")
	unsigned := fs.Bool("unsigned", false, "disable signing for this add")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || fs.NArg() != 1 {
		return errors.New("usage: ghost-node add --data-dir DIR [--unsigned] FILE")
	}
	var identity *publication.Identity
	if !*unsigned {
		loaded, err := publication.LoadIdentity(*dir)
		if err == nil {
			identity = &loaded
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	manifest, path, err := publication.NewStore(*dir).AddFileSigned(fs.Arg(0), identity)
	if err != nil {
		return err
	}
	fmt.Printf("publication: %s\nsize: %d bytes\nchunks: %d\nsignature: %s\nstored: %s\n", manifest.ID, manifest.Length, manifest.ChunkCount, nodeSignatureStatus(manifest, publication.NewTrustStore(*dir)), path)
	return nil
}
func want(args []string) error {
	fs := flag.NewFlagSet("want", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "node data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || fs.NArg() != 1 {
		return errors.New("usage: ghost-node want --data-dir DIR ID_OR_PREFIX")
	}
	store := publication.NewStore(*dir)
	id, err := store.Resolve(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := store.Want(id); err != nil {
		return err
	}
	fmt.Printf("wanted: %s\n", id)
	return nil
}

type runOptions struct {
	configPath, device, dir, nodeID, signaturePolicy string
	interval, startup                                time.Duration
	maxAuto                                          byteSizeFlag
	frequency, bandwidth                             float64
	sf, cr, preamble                                 uint
	power                                            int
	sync                                             string
	visited                                          map[string]bool
}

func parseRun(args []string) (runOptions, appconfig.File, error) {
	var o runOptions
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.StringVar(&o.configPath, "config", "", "TOML configuration file")
	fs.StringVar(&o.device, "device", "", "modem serial device")
	fs.StringVar(&o.dir, "data-dir", "", "node data directory")
	fs.StringVar(&o.nodeID, "node-id", "", "hexadecimal node ID")
	fs.DurationVar(&o.interval, "advertise-interval", 0, "inventory summary interval")
	fs.DurationVar(&o.startup, "startup-backoff", 0, "maximum listen-only startup delay")
	fs.Var(&o.maxAuto, "max-auto-size", "maximum automatic object size")
	fs.StringVar(&o.signaturePolicy, "signature-policy", "", "permissive, signed, or trusted")
	fs.Float64Var(&o.frequency, "frequency", 0, "MHz")
	fs.Float64Var(&o.bandwidth, "bandwidth", 0, "kHz")
	fs.UintVar(&o.sf, "sf", 0, "spreading factor")
	fs.UintVar(&o.cr, "cr", 0, "coding rate denominator")
	fs.IntVar(&o.power, "power", 0, "TX power dBm")
	fs.UintVar(&o.preamble, "preamble", 0, "preamble symbols")
	fs.StringVar(&o.sync, "sync", "", "sync word, e.g. 0x12")
	if err := fs.Parse(args); err != nil {
		return o, appconfig.File{}, err
	}
	o.visited = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { o.visited[f.Name] = true })
	var cfg appconfig.File
	if o.configPath != "" {
		loaded, err := appconfig.Decode(o.configPath)
		if err != nil {
			return o, cfg, err
		}
		cfg = loaded
	}
	if !o.visited["device"] {
		o.device = cfg.Radio.Device
	}
	if !o.visited["data-dir"] {
		o.dir = cfg.Node.DataDir
	}
	if !o.visited["node-id"] {
		o.nodeID = cfg.Node.ID
	}
	if !o.visited["advertise-interval"] {
		o.interval = cfg.Discovery.AdvertiseInterval.Duration
	}
	if !o.visited["startup-backoff"] {
		o.startup = cfg.Discovery.StartupBackoff.Duration
	}
	if !o.visited["max-auto-size"] && cfg.Acquisition.MaxAutoSize.Set {
		o.maxAuto = byteSizeFlag{cfg.Acquisition.MaxAutoSize.Value, true}
	}
	if !o.visited["signature-policy"] {
		o.signaturePolicy = cfg.Acquisition.SignaturePolicy
	}
	if o.device == "" || o.dir == "" {
		return o, cfg, errors.New("radio device and data directory are required")
	}
	if o.interval < 0 || (o.visited["advertise-interval"] && o.interval == 0) {
		return o, cfg, errors.New("--advertise-interval must be positive")
	}
	if o.startup < 0 {
		return o, cfg, errors.New("--startup-backoff cannot be negative")
	}
	return o, cfg, nil
}

func runNode(args []string) error {
	o, fileCfg, err := parseRun(args)
	if err != nil {
		return err
	}
	id, err := loadNodeID(o.dir, o.nodeID)
	if err != nil {
		return err
	}
	rng := mathrand.New(mathrand.NewSource(int64(id) ^ time.Now().UnixNano()))
	radio, err := node.OpenRadio(o.device, 115200)
	if err != nil {
		return err
	}
	defer radio.Close()
	radioCfg := fileCfg.RadioConfig()
	configured := fileCfg.RadioConfigured()
	if o.visited["frequency"] {
		radioCfg.FrequencyMHz = float32(o.frequency)
		configured = true
	}
	if o.visited["bandwidth"] {
		radioCfg.BandwidthKHz = float32(o.bandwidth)
		configured = true
	}
	if o.visited["sf"] {
		if o.sf > 255 {
			return errors.New("sf out of range")
		}
		radioCfg.SpreadingFactor = uint8(o.sf)
		configured = true
	}
	if o.visited["cr"] {
		if o.cr > 255 {
			return errors.New("cr out of range")
		}
		radioCfg.CodingRate = uint8(o.cr)
		configured = true
	}
	if o.visited["power"] {
		if o.power < -128 || o.power > 127 {
			return errors.New("power out of range")
		}
		radioCfg.TXPowerDBm = int8(o.power)
		configured = true
	}
	if o.visited["preamble"] {
		if o.preamble > 65535 {
			return errors.New("preamble out of range")
		}
		radioCfg.PreambleSymbols = uint16(o.preamble)
		configured = true
	}
	if o.visited["sync"] {
		value, err := strconv.ParseUint(o.sync, 0, 8)
		if err != nil {
			return err
		}
		radioCfg.SyncWord = uint8(value)
		configured = true
	}
	if err := validateRadio(radioCfg); err != nil {
		return err
	}
	info, err := configureModem(radio, radioCfg, configured)
	if err != nil {
		return err
	}
	fmt.Printf("modem: %s\n", info)
	if configured {
		fmt.Printf("radio configured: frequency=%.3fMHz sf=%d bandwidth=%.1fkHz coding_rate=4/%d tx_power=%ddBm preamble=%d sync=0x%02X\n", radioCfg.FrequencyMHz, radioCfg.SpreadingFactor, radioCfg.BandwidthKHz, radioCfg.CodingRate, radioCfg.TXPowerDBm, radioCfg.PreambleSymbols, radioCfg.SyncWord)
	}
	signaturePolicy, err := node.ParseSignaturePolicy(o.signaturePolicy)
	if err != nil {
		return err
	}
	controller := node.NewController(rng)
	policy := node.AcquisitionPolicy{MaxAutoSize: o.maxAuto.value, HasMaxSize: o.maxAuto.set, Signature: signaturePolicy, Trust: publication.NewTrustStore(o.dir)}
	fmt.Printf("ghost-node id=%08x device=%s data=%s signature_policy=%s\n", uint32(id), o.device, o.dir, defaultString(o.signaturePolicy, "permissive"))
	runtime, err := node.NewRuntime(node.Config{ID: id, Store: publication.NewStore(o.dir), Radio: radio, Controller: controller, AdvertiseInterval: o.interval, StartupBackoff: o.startup, Policy: policy, Log: os.Stdout}, rng, time.Now())
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() { <-signals; close(stop) }()
	return runtime.Run(stop)
}

type modemConfigurator interface {
	Info(time.Duration) (string, error)
	Configure(modem.RadioConfig, time.Duration) (string, error)
}

func configureModem(device modemConfigurator, cfg modem.RadioConfig, configured bool) (string, error) {
	info, err := device.Info(3 * time.Second)
	if err != nil {
		return "", err
	}
	if !configured {
		if strings.Contains(info, "radio: ERROR") {
			return "", errors.New("modem reports radio initialization failure: " + info)
		}
		return info, nil
	}
	status, err := device.Configure(cfg, 5*time.Second)
	if err != nil {
		return "", err
	}
	if err := appconfig.ConfirmRadioStatus(status, cfg); err != nil {
		return "", err
	}
	readback, err := device.Info(3 * time.Second)
	if err != nil {
		return "", err
	}
	if err := appconfig.ConfirmRadioStatus(readback, cfg); err != nil {
		return "", err
	}
	return readback, nil
}

func validateRadio(v modem.RadioConfig) error {
	if v.FrequencyMHz <= 0 || v.BandwidthKHz <= 0 || math.IsNaN(float64(v.FrequencyMHz)) || math.IsInf(float64(v.FrequencyMHz), 0) || math.IsNaN(float64(v.BandwidthKHz)) || math.IsInf(float64(v.BandwidthKHz), 0) {
		return errors.New("frequency and bandwidth must be positive")
	}
	if v.SpreadingFactor < 5 || v.SpreadingFactor > 12 {
		return errors.New("spreading factor must be 5-12")
	}
	if v.CodingRate < 5 || v.CodingRate > 8 {
		return errors.New("coding rate must be 5-8")
	}
	if v.TXPowerDBm < -9 || v.TXPowerDBm > 22 {
		return errors.New("TX power must be -9 through 22")
	}
	if v.PreambleSymbols == 0 {
		return errors.New("preamble must be positive")
	}
	return nil
}

type byteSizeFlag struct {
	value uint64
	set   bool
}

func (f *byteSizeFlag) String() string {
	if !f.set {
		return "unlimited"
	}
	return strconv.FormatUint(f.value, 10)
}
func (f *byteSizeFlag) Set(value string) error {
	size, err := appconfig.ParseSize(value)
	if err != nil {
		return err
	}
	f.value = size
	f.set = true
	return nil
}
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func loadNodeID(dir, value string) (node.NodeID, error) {
	if value != "" {
		parsed, err := strconv.ParseUint(strings.TrimPrefix(value, "0x"), 16, 32)
		if err != nil || parsed == 0 || uint32(parsed) == uint32(node.Broadcast) {
			return 0, errors.New("invalid --node-id")
		}
		return node.NodeID(parsed), nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, err
	}
	path := filepath.Join(dir, "node-id")
	if b, err := os.ReadFile(path); err == nil {
		parsed, err := strconv.ParseUint(strings.TrimSpace(string(b)), 16, 32)
		if err == nil && parsed != 0 && uint32(parsed) != uint32(node.Broadcast) {
			return node.NodeID(parsed), nil
		}
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	id := binary.LittleEndian.Uint32(b[:])
	if id == 0 || id == uint32(node.Broadcast) {
		id = 1
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%08x\n", id)), 0600); err != nil {
		return 0, err
	}
	return node.NodeID(id), nil
}
