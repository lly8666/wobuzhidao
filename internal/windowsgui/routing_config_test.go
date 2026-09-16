package windowsgui

import (
	"path/filepath"
	"testing"
)

func TestLoadRuntimeProfileAcceptsExplicitRoutingPolicy(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir()
	path := writeTestProfile(t, dir, `,
  "proxy_lan": true,
  "proxy_china": false,
  "proxy_other": true`)
	profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.RoutingPolicy == nil {
		t.Fatal("explicit routing policy missing")
	}
	got := *profile.RoutingPolicy
	if !got.ProxyLAN || got.ProxyChina || !got.ProxyOther {
		t.Fatalf("routing policy=%+v", got)
	}
	if !profile.AutoUpdateCN {
		t.Fatal("Windows product profile must enable automatic CN bundle refresh")
	}
}

func TestLoadRuntimeProfileRejectsPartialRoutingPolicy(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir()
	path := writeTestProfile(t, dir, `,
  "proxy_lan": true,
  "proxy_other": true`)
	if _, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state")); err == nil {
		t.Fatal("partial routing policy unexpectedly accepted")
	}
}

func TestLoadRuntimeProfileRejectsMixedLegacyAndExplicitRoutingPolicy(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir()
	path := writeTestProfile(t, dir, `,
  "route_mode": "Foreign",
  "proxy_lan": false,
  "proxy_china": false,
  "proxy_other": true`)
	if _, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state")); err == nil {
		t.Fatal("mixed legacy/new routing policy unexpectedly accepted")
	}
}
