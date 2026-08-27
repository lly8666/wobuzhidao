package windowsgui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func TestLoadRuntimeProfileAppliesOnlyProductDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	body := `{
  "server_front": "198.51.100.10:40443",
  "server_name": "front.example",
  "route_key": "0123456789abcdef",
  "username": "solo",
  "password": "shared-password",
  "server_raw": "198.51.100.10:40000",
  "verify_server": false,
  "fec": "20:20"
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "state")
	profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if profile.FEC != "20:20" || profile.IfName != "WBD" || profile.MTU != 1400 || profile.RouteMode != windowsruntime.RouteFull || profile.DNSMode != windowsruntime.DNSAuto || profile.TunnelIPv4 != "10.66.0.2/30" {
		t.Fatalf("unexpected defaults: %+v", profile)
	}
	if !strings.HasSuffix(profile.TicketPath, filepath.Join("state", "reality-ticket.tmp")) ||
		!strings.HasSuffix(profile.RouteState, filepath.Join("state", "route-state.json")) ||
		!strings.HasSuffix(profile.CNSetDir, filepath.Join("state", "ipsets", "cn")) {
		t.Fatalf("WBD-owned paths escaped installed state dir: %+v", profile)
	}
}

func TestLoadRuntimeProfileAcceptsUserRoutingAndDNSPolicyWithoutPathOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	body := `{
  "server_front": "198.51.100.10:40443",
  "server_name": "front.example",
  "route_key": "0123456789abcdef",
  "username": "solo",
  "password": "shared-password",
  "server_raw": "198.51.100.10:40000",
  "route_mode": "Foreign",
  "dns_mode": "Custom",
  "dns_server": "9.9.9.9"
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "state")
	profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if profile.RouteMode != windowsruntime.RouteForeign || profile.DNSMode != windowsruntime.DNSCustom || profile.DNSServer != "9.9.9.9" {
		t.Fatalf("routing policy = %+v", profile)
	}
	if profile.CNSetDir != filepath.Join(stateDir, "ipsets", "cn") {
		t.Fatalf("CNSetDir = %q", profile.CNSetDir)
	}
}

func TestLoadRuntimeProfileRejectsPathInjectionFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	body := `{
  "server_front": "198.51.100.10:40443",
  "server_name": "front.example",
  "route_key": "0123456789abcdef",
  "username": "solo",
  "password": "shared-password",
  "server_raw": "198.51.100.10:40000",
  "bin_dir": "C:\\evil"
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state")); err == nil {
		t.Fatal("unknown path-bearing profile field must be rejected")
	}
}

func TestLoadRuntimeProfileRejectsUnfrozenFEC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	body := `{
  "server_front": "198.51.100.10:40443",
  "server_name": "front.example",
  "route_key": "0123456789abcdef",
  "username": "solo",
  "password": "shared-password",
  "server_raw": "198.51.100.10:40000",
  "fec": "auto"
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state")); err == nil {
		t.Fatal("unfrozen FEC must be rejected")
	}
}
