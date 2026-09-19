package faketcp

import (
	"bytes"
	"testing"
	"time"
)

func ordinarySYNForAudit(port uint16, seq uint32, mss uint16, mssSet, sack, wsSet bool, ws uint8) Segment {
	return Segment{
		SrcIP:          [4]byte{10, 2, 0, byte(port%200 + 1)},
		DstIP:          [4]byte{10, 2, 1, 1},
		SrcPort:        port,
		DstPort:        443,
		Seq:            seq,
		Flags:          FlagSYN,
		Window:         64240,
		MSS:            mss,
		MSSSet:         mssSet,
		SACKPermitted:  sack,
		WindowScale:    ws,
		WindowScaleSet: wsSet,
	}
}

func TestServerAssociationAcceptsOrdinaryInitialSYNProfiles(t *testing.T) {
	cases := []struct {
		name         string
		syn          Segment
		wantPeerMSS  uint16
		wantSACK     bool
		wantWS       bool
		wantWireDoFF byte
	}{
		{
			name: "common-1460-ws7-sack",
			syn: ordinarySYNForAudit(23001, 100, 1460, true, true, true, 7),
			wantPeerMSS: 1460, wantSACK: true, wantWS: true, wantWireDoFF: 8,
		},
		{
			name: "no-options-default-ipv4-mss",
			syn: ordinarySYNForAudit(23002, 200, 0, false, false, false, 0),
			wantPeerMSS: DefaultIPv4PeerMSS, wantSACK: false, wantWS: false, wantWireDoFF: 6,
		},
		{
			name: "small-mss-no-sack-ws4",
			syn: ordinarySYNForAudit(23003, 300, 600, true, false, true, 4),
			wantPeerMSS: 600, wantSACK: false, wantWS: true, wantWireDoFF: 7,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsWBDHandshakeSegment(tc.syn) {
				t.Fatal("ordinary SYN unexpectedly classified as WBD presentation")
			}
			a, err := NewServerAssociation(tc.syn, 9000, time.Second, func(Segment) error { return nil })
			if err != nil {
				t.Fatalf("ordinary SYN rejected: %v", err)
			}
			defer a.Close()

			peer := a.PeerTCPProfile()
			if peer.MSS != tc.wantPeerMSS || peer.AdvertisedMSS != tc.syn.MSSSet ||
				peer.SACKPermitted != tc.syn.SACKPermitted ||
				peer.WindowScaleSet != tc.syn.WindowScaleSet ||
				peer.WindowScale != tc.syn.WindowScale {
				t.Fatalf("peer profile=%#v", peer)
			}

			synack, err := a.SYNACKSegment()
			if err != nil {
				t.Fatal(err)
			}
			if !synack.MSSSet || synack.MSS != DefaultMSS ||
				synack.SACKPermitted != tc.wantSACK ||
				synack.WindowScaleSet != tc.wantWS {
				t.Fatalf("SYN-ACK negotiation=%#v", synack)
			}
			if tc.wantWS && synack.WindowScale != DefaultWindowScale {
				t.Fatalf("server WS=%d want=%d", synack.WindowScale, DefaultWindowScale)
			}

			wire := MarshalSegment(synack, 7, PacketPersonaLegacy)
			if got := wire[20+12] >> 4; got != tc.wantWireDoFF {
				t.Fatalf("TCP data offset=%d want=%d", got, tc.wantWireDoFF)
			}
			parsed, err := ParseIPv4TCP(wire)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.MSS != DefaultMSS || !parsed.MSSSet ||
				parsed.SACKPermitted != tc.wantSACK ||
				parsed.WindowScaleSet != tc.wantWS {
				t.Fatalf("serialized SYN-ACK options=%#v", parsed)
			}
		})
	}
}

func TestServerBootstrapHonorsPeerMSS(t *testing.T) {
	syn := ordinarySYNForAudit(23101, 1000, 600, true, false, true, 5)
	emitCh := make(chan Segment, 4)
	a, err := NewServerAssociation(syn, 5000, 10*time.Second, func(seg Segment) error {
		emitCh <- cloneSegmentForAudit(seg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	finalACK := syn
	finalACK.Flags = FlagACK
	finalACK.Seq = syn.Seq + 1
	finalACK.Ack = 5001
	if _, err := a.HandleSegment(finalACK, time.Now()); err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte{0x5a}, 1000)
	done := make(chan error, 1)
	go func() {
		_, err := a.BootstrapConn().Write(payload)
		done <- err
	}()

	first := <-emitCh
	if len(first.Payload) != 600 {
		t.Fatalf("first bootstrap payload=%d want peer MSS 600", len(first.Payload))
	}
	ack := finalACK
	ack.Ack = first.Seq + uint32(len(first.Payload))
	if _, err := a.HandleSegment(ack, time.Now()); err != nil {
		t.Fatal(err)
	}

	second := <-emitCh
	if len(second.Payload) != 400 {
		t.Fatalf("second bootstrap payload=%d want=400", len(second.Payload))
	}
	ack.Ack = second.Seq + uint32(len(second.Payload))
	if _, err := a.HandleSegment(ack, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap write did not finish")
	}
}

func TestServerBootstrapUsesIPv4DefaultMSSWhenPeerOmitsMSS(t *testing.T) {
	syn := ordinarySYNForAudit(23102, 2000, 0, false, false, false, 0)
	emitCh := make(chan Segment, 1)
	a, err := NewServerAssociation(syn, 6000, time.Second, func(seg Segment) error {
		emitCh <- cloneSegmentForAudit(seg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	ack := syn
	ack.Flags = FlagACK
	ack.Seq++
	ack.Ack = 6001
	if _, err := a.HandleSegment(ack, time.Now()); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := a.BootstrapConn().Write(bytes.Repeat([]byte{1}, 700))
		done <- err
	}()
	first := <-emitCh
	if len(first.Payload) != DefaultIPv4PeerMSS {
		t.Fatalf("payload=%d want default IPv4 peer MSS=%d", len(first.Payload), DefaultIPv4PeerMSS)
	}
	ack.Ack = first.Seq + uint32(len(first.Payload))
	if _, err := a.HandleSegment(ack, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Close rather than finish the second chunk; this test only fixes the
	// implicit-MSS send ceiling.
	a.Close()
	<-done
}

func TestInitialSYNValidationRejectsMalformedCombinations(t *testing.T) {
	base := ordinarySYNForAudit(23201, 1, 1460, true, true, true, 7)
	cases := []Segment{
		func() Segment { s := base; s.Flags |= FlagACK; return s }(),
		func() Segment { s := base; s.Flags |= FlagRST; return s }(),
		func() Segment { s := base; s.Payload = []byte{1}; return s }(),
		func() Segment { s := base; s.MSS = 0; return s }(),
		func() Segment { s := base; s.WindowScale = MaxWindowScale + 1; return s }(),
	}
	for i, syn := range cases {
		if IsInitialSYN(syn) {
			t.Fatalf("case %d accepted as legal initial SYN: %#v", i, syn)
		}
		if _, err := NewServerAssociation(syn, 1, time.Second, func(Segment) error { return nil }); err == nil {
			t.Fatalf("case %d association accepted malformed SYN", i)
		}
	}
}

func cloneSegmentForAudit(seg Segment) Segment {
	seg.Payload = append([]byte(nil), seg.Payload...)
	return seg
}
