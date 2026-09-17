package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestServiceIsNonRootAndVolatile(t *testing.T) {
	b, err := os.ReadFile("../../deploy/ghost-node.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(b)
	for _, required := range []string{"User=ghostcache", "ExecStart=/usr/local/bin/ghost-node run --config /etc/ghostcache/config.toml", "RuntimeDirectory=ghostcache", "StandardOutput=journal", "StandardError=journal", "LogNamespace=ghostcache"} {
		if !strings.Contains(unit, required) {
			t.Fatalf("unit missing %q", required)
		}
	}
	if strings.Contains(unit, "network.target") {
		t.Fatal("service unnecessarily depends on networking")
	}
}

func TestJournalNamespaceIsBoundedAndVolatile(t *testing.T) {
	b, err := os.ReadFile("../../deploy/journald-ghostcache.conf")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, required := range []string{"Storage=volatile", "RuntimeMaxUse=16M", "RuntimeMaxFileSize=4M"} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing %s", required)
		}
	}
}
func TestUninstallPreservesDataByDefault(t *testing.T) {
	b, err := os.ReadFile("../../scripts/uninstall-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	if !strings.Contains(script, "preserved /var/lib/ghostcache") || !strings.Contains(script, "--remove-data") {
		t.Fatal("uninstall preservation contract missing")
	}
}

func TestInstallerValidatesCandidateBeforeInstall(t *testing.T) {
	b, err := os.ReadFile("../../scripts/install-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	validate := strings.Index(script, `"$candidate_binary" config check`)
	install := strings.Index(script, `install -m 0755 "$candidate_binary" /usr/local/bin/ghost-node`)
	if validate < 0 || install < 0 || validate > install {
		t.Fatal("candidate is not validated before installation")
	}
}

func TestInstallerHasRollbackTrap(t *testing.T) {
	b, err := os.ReadFile("../../scripts/install-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	for _, required := range []string{"rollback()", "trap rollback EXIT", "committed=1", "systemctl daemon-reload || true", "was_enabled", "was_active", "enabled-runtime", `usermod -G "$old_groups"`} {
		if !strings.Contains(script, required) {
			t.Fatalf("installer rollback missing %q", required)
		}
	}
	if strings.Index(script, "trap rollback EXIT") < strings.Index(script, "config check") {
		t.Fatal("rollback trap is active during candidate preflight")
	}
}

func TestInstallerRestrictsSerialGroupGrant(t *testing.T) {
	b, err := os.ReadFile("../../scripts/install-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	for _, required := range []string{`-c "$device"`, "dialout|uucp", "configured device is not a character device", "unrecognized serial group"} {
		if !strings.Contains(script, required) {
			t.Fatalf("serial permission guard missing %q", required)
		}
	}
}

func TestUninstallRemovesOnlyOwnedJournalDropIn(t *testing.T) {
	b, err := os.ReadFile("../../scripts/uninstall-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	if !strings.Contains(script, "journald@ghostcache.conf.d/volatile.conf") {
		t.Fatal("owned drop-in is not removed")
	}
	if strings.Contains(script, "rm -rf /etc/systemd/journald@ghostcache.conf.d") {
		t.Fatal("uninstaller removes administrator drop-ins")
	}
}
