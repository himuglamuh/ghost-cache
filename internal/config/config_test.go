package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAndRadioConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `[node]
data_dir="/var/lib/ghostcache"
[radio]
device="/dev/ttyUSB0"
frequency_mhz=914.5
spreading_factor=8
bandwidth_khz=250.0
coding_rate=6
tx_power_dbm=4
preamble=10
sync_word=0x34
[discovery]
advertise_interval="2m"
[acquisition]
max_auto_size="256KiB"
signature_policy="trusted"
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Discovery.AdvertiseInterval.Duration != 2*time.Minute || cfg.Acquisition.MaxAutoSize.Value != 256*1024 {
		t.Fatal("parsed values differ")
	}
	radio := cfg.RadioConfig()
	if radio.FrequencyMHz != 914.5 || radio.SpreadingFactor != 8 || radio.SyncWord != 0x34 {
		t.Fatalf("radio=%+v", radio)
	}
}

func TestOptionalRadioAndInvalidInputs(t *testing.T) {
	valid := filepath.Join(t.TempDir(), "valid.toml")
	if err := os.WriteFile(valid, []byte("[node]\ndata_dir='x'\n[radio]\ndevice='d'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(valid)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RadioConfigured() {
		t.Fatal("optional radio considered configured")
	}
	cases := []string{"[node]\ndata_dir='x'\n[radio]\ndevice='d'\nspreading_factor=13\n", "[node]\ndata_dir='x'\n[radio]\ndevice='d'\ncoding_rate=4\n", "[node]\ndata_dir='x'\n[radio]\ndevice='d'\n[acquisition]\nsignature_policy='bad'\n"}
	for _, data := range cases {
		path := filepath.Join(t.TempDir(), "bad.toml")
		os.WriteFile(path, []byte(data), 0600)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}

func TestSizeDurationAndConfirmation(t *testing.T) {
	size, err := ParseSize("32KiB")
	if err != nil || size != 32768 {
		t.Fatalf("size=%d err=%v", size, err)
	}
	if _, err := ParseSize("bad"); err == nil {
		t.Fatal("accepted bad size")
	}
	cfg := File{}
	radio := cfg.RadioConfig()
	status := "frequency=915.000MHz sf=7 bw=125.0kHz cr=4/5 power=5dBm preamble=8 sync=0x12"
	if err := ConfirmRadioStatus(status, radio); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmRadioStatus("wrong", radio); err == nil {
		t.Fatal("accepted unconfirmed config")
	}
	if err := ConfirmRadioStatus("frequency=915.000MHz sf=70 bw=125.0kHz cr=4/5 power=5dBm preamble=80 sync=0x123", radio); err == nil {
		t.Fatal("accepted substring-only readback")
	}
}

func TestWriteSamplePreservesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSample(path, "/data", "/dev/test", false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := WriteSample(path, "other", "other", false); err == nil {
		t.Fatal("overwrote config")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("config changed")
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[node]\ndata_dir='x'\n[radio]\ndevice='d'\nbandwith_khz=125\n"), 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown key accepted")
	}
}
