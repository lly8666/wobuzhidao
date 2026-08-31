package main

import (
	"net/netip"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/platformproxy"
)

func TestClassifyServicePayloadRawIP(t *testing.T) {
	packet := make([]byte, 20)
	packet[0] = 0x45
	packet[2] = 0
	packet[3] = 20
	wire, err := dataplane.MarshalIP(packet)
	if err != nil { t.Fatal(err) }
	got, err := classifyServicePayload(wire)
	if err != nil { t.Fatal(err) }
	if got != backendRawIP { t.Fatalf("backend=%q want %q", got, backendRawIP) }
}

func TestClassifyServicePayloadGame(t *testing.T) {
	var sid gamelane.SessionID
	for i := range sid { sid[i] = byte(i + 1) }
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }
	_, copies, err := enc.WrapCopies([]byte("raw-ip-inner"), []uint8{1,2})
	if err != nil { t.Fatal(err) }
	got, err := classifyServicePayload(copies[0].Wire)
	if err != nil { t.Fatal(err) }
	if got != backendGame { t.Fatalf("backend=%q want %q", got, backendGame) }
}

func TestClassifyServicePayloadPlatformProxy(t *testing.T) {
	wire, err := platformproxy.Marshal(platformproxy.Frame{Kind: platformproxy.KindUDPDatagram, FlowID: 7, Peer: netip.MustParseAddrPort("1.1.1.1:53"), Payload: []byte("dns")})
	if err != nil { t.Fatal(err) }
	got, err := classifyServicePayload(wire)
	if err != nil { t.Fatal(err) }
	if got != backendPlatform { t.Fatalf("backend=%q want %q", got, backendPlatform) }
}

func TestClassifyServicePayloadRejectsUnknown(t *testing.T) {
	if got, err := classifyServicePayload([]byte("not-a-wbd-frame")); err == nil { t.Fatalf("backend=%q, want error", got) }
}
