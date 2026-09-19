package windowsgui

import (
	"reflect"
	"testing"
)

func boolp(v bool) *bool { return &v }
func intp(v int) *int    { return &v }

func TestProfileStoreRoundTripsAllUserConfigurationFields(t *testing.T) {
	cfg := RuntimeProfileFile{
		ServerIP: "198.51.100.42", ServerPort: 40443, ServerName: "front.example", RouteKey: "0123456789abcdef",
		Username: "alice", Password: "secret", VerifyServer: true, FEC: "20:20", IfName: "WBD-CN", MTU: 1320,
		RouteMode: "Foreign", ProxyLAN: boolp(true), ProxyChina: boolp(false), ProxyOther: boolp(true),
		DNSMode: "Custom", DNSServer: "9.9.9.9", Lanes: 4, IdleTimeout: intp(123), KeepaliveSeconds: intp(0),
		LaneRotationMinSeconds: intp(60), LaneRotationMaxSeconds: intp(120), TunnelIPv4: "10.66.0.2/30",
	}
	store := NewProfileStore()
	p, err := store.Add("东京 1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	store.SelectedID = p.ID
	path := t.TempDir() + "/profiles.json"
	if err := SaveProfileStore(path, store); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProfileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := got.Selected()
	if !ok {
		t.Fatal("selected profile lost")
	}
	if selected.Name != "东京 1" || !reflect.DeepEqual(selected.Config, cfg) {
		t.Fatalf("round trip=%+v", selected)
	}
}

func TestProfileStoreSwitchDeleteAndDefaultSelection(t *testing.T) {
	s := NewProfileStore()
	a, _ := s.Add("A", RuntimeProfileFile{ServerIP: "192.0.2.1"})
	b, _ := s.Add("B", RuntimeProfileFile{ServerIP: "192.0.2.2"})
	if !s.Select(b.ID) {
		t.Fatal("select B failed")
	}
	if p, _ := s.Selected(); p.ID != b.ID {
		t.Fatalf("selected=%s", p.ID)
	}
	if !s.Delete(b.ID) {
		t.Fatal("delete B failed")
	}
	if p, ok := s.Selected(); !ok || p.ID != a.ID {
		t.Fatalf("fallback selected=%+v ok=%v", p, ok)
	}
}

func TestRuntimeProfileFileReadWritePreservesPointers(t *testing.T) {
	cfg := RuntimeProfileFile{ServerIP: "203.0.113.9", ServerPort: 40443, ProxyLAN: boolp(false), ProxyChina: boolp(true), ProxyOther: boolp(false), KeepaliveSeconds: intp(0)}
	path := t.TempDir() + "/wbd.json"
	if err := WriteRuntimeProfileFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRuntimeProfileFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("got=%+v want=%+v", got, cfg)
	}
}
