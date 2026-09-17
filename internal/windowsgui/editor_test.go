package windowsgui

import (
	"reflect"
	"testing"
)

func TestProfileEditorRoundTripsEffectiveConfigFieldsAndExplicitZero(t *testing.T) {
	z := 0
	idle := 77
	a := true
	b := false
	p := SavedProfile{ID: "id", Name: "东京", Config: RuntimeProfileFile{ServerIP: "198.51.100.8", ServerPort: 443, ServerFront: "", ServerRaw: "", ServerName: "sni", RouteKey: "rk", Username: "u", Password: "p", VerifyServer: true, FEC: "20:10", IfName: "WBD-X", MTU: 1350, RouteMode: "", ProxyLAN: &a, ProxyChina: &b, ProxyOther: &a, DNSMode: "Custom", DNSServer: "9.9.9.9", Lanes: 4, IdleTimeout: &idle, KeepaliveSeconds: &z, LaneRotationMinSeconds: &idle, LaneRotationMaxSeconds: &z}}
	v := EditorValuesFromSavedProfile(p)
	if v.Keepalive != "0" || v.RotationMax != "0" {
		t.Fatalf("explicit zero lost: %+v", v)
	}
	got, err := v.ApplyToSavedProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Fatalf("roundtrip\n got=%+v\nwant=%+v", got, p)
	}
}

func TestProfileEditorMigratesLegacyEndpointWhenNewEndpointIsEntered(t *testing.T) {
	p := SavedProfile{ID: "id", Name: "legacy", Config: RuntimeProfileFile{
		ServerFront: "198.51.100.7:443",
		ServerRaw:   "198.51.100.7:443",
		TunnelIPv4: "10.66.0.2/32",
	}}
	v := EditorValuesFromSavedProfile(p)
	v.Name = "migrated"
	v.ServerIP = "203.0.113.9"
	v.ServerPort = "8443"
	got, err := v.ApplyToSavedProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.ServerIP != "203.0.113.9" || got.Config.ServerPort != 8443 {
		t.Fatalf("new endpoint not saved: %+v", got.Config)
	}
	if got.Config.ServerFront != "" || got.Config.ServerRaw != "" {
		t.Fatalf("legacy endpoint survived migration: %+v", got.Config)
	}
	if got.Config.TunnelIPv4 != "" {
		t.Fatalf("server-assigned tunnel address was written back: %+v", got.Config)
	}
}

func TestProfileEditorPreservesLegacyEndpointUntilCurrentEndpointIsSupplied(t *testing.T) {
	p := SavedProfile{ID: "id", Name: "legacy", Config: RuntimeProfileFile{ServerFront: "198.51.100.7:443", ServerRaw: "198.51.100.7:443"}}
	v := EditorValuesFromSavedProfile(p)
	got, err := v.ApplyToSavedProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.ServerFront != p.Config.ServerFront || got.Config.ServerRaw != p.Config.ServerRaw {
		t.Fatalf("legacy endpoint changed before migration: got=%+v want=%+v", got.Config, p.Config)
	}
}

func TestProfileEditorBlankOptionalIntsRemainNil(t *testing.T) {
	p := SavedProfile{ID: "id", Name: "A"}
	v := EditorValuesFromSavedProfile(p)
	v.Name = "A"
	got, err := v.ApplyToSavedProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.IdleTimeout != nil || got.Config.KeepaliveSeconds != nil || got.Config.LaneRotationMinSeconds != nil || got.Config.LaneRotationMaxSeconds != nil {
		t.Fatalf("blank optional fields became explicit values: %+v", got.Config)
	}
}
