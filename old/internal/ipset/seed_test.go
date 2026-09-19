package ipset

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedCNBaselineInstallsOfflineVerifiedBundle(t *testing.T) {
	dir := t.TempDir()
	m, installed, err := EnsureEmbeddedCNBaseline(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("missing bundle should install embedded baseline")
	}
	if m.Source != EmbeddedCNSource {
		t.Fatalf("source=%q want=%q", m.Source, EmbeddedCNSource)
	}
	if m.IPv4Count < 5000 {
		t.Fatalf("embedded IPv4 baseline unexpectedly small: %d", m.IPv4Count)
	}
	if _, err := VerifyCNBundle(dir); err != nil {
		t.Fatalf("installed baseline does not verify: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, CNIPv4File))
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []string{"1.0.1.0/24", "1.1.0.0/24"} {
		if !strings.Contains(string(raw), sample) {
			t.Fatalf("embedded baseline missing representative prefix %s", sample)
		}
	}
}

func TestEmbeddedCNBaselinePreservesExistingVerifiedBundle(t *testing.T) {
	dir := t.TempDir()
	prefixes := []netip.Prefix{netip.MustParsePrefix("1.2.3.0/24")}
	if _, err := WriteCNBundle(dir, "manual-test", prefixes); err != nil {
		t.Fatal(err)
	}
	m, installed, err := EnsureEmbeddedCNBaseline(dir)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("verified existing bundle must not be replaced by embedded baseline")
	}
	if m.Source != "manual-test" || m.IPv4Count != 1 {
		t.Fatalf("existing bundle changed: %+v", m)
	}
}
