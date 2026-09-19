package rawipbackend

import (
	"net/netip"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func TestSessionMetaRoundTrip(t *testing.T) {
	wire, err := MarshalSessionMeta("7c31a9")
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != MetaLen {
		t.Fatalf("len=%d want=%d", len(wire), MetaLen)
	}
	got, ok := UnmarshalSessionMeta(wire)
	if !ok || got.SID != "7c31a9" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestSessionMetaRejectsInvalidSID(t *testing.T) {
	for _, sid := range []string{"", "123", "zzzzzz", "12345678"} {
		if _, err := MarshalSessionMeta(sid); err == nil {
			t.Fatalf("MarshalSessionMeta(%q) unexpectedly succeeded", sid)
		}
	}
}

func TestTunnelMetaRoundTrip(t *testing.T) {
	id, err := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	addr := netip.MustParseAddr("10.66.0.17")
	wire, err := MarshalTunnelMeta(id, addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != TunnelMetaLen {
		t.Fatalf("len=%d want=%d", len(wire), TunnelMetaLen)
	}
	got, ok := UnmarshalTunnelMeta(wire)
	if !ok || got.TunnelID != id || got.Address4 != addr {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
	if _, ok := UnmarshalSessionMeta(wire); ok {
		t.Fatal("v2 tunnel metadata was misclassified as historical v1 SID metadata")
	}
}
