package platformproxy

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
)

func TestRoundTripUDPIPv4AndIPv6(t *testing.T) {
	for _, peer := range []netip.AddrPort{
		netip.MustParseAddrPort("203.0.113.9:53"),
		netip.MustParseAddrPort("[2001:db8::9]:5353"),
	} {
		want := Frame{Kind: KindUDPDatagram, FlowID: 7, Peer: peer, Payload: []byte("dns")}
		wire, err := Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if len(wire) != HeaderSize+len(want.Payload) {
			t.Fatalf("wire len=%d", len(wire))
		}
		got, err := Unmarshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		assertFrameEqual(t, got, want)
	}
}

func TestRoundTripTCPControlAndData(t *testing.T) {
	frames := []Frame{
		{Kind: KindTCPOpen, FlowID: 11, Peer: netip.MustParseAddrPort("198.51.100.4:443")},
		{Kind: KindTCPData, FlowID: 11, Offset: 4096, Payload: []byte("hello")},
		{Kind: KindTCPData, FlowID: 11, Offset: 4101, FIN: true, Payload: []byte("world")},
		{Kind: KindTCPData, FlowID: 11, Offset: 4106, FIN: true},
		{Kind: KindTCPAck, FlowID: 11, Offset: 4106},
		{Kind: KindTCPClose, FlowID: 11},
	}
	for _, want := range frames {
		wire, err := Marshal(want)
		if err != nil {
			t.Fatalf("%v: %v", want.Kind, err)
		}
		got, err := Unmarshal(wire)
		if err != nil {
			t.Fatalf("%v: %v", want.Kind, err)
		}
		assertFrameEqual(t, got, want)
	}
}

func TestRejectsMalformedAndOversize(t *testing.T) {
	if _, err := Marshal(Frame{Kind: KindTCPClose}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("zero flow err=%v", err)
	}
	if _, err := Marshal(Frame{Kind: KindUDPDatagram, FlowID: 1, Peer: netip.MustParseAddrPort("203.0.113.1:53"), Offset: 1}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("udp offset err=%v", err)
	}
	if _, err := Marshal(Frame{Kind: KindTCPData, FlowID: 1}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("empty data err=%v", err)
	}
	if _, err := Marshal(Frame{Kind: KindTCPData, FlowID: 1, Payload: make([]byte, MaxPayload+1)}); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversize err=%v", err)
	}

	wire, err := Marshal(Frame{Kind: KindTCPAck, FlowID: 5, Offset: 99})
	if err != nil {
		t.Fatal(err)
	}
	wire = append(wire, 0)
	if _, err := Unmarshal(wire); !errors.Is(err, ErrMalformed) {
		t.Fatalf("trailing err=%v", err)
	}

	bad := make([]byte, HeaderSize)
	copy(bad[:4], magic[:])
	bad[4] = Version1
	bad[5] = byte(KindTCPAck)
	bad[7] = 9
	bad[15] = 1 // flow id 1
	if _, err := Unmarshal(bad); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("family err=%v", err)
	}
}

func assertFrameEqual(t *testing.T, got, want Frame) {
	t.Helper()
	if got.Kind != want.Kind || got.FlowID != want.FlowID || got.Offset != want.Offset || got.FIN != want.FIN || got.Peer != want.Peer || !bytes.Equal(got.Payload, want.Payload) {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
}
