package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/modem"
)

func TestValidateRadioRejectsNonFinite(t *testing.T) {
	cfg := modem.DefaultConfig
	cfg.FrequencyMHz = float32(math.NaN())
	if err := validateRadio(cfg); err == nil {
		t.Fatal("accepted NaN frequency")
	}
	cfg = modem.DefaultConfig
	cfg.BandwidthKHz = float32(math.Inf(1))
	if err := validateRadio(cfg); err == nil {
		t.Fatal("accepted infinite bandwidth")
	}
}

type fakeConfigurator struct {
	status       string
	configureErr error
	configured   bool
}

func (f *fakeConfigurator) Info(time.Duration) (string, error) { return f.status, nil }
func (f *fakeConfigurator) Configure(modem.RadioConfig, time.Duration) (string, error) {
	f.configured = true
	return f.status, f.configureErr
}

func TestConfigureModemApplyAndFailure(t *testing.T) {
	cfg := modem.DefaultConfig
	status := "frequency=915.000MHz sf=7 bw=125.0kHz cr=4/5 power=5dBm preamble=8 sync=0x12"
	fake := &fakeConfigurator{status: status}
	if _, err := configureModem(fake, cfg, true); err != nil || !fake.configured {
		t.Fatalf("configured=%v err=%v", fake.configured, err)
	}
	fake = &fakeConfigurator{status: status, configureErr: errors.New("rejected")}
	if _, err := configureModem(fake, cfg, true); err == nil {
		t.Fatal("configuration failure ignored")
	}
	fake = &fakeConfigurator{status: "mismatch"}
	if _, err := configureModem(fake, cfg, true); err == nil {
		t.Fatal("readback mismatch ignored")
	}
}

func TestRunConfigCLIPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "[node]\ndata_dir='/from-file'\n[radio]\ndevice='/dev/file'\n[discovery]\nadvertise_interval='10m'\n[acquisition]\nmax_auto_size='1KiB'\nsignature_policy='signed'\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	options, _, err := parseRun([]string{"--config", path, "--device", "/dev/cli", "--advertise-interval", "5s", "--max-auto-size", "2KiB", "--signature-policy", "trusted"})
	if err != nil {
		t.Fatal(err)
	}
	if options.device != "/dev/cli" || options.dir != "/from-file" || options.interval != 5*time.Second || options.maxAuto.value != 2048 || options.signaturePolicy != "trusted" {
		t.Fatalf("options=%+v", options)
	}
}

func TestRunRejectsNegativeTOMLDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "[node]\ndata_dir='/data'\n[radio]\ndevice='/dev/test'\n[discovery]\nadvertise_interval='-1s'\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseRun([]string{"--config", path}); err == nil {
		t.Fatal("negative TOML duration accepted")
	}
}
