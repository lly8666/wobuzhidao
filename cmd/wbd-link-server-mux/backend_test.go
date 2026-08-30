package main

import (
	"net/netip"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/platformproxy"
)

func TestClassifyServicePayloadRawIP(t *testing.T) {
	packet := make([]byte, 20)
	packet[0] = 0x45
	packet[2] = 0
	packet[3] = 20
	wire, err := dataplane.MarshalIP(packet)
	if err != nil {
		t.Fatal(err)
	}
	got, err := classifyServicePayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got != backendRawIP {
		t.Fatalf("backend=%q want %q", got, backendRawIP)
	}
}

func TestClassifyServicePayloadPlatformProxy(t *testing.T) {
	wire, err := platformproxy.Marshal(platformproxy.Frame{
		Kind:    platformproxy.KindUDPDatagram,
		FlowID:  7,
		Peer:    netip.MustParseAddrPort("1.1.1.1:53"),
		Payload: []byte("dns"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := classifyServicePayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got != backendPlatform {
		t.Fatalf("backend=%q want %q", got, backendPlatform)
	}
}

func TestClassifyServicePayloadRejectsUnknown(t *testing.T) {
	if got, err := classifyServicePayload([]byte("not-a-wbd-frame")); err == nil {
		t.Fatalf("backend=%q, want error", got)
	}
}
