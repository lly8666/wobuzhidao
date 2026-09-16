package windowsgui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func intPtrSwitch(v int) *int    { return &v }
func boolPtrSwitch(v bool) *bool { return &v }

func modernRouting(lan, china, other bool) (*bool, *bool, *bool) {
	return boolPtrSwitch(lan), boolPtrSwitch(china), boolPtrSwitch(other)
}

func TestAllFixedFECProfilesRoundTripThroughEditorAndRuntime(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	profiles := []string{"off", "20:4", "20:8", "20:10", "20:12", "20:16", "20:20"}
	for i, fec := range profiles {
		t.Run(strings.ReplaceAll(fec, ":", "_"), func(t *testing.T) {
			lan, china, other := modernRouting(false, true, true)
			cfg := RuntimeProfileFile{
				ServerIP: "198.51.100.10", ServerPort: 40443,
				ServerName: "front.example", RouteKey: "0123456789abcdef",
				Username: "solo", Password: "shared-password", FEC: fec,
				IfName: "WBD", MTU: 1420,
				ProxyLAN: lan, ProxyChina: china, ProxyOther: other,
				DNSMode: windowsruntime.DNSSystem, Lanes: 1,
			}
			saved := SavedProfile{ID: fmt.Sprintf("id-%d", i), Name: "FEC " + fec, Config: cfg}
			values := EditorValuesFromSavedProfile(saved)
			if values.FEC != fec {
				t.Fatalf("editor FEC=%q want=%q", values.FEC, fec)
			}
			edited, err := values.ApplyToSavedProfile(saved)
			if err != nil {
				t.Fatal(err)
			}
			if edited.Config.FEC != fec {
				t.Fatalf("edited FEC=%q want=%q", edited.Config.FEC, fec)
			}
			if edited.Config.RouteMode != "" {
				t.Fatalf("modern GUI emitted legacy route_mode=%q", edited.Config.RouteMode)
			}
			dir := t.TempDir()
			profilePath := filepath.Join(dir, "active.json")
			if err := WriteRuntimeProfileFile(profilePath, edited.Config); err != nil {
				t.Fatal(err)
			}
			runtimeProfile, err := LoadRuntimeProfile(profilePath, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			if runtimeProfile.FEC != fec {
				t.Fatalf("runtime FEC=%q want=%q", runtimeProfile.FEC, fec)
			}
		})
	}
}

func TestProfileEditorMigratesLegacyRouteModeToExplicitPolicy(t *testing.T) {
	cases := []struct {
		mode string
		want windowsruntime.RoutingPolicy
	}{
		{windowsruntime.RouteFull, windowsruntime.RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: true}},
		{windowsruntime.RouteForeign, windowsruntime.RoutingPolicy{ProxyLAN: false, ProxyChina: false, ProxyOther: true}},
		{windowsruntime.RouteChina, windowsruntime.RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: false}},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			saved := SavedProfile{ID: "legacy", Name: "旧配置", Config: RuntimeProfileFile{
				ServerIP: "198.51.100.30", ServerPort: 443, ServerName: "legacy.example",
				RouteKey: "0123456789abcdef", Username: "legacy", Password: "legacy-password",
				FEC: "off", IfName: "WBD", MTU: 1420, RouteMode: tc.mode,
				DNSMode: windowsruntime.DNSSystem, Lanes: 1,
			}}
			values := EditorValuesFromSavedProfile(saved)
			if values.ProxyLAN != tc.want.ProxyLAN || values.ProxyChina != tc.want.ProxyChina || values.ProxyOther != tc.want.ProxyOther {
				t.Fatalf("legacy %s displayed policy=(%v,%v,%v) want=%+v", tc.mode, values.ProxyLAN, values.ProxyChina, values.ProxyOther, tc.want)
			}
			edited, err := values.ApplyToSavedProfile(saved)
			if err != nil {
				t.Fatal(err)
			}
			if edited.Config.RouteMode != "" {
				t.Fatalf("legacy route_mode not cleared: %q", edited.Config.RouteMode)
			}
			if edited.Config.ProxyLAN == nil || edited.Config.ProxyChina == nil || edited.Config.ProxyOther == nil {
				t.Fatal("explicit policy missing after migration")
			}
			dir := t.TempDir()
			profilePath := filepath.Join(dir, "active.json")
			if err := WriteRuntimeProfileFile(profilePath, edited.Config); err != nil {
				t.Fatal(err)
			}
			got, err := LoadRuntimeProfile(profilePath, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.EffectiveRoutingPolicy(), tc.want) {
				t.Fatalf("effective policy=%+v want=%+v", got.EffectiveRoutingPolicy(), tc.want)
			}
		})
	}
}

func TestProfileStoreSwitchesServersAndAppliesSettingsWithoutBleed(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir()
	storePath := filepath.Join(dir, "profiles.json")
	activePath := filepath.Join(dir, "active-profile.json")
	binDir := filepath.Join(dir, "bin")
	stateDir := filepath.Join(dir, "state")

	aLAN, aChina, aOther := modernRouting(false, true, true)
	bLAN, bChina, bOther := modernRouting(true, false, true)
	a := RuntimeProfileFile{
		ServerIP: "198.51.100.11", ServerPort: 41001,
		ServerName: "a.example", RouteKey: "0123456789abcdef",
		Username: "alice", Password: "password-a", VerifyServer: false,
		FEC: "20:4", IfName: "WBD-A", MTU: 1280,
		ProxyLAN: aLAN, ProxyChina: aChina, ProxyOther: aOther,
		DNSMode: windowsruntime.DNSSystem, Lanes: 1,
		IdleTimeout: intPtrSwitch(900), KeepaliveSeconds: intPtrSwitch(0),
		LaneRotationMinSeconds: intPtrSwitch(90), LaneRotationMaxSeconds: intPtrSwitch(90),
	}
	b := RuntimeProfileFile{
		ServerIP: "198.51.100.22", ServerPort: 42002,
		ServerName: "b.example", RouteKey: "fedcba9876543210",
		Username: "bob", Password: "password-b", VerifyServer: true,
		FEC: "20:16", IfName: "WBD-B", MTU: 1400,
		ProxyLAN: bLAN, ProxyChina: bChina, ProxyOther: bOther,
		DNSMode: windowsruntime.DNSCustom, DNSServer: "9.9.9.9", Lanes: 4,
		IdleTimeout: intPtrSwitch(1200), KeepaliveSeconds: intPtrSwitch(15),
		LaneRotationMinSeconds: intPtrSwitch(120), LaneRotationMaxSeconds: intPtrSwitch(240),
	}

	store := NewProfileStore()
	pa, err := store.Add("东京-A", a)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := store.Add("东京-B", b)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Select(pa.ID) {
		t.Fatal("select A failed")
	}
	if err := SaveProfileStore(storePath, store); err != nil {
		t.Fatal(err)
	}

	activate := func(id string) windowsruntime.Profile {
		t.Helper()
		loaded, err := LoadProfileStore(storePath)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded.Select(id) {
			t.Fatalf("select %q failed", id)
		}
		if err := SaveProfileStore(storePath, loaded); err != nil {
			t.Fatal(err)
		}
		persisted, err := LoadProfileStore(storePath)
		if err != nil {
			t.Fatal(err)
		}
		selected, ok := persisted.Selected()
		if !ok {
			t.Fatal("selected profile missing")
		}
		if err := WriteRuntimeProfileFile(activePath, selected.Config); err != nil {
			t.Fatal(err)
		}
		runtimeProfile, err := LoadRuntimeProfile(activePath, binDir, stateDir)
		if err != nil {
			t.Fatal(err)
		}
		return runtimeProfile
	}

	assertRuntimeProfile := func(label string, got windowsruntime.Profile, cfg RuntimeProfileFile) {
		t.Helper()
		endpoint := fmt.Sprintf("%s:%d", cfg.ServerIP, cfg.ServerPort)
		if got.ServerRaw != endpoint || got.ServerFront != endpoint || got.ServerName != cfg.ServerName || got.Username != cfg.Username || got.Password != cfg.Password || got.VerifyServer != cfg.VerifyServer {
			t.Fatalf("%s endpoint/auth mismatch: %+v", label, got)
		}
		if got.FEC != cfg.FEC || got.IfName != cfg.IfName || got.MTU != cfg.MTU || got.DNSMode != cfg.DNSMode || got.DNSServer != cfg.DNSServer || got.Lanes != cfg.Lanes || got.IdleTimeoutSeconds != *cfg.IdleTimeout || got.LaneRotationMinSeconds != *cfg.LaneRotationMinSeconds || got.LaneRotationMaxSeconds != *cfg.LaneRotationMaxSeconds {
			t.Fatalf("%s runtime settings mismatch: %+v", label, got)
		}
		if got.KeepaliveSeconds == nil || *got.KeepaliveSeconds != *cfg.KeepaliveSeconds {
			t.Fatalf("%s keepalive mismatch: %+v", label, got.KeepaliveSeconds)
		}
		wantPolicy := windowsruntime.RoutingPolicy{ProxyLAN: *cfg.ProxyLAN, ProxyChina: *cfg.ProxyChina, ProxyOther: *cfg.ProxyOther}
		if !reflect.DeepEqual(got.EffectiveRoutingPolicy(), wantPolicy) {
			t.Fatalf("%s routing policy=%+v want=%+v", label, got.EffectiveRoutingPolicy(), wantPolicy)
		}
	}

	gotA := activate(pa.ID)
	assertRuntimeProfile("A", gotA, a)
	gotB := activate(pb.ID)
	assertRuntimeProfile("B", gotB, b)
	if gotB.ServerRaw == gotA.ServerRaw || gotB.FEC == gotA.FEC || gotB.IfName == gotA.IfName || gotB.MTU == gotA.MTU || gotB.Lanes == gotA.Lanes {
		t.Fatalf("server B retained A settings: A=%+v B=%+v", gotA, gotB)
	}
	gotA2 := activate(pa.ID)
	assertRuntimeProfile("A-again", gotA2, a)
	if gotA2.ServerRaw != gotA.ServerRaw || gotA2.FEC != gotA.FEC || gotA2.MTU != gotA.MTU || gotA2.Lanes != gotA.Lanes {
		t.Fatalf("switching back to A did not restore A: first=%+v again=%+v", gotA, gotA2)
	}
}
