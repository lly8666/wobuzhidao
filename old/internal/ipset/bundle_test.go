package ipset

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteVerifyAndRestorePreviousCNBundle(t *testing.T) {
	dir := t.TempDir()
	first := []netip.Prefix{netip.MustParsePrefix("1.2.0.0/16"), netip.MustParsePrefix("240e::/20")}
	if _, err := WriteCNBundle(dir, "first", first); err != nil {
		t.Fatal(err)
	}
	m, err := VerifyCNBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.IPv4Count != 1 || m.IPv6Count != 1 || m.Source != "first" {
		t.Fatalf("manifest = %+v", m)
	}

	second := []netip.Prefix{netip.MustParsePrefix("14.0.0.0/8")}
	if _, err := WriteCNBundle(dir, "second", second); err != nil {
		t.Fatal(err)
	}
	m, err = VerifyCNBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.IPv4Count != 1 || m.IPv6Count != 0 || m.Source != "second" {
		t.Fatalf("second manifest = %+v", m)
	}
	if err := RestorePrevious(dir); err != nil {
		t.Fatal(err)
	}
	m, err = VerifyCNBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "first" {
		t.Fatalf("restored source = %q", m.Source)
	}
}

func TestVerifyCNBundleDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteCNBundle(dir, "manual", []netip.Prefix{netip.MustParsePrefix("1.2.0.0/16")}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cn4.txt"), []byte("8.8.8.0/24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyCNBundle(dir); err == nil {
		t.Fatal("tampered bundle unexpectedly verified")
	}
}
