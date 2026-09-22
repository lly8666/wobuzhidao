package datapath

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"
)

func startupHello() []byte {
	// TLS record + ClientHello with TLS 1.2 legacy_version, empty session ID,
	// one TLS 1.3 cipher, null compression, no extensions (parser-only fixture).
	b := make([]byte, 50)
	copy(b, []byte{22, 3, 1, 0, 45, 1, 0, 0, 41, 3, 3})
	b[45], b[46], b[47], b[48] = 2, 0x13, 1, 1
	return b
}

func startupTCP(seq uint32, flags byte, data []byte, reverse bool) []byte {
	b := make([]byte, 40+len(data))
	b[0], b[8], b[9] = 0x45, 64, 6
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	copy(b[12:16], []byte{10, 66, 0, 2})
	copy(b[16:20], []byte{1, 1, 1, 1})
	binary.BigEndian.PutUint16(b[20:22], 12345)
	binary.BigEndian.PutUint16(b[22:24], 443)
	if reverse {
		a, c := append([]byte(nil), b[12:16]...), append([]byte(nil), b[20:22]...)
		copy(b[12:16], b[16:20])
		copy(b[16:20], a)
		copy(b[20:22], b[22:24])
		copy(b[22:24], c)
	}
	binary.BigEndian.PutUint32(b[24:28], seq)
	b[32], b[33] = 0x50, flags
	copy(b[40:], data)
	return b
}

func TestStartupDetectionSplitReorderConflictAndExpiry(t *testing.T) {
	now := time.Unix(500, 0)
	hello := startupHello()
	var s startupTracker
	s.observe(startupTCP(100, 2, nil, false), true, now)
	if s.observe(startupTCP(111, 16, hello[10:], false), true, now) != nil {
		t.Fatal("tail detected early")
	}
	s.observe(startupTCP(101, 16, hello[:10], false), true, now)
	if s.detected != 1 {
		t.Fatal("split/reordered hello not detected")
	}
	p := startupTCP(800, 16, []byte("reply"), true)
	f := s.observe(p, true, now.Add(time.Second))
	if f == nil {
		t.Fatal("reverse direction not eligible")
	}
	deadline := f.deadline
	if s.observe(p, true, now.Add(1500*time.Millisecond)) != nil || f.deadline != deadline {
		t.Fatal("duplicate renewed budget/window")
	}
	if s.observe(startupTCP(805, 16, []byte("later"), true), true, deadline) != nil {
		t.Fatal("expired flow padded")
	}
	for _, d := range f.directions {
		if d.prefix != nil {
			t.Fatal("expired prefix retained")
		}
	}
	s.expire(now.Add(StartupRetention))
	if len(s.flows) != 0 {
		t.Fatal("retention unbounded")
	}
	var conflict startupTracker
	conflict.observe(startupTCP(1, 16, hello[:8], false), true, now)
	bad := append([]byte(nil), hello...)
	bad[3]++
	if conflict.observe(startupTCP(1, 16, bad, false), true, now) != nil || conflict.detected != 0 {
		t.Fatal("conflicting overlap accepted")
	}
}

func TestStartupBoundedCapacityAndMalformedBypass(t *testing.T) {
	now := time.Unix(501, 0)
	var s startupTracker
	for i := 0; i < StartupMaxFlows+1; i++ {
		p := startupTCP(1, 2, nil, false)
		binary.BigEndian.PutUint16(p[20:22], uint16(i+1))
		s.observe(p, true, now)
	}
	if len(s.flows) != StartupMaxFlows || s.capacitySkips != 1 {
		t.Fatalf("state=%d skips=%d", len(s.flows), s.capacitySkips)
	}
	s.expire(now.Add(StartupRetention))
	if len(s.flows) != 0 {
		t.Fatal("capacity not released")
	}
	for _, b := range [][]byte{[]byte("GET / HTTP/1.1\r\n"), {22, 3, 3, 255, 255}, {22, 3, 3, 0, 45, 2, 0, 0, 41}} {
		valid, reject := startupClientHello(b)
		if valid || !reject {
			t.Fatalf("malformed not bypassed: %x", b)
		}
	}
}

func startupTestOwner(t *testing.T, role Role, parity int) (*TunnelOwner, *Lane) {
	t.Helper()
	o, err := NewTunnelOwner(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Close)
	p := TLSStartupPaddingPolicy(true)
	p.BytesPerRecord = 1
	p.MaxPaddingRatioPPM = PaddingRatioScalePPM
	if err = o.ConfigurePadding(p); err != nil {
		t.Fatal(err)
	}
	l := tunnelTestLane(t, role, parity, 61)
	if _, err = o.AttachInitial(1, l); err != nil {
		t.Fatal(err)
	}
	return o, l
}

func TestStartupNormalNoWaitNoHOLAndInboundDetection(t *testing.T) {
	c, _ := startupTestOwner(t, RoleClient, 0)
	s, _ := startupTestOwner(t, RoleServer, 0)
	now := time.Unix(502, 0)
	hello := startupHello()
	first := startupTCP(1, 16, hello[:8], false)
	r, err := c.NormalOutbound(first, now)
	if err != nil || len(r) != 1 || r[0].PaddingBytes != 0 {
		t.Fatalf("partial hello held/padded: %v %v", r, err)
	}
	ref := s.active[1].ref
	got, err := s.InboundPayload(ref, r[0].Wire, now)
	if err != nil || len(got.Datagrams) != 1 || !bytes.Equal(got.Datagrams[0], first) {
		t.Fatal("partial hello did not immediately roundtrip")
	}
	r, err = c.NormalOutbound(startupTCP(9, 16, hello[8:], false), now)
	if err != nil || len(r) != 1 || r[0].PaddingBytes != 1 {
		t.Fatalf("detection output: %v %v", r, err)
	}
	if _, err = s.InboundPayload(ref, r[0].Wire, now); err != nil {
		t.Fatal(err)
	}
	if s.Stats().Padding.StartupDetected != 1 {
		t.Fatal("inbound detection absent")
	}
	// Lose server PN0 permanently; PN1 must decrypt/deliver immediately.
	if _, err = s.NormalOutbound(startupTCP(500, 16, []byte("lost"), true), now); err != nil {
		t.Fatal(err)
	}
	want := startupTCP(504, 16, []byte("received"), true)
	r, err = s.NormalOutbound(want, now)
	if err != nil || r[0].PaddingBytes != 1 {
		t.Fatal("reply not padded")
	}
	got, err = c.InboundPayload(c.active[1].ref, r[0].Wire, now)
	if err != nil || len(got.Datagrams) != 1 || !bytes.Equal(got.Datagrams[0], want) {
		t.Fatal("padding introduced cross-record HOL")
	}
}

func TestStartupAllFECProfilesSkipParityAndBoundRecords(t *testing.T) {
	for _, parity := range []int{0, 4, 8, 10, 12, 16, 20} {
		t.Run(fmt.Sprint(parity), func(t *testing.T) {
			o, _ := startupTestOwner(t, RoleClient, parity)
			now := time.Unix(503, 0)
			for i := 0; i < 20; i++ {
				payload := []byte("next")
				seq := uint32(51 + (i-1)*4)
				if i == 0 {
					payload = startupHello()
					seq = 1
				}
				r, err := o.NormalOutbound(startupTCP(seq, 16, payload, false), now)
				if err != nil {
					t.Fatal(err)
				}
				if i == 19 {
					for _, w := range r[1:] {
						if w.PaddingBytes != 0 {
							t.Fatal("parity padded")
						}
					}
				}
			}
			if st := o.Stats().Padding; st.PaddedRecords != StartupMaxRecords || st.PaddingBytes != StartupMaxRecords {
				t.Fatalf("flow budget: %+v", st)
			}
			r, err := o.TickLane(o.active[1].ref, now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range r {
				if w.PaddingBytes != 0 {
					t.Fatal("timer parity padded")
				}
			}
		})
	}
}

func TestStartupReservationRollbackAndHeadroom(t *testing.T) {
	o, _ := startupTestOwner(t, RoleClient, 0)
	now := time.Unix(504, 0)
	selector := o.paddingSelectorForPayload(startupTCP(1, 16, startupHello(), false), now)
	if selector == nil {
		t.Fatal("missing selector")
	}
	no, err := selector(0, true)
	if err != nil || no.bytes != 0 || !no.skipped {
		t.Fatal("headroom bypass")
	}
	for i := 0; i < StartupMaxRecords+1; i++ {
		r, err := selector(100, true)
		if err != nil || r.bytes != 1 {
			t.Fatal("reservation leaked")
		}
		r.finish(false)
	}
	if st := o.Stats().Padding; st.PaddingBytes != 0 || st.PaddedRecords != 0 {
		t.Fatal("failed seal charged")
	}
	r, err := selector(100, false)
	if err != nil || r.bytes != 0 {
		t.Fatal("parity eligible")
	}
}

func FuzzStartupPacket(f *testing.F) {
	f.Add(startupTCP(1, 16, startupHello(), false))
	f.Add([]byte{22, 3, 3})
	f.Fuzz(func(t *testing.T, b []byte) {
		var s startupTracker
		s.observe(b, true, time.Unix(1, 0))
		startupClientHello(b)
	})
}

func TestStartupGameCopiesShareFlowBudget(t *testing.T) {
	lease := gameLease(t)
	addr, err := lease.Config.LeaseIPv4()
	if err != nil {
		t.Fatal(err)
	}
	o, err := NewLeasedTunnelOwner(lease, 4, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	p := TLSStartupPaddingPolicy(true)
	p.BytesPerRecord = 1
	p.MaxPaddingRatioPPM = PaddingRatioScalePPM
	if err = o.ConfigurePadding(p); err != nil {
		t.Fatal(err)
	}
	for id := uint8(1); id <= 4; id++ {
		if _, err = o.AttachInitial(id, leasedTestLane(t, RoleClient, 0, 90+id, lease)); err != nil {
			t.Fatal(err)
		}
	}
	var useful uint64
	for i := 0; i < 5; i++ {
		data := []byte("more")
		seq := uint32(51 + (i-1)*4)
		if i == 0 {
			data = startupHello()
			seq = 1
		}
		packet := startupTCP(seq, 16, data, false)
		ip := addr.As4()
		copy(packet[12:16], ip[:])
		useful += uint64(len(packet))
		out, err := o.GameOutbound(packet, time.Unix(505, 0))
		if err != nil || len(out.Failures) != 0 || len(out.Lanes) != 4 {
			t.Fatalf("game output: %+v %v", out, err)
		}
	}
	st := o.Stats().Padding
	if st.PaddedRecords != StartupMaxRecords || st.UsefulPayloadBytes != useful || st.StartupDetected != 1 {
		t.Fatalf("per-lane budget multiplication: %+v", st)
	}
}

func TestStartupConcurrentReservationsBounded(t *testing.T) {
	o, _ := startupTestOwner(t, RoleClient, 0)
	selector := o.paddingSelectorForPayload(startupTCP(1, 16, startupHello(), false), time.Unix(506, 0))
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := selector(100, true)
			if err != nil {
				t.Error(err)
				return
			}
			if r.finish != nil {
				r.finish(true)
			}
		}()
	}
	wg.Wait()
	if st := o.Stats().Padding; st.PaddedRecords != StartupMaxRecords || st.PaddingBytes != StartupMaxRecords {
		t.Fatalf("concurrent overrun: %+v", st)
	}
}
