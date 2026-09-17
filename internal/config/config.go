package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/himuglamuh/ghost-cache/internal/modem"
	"github.com/himuglamuh/ghost-cache/internal/node"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	value, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = value
	return nil
}

type ByteSize struct {
	Value uint64
	Set   bool
}

func (s *ByteSize) UnmarshalText(text []byte) error {
	value, err := ParseSize(string(text))
	if err != nil {
		return err
	}
	s.Value = value
	s.Set = true
	return nil
}

type File struct {
	Node struct {
		DataDir string `toml:"data_dir"`
		ID      string `toml:"id"`
	} `toml:"node"`
	Radio struct {
		Device          string   `toml:"device"`
		FrequencyMHz    *float32 `toml:"frequency_mhz"`
		SpreadingFactor *uint8   `toml:"spreading_factor"`
		BandwidthKHz    *float32 `toml:"bandwidth_khz"`
		CodingRate      *uint8   `toml:"coding_rate"`
		TXPowerDBm      *int8    `toml:"tx_power_dbm"`
		Preamble        *uint16  `toml:"preamble"`
		SyncWord        *uint8   `toml:"sync_word"`
	} `toml:"radio"`
	Discovery struct {
		AdvertiseInterval Duration `toml:"advertise_interval"`
		StartupBackoff    Duration `toml:"startup_backoff"`
	} `toml:"discovery"`
	Acquisition struct {
		MaxAutoSize     ByteSize `toml:"max_auto_size"`
		SignaturePolicy string   `toml:"signature_policy"`
	} `toml:"acquisition"`
}

func Decode(path string) (File, error) {
	var cfg File
	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, err
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("unknown configuration key %s", undecoded[0])
	}
	return cfg, nil
}
func Load(path string) (File, error) {
	cfg, err := Decode(path)
	if err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}
func (c File) Validate() error {
	if c.Node.DataDir == "" {
		return errors.New("node.data_dir is required")
	}
	if c.Radio.Device == "" {
		return errors.New("radio.device is required")
	}
	if c.Radio.FrequencyMHz != nil && (*c.Radio.FrequencyMHz <= 0 || math.IsNaN(float64(*c.Radio.FrequencyMHz)) || math.IsInf(float64(*c.Radio.FrequencyMHz), 0)) {
		return errors.New("radio.frequency_mhz must be positive")
	}
	if c.Radio.BandwidthKHz != nil && (*c.Radio.BandwidthKHz <= 0 || math.IsNaN(float64(*c.Radio.BandwidthKHz)) || math.IsInf(float64(*c.Radio.BandwidthKHz), 0)) {
		return errors.New("radio.bandwidth_khz must be positive")
	}
	if c.Radio.SpreadingFactor != nil && (*c.Radio.SpreadingFactor < 5 || *c.Radio.SpreadingFactor > 12) {
		return errors.New("radio.spreading_factor must be 5-12")
	}
	if c.Radio.CodingRate != nil && (*c.Radio.CodingRate < 5 || *c.Radio.CodingRate > 8) {
		return errors.New("radio.coding_rate must be 5-8")
	}
	if c.Radio.TXPowerDBm != nil && (*c.Radio.TXPowerDBm < -9 || *c.Radio.TXPowerDBm > 22) {
		return errors.New("radio.tx_power_dbm must be -9 through 22")
	}
	if c.Radio.Preamble != nil && *c.Radio.Preamble == 0 {
		return errors.New("radio.preamble must be positive")
	}
	if c.Discovery.AdvertiseInterval.Duration < 0 || c.Discovery.StartupBackoff.Duration < 0 {
		return errors.New("discovery durations cannot be negative")
	}
	_, err := node.ParseSignaturePolicy(c.Acquisition.SignaturePolicy)
	return err
}
func (c File) RadioConfigured() bool {
	return c.Radio.FrequencyMHz != nil || c.Radio.SpreadingFactor != nil || c.Radio.BandwidthKHz != nil || c.Radio.CodingRate != nil || c.Radio.TXPowerDBm != nil || c.Radio.Preamble != nil || c.Radio.SyncWord != nil
}
func (c File) RadioConfig() modem.RadioConfig {
	v := modem.DefaultConfig
	if c.Radio.FrequencyMHz != nil {
		v.FrequencyMHz = *c.Radio.FrequencyMHz
	}
	if c.Radio.BandwidthKHz != nil {
		v.BandwidthKHz = *c.Radio.BandwidthKHz
	}
	if c.Radio.SpreadingFactor != nil {
		v.SpreadingFactor = *c.Radio.SpreadingFactor
	}
	if c.Radio.CodingRate != nil {
		v.CodingRate = *c.Radio.CodingRate
	}
	if c.Radio.TXPowerDBm != nil {
		v.TXPowerDBm = *c.Radio.TXPowerDBm
	}
	if c.Radio.Preamble != nil {
		v.PreambleSymbols = *c.Radio.Preamble
	}
	if c.Radio.SyncWord != nil {
		v.SyncWord = *c.Radio.SyncWord
	}
	return v
}
func ConfirmRadioStatus(status string, v modem.RadioConfig) error {
	fields := strings.FieldsFunc(status, func(r rune) bool { return r == ' ' || r == ';' })
	actual := map[string]string{}
	for _, field := range fields {
		if i := strings.IndexByte(field, '='); i > 0 {
			actual[field[:i]] = field[i+1:]
		}
	}
	expected := map[string]string{"frequency": fmt.Sprintf("%.3fMHz", v.FrequencyMHz), "sf": fmt.Sprintf("%d", v.SpreadingFactor), "bw": fmt.Sprintf("%.1fkHz", v.BandwidthKHz), "cr": fmt.Sprintf("4/%d", v.CodingRate), "power": fmt.Sprintf("%ddBm", v.TXPowerDBm), "preamble": fmt.Sprintf("%d", v.PreambleSymbols), "sync": fmt.Sprintf("0x%02X", v.SyncWord)}
	for key, want := range expected {
		if actual[key] != want {
			return fmt.Errorf("modem status did not confirm %s=%s: %s", key, want, status)
		}
	}
	return nil
}
func ParseSize(value string) (uint64, error) {
	multipliers := map[string]uint64{"": 1, "B": 1, "KIB": 1024, "MIB": 1024 * 1024, "GIB": 1024 * 1024 * 1024, "KB": 1000, "MB": 1000000, "GB": 1000000000}
	upper := strings.ToUpper(strings.TrimSpace(value))
	suffix := ""
	for _, candidate := range []string{"KIB", "MIB", "GIB", "KB", "MB", "GB", "B"} {
		if strings.HasSuffix(upper, candidate) {
			suffix = candidate
			upper = strings.TrimSpace(strings.TrimSuffix(upper, candidate))
			break
		}
	}
	number, err := strconv.ParseUint(upper, 10, 64)
	if err != nil {
		return 0, err
	}
	m := multipliers[suffix]
	if number > ^uint64(0)/m {
		return 0, errors.New("size overflows")
	}
	return number * m, nil
}
func Sample(dataDir, device string) string {
	return fmt.Sprintf("[node]\ndata_dir = %q\n\n[radio]\ndevice = %q\nfrequency_mhz = 915.0\nspreading_factor = 7\nbandwidth_khz = 125.0\ncoding_rate = 5\ntx_power_dbm = 5\npreamble = 8\nsync_word = 0x12\n\n[discovery]\nadvertise_interval = \"10m\"\nstartup_backoff = \"5s\"\n\n[acquisition]\n# max_auto_size = \"128KiB\"\nsignature_policy = \"permissive\"\n", dataDir, device)
}
func WriteSample(path, dataDir, device string, replace bool) error {
	if !replace {
		if _, err := os.Stat(path); err == nil {
			return errors.New("config already exists")
		}
	}
	return os.WriteFile(path, []byte(Sample(dataDir, device)), 0640)
}
