package windowsclient

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type probePacketWriter func([]byte) (int, error)

func (f probePacketWriter) WritePacket(p []byte) (int, error) { return f(p) }

func probeFragmentPacket(flags uint16) []byte {
	p := make([]byte, 44)
	copy(p, []byte{0x45, 0, 0, 44, 0x12, 0x34, 0x20, 0, 64, 17, 0, 0,
		198, 18, 0, 1, 10, 66, 0, 7})
	binary.BigEndian.PutUint16(p[6:8], flags)
	binary.BigEndian.PutUint16(p[20:22], 18446)
	binary.BigEndian.PutUint16(p[22:24], 51000)
	binary.BigEndian.PutUint16(p[24:26], 65515)
	binary.BigEndian.PutUint16(p[26:28], 0xabcd)
	copy(p[28:32], "P7M1")
	binary.LittleEndian.PutUint32(p[32:36], 1828)
	return p
}

func probeWriter(t *testing.T, next PacketWriter) *ProbeFragmentWriter {
	t.Helper()
	w, err := NewProbeFragmentWriter(next, netip.MustParseAddr("10.66.0.7"))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestProbeFragmentWriterMetadataAndExactOutcome(t *testing.T) {
	p := probeFragmentPacket(8192)
	writeErr := errors.New("driver failure")
	w := probeWriter(t, probePacketWriter(func(got []byte) (int, error) {
		if &got[0] != &p[0] {
			t.Error("observer cloned/replaced the input")
		}
		return 17, writeErr
	}))
	n, err := w.WritePacket(p)
	if n != 17 || err != writeErr {
		t.Fatalf("write outcome changed: %d %v", n, err)
	}
	s := w.DiagnosticSnapshot()
	if s.Observed != 1 || s.Recorded != 1 || len(s.Rows) != 1 {
		t.Fatalf("coverage: %+v", s)
	}
	r := s.Rows[0]
	if r.IPID != 0x1234 || r.Sequence != 1828 || r.UDPLength != 65515 || r.UDPChecksum != 0xabcd ||
		r.UDPSourcePort != 18446 || r.UDPDestinationPort != 51000 || r.Offset != 0 || !r.MoreFragments ||
		r.IPLength != 44 || r.HeaderLength != 20 || r.FragmentPayloadLength != 24 || r.HeaderChecksumOK ||
		r.WrittenBytes != 17 || !r.WriteError || r.UnixNS == 0 || r.WriteEndUnixNS < r.UnixNS {
		t.Fatalf("wrong metadata: %+v", r)
	}
	if after := w.DiagnosticSnapshot(); len(after.Rows) != 0 || after.Recorded != 1 {
		t.Fatalf("rows must drain without resetting counters: %+v", after)
	}
}

func TestProbeFragmentWriterScopeAndNonfirstFragment(t *testing.T) {
	var calls int
	w := probeWriter(t, probePacketWriter(func(p []byte) (int, error) { calls++; return len(p), nil }))
	for _, modify := range []func([]byte){
		func(p []byte) { p[12] = 1 }, func(p []byte) { p[19] = 8 },
		func(p []byte) { p[9] = 6 }, func(p []byte) { p[21]++ },
		func(p []byte) { p[0] = 0x44 }, func(p []byte) { p[3]-- },
	} {
		p := probeFragmentPacket(0)
		modify(p)
		if n, err := w.WritePacket(p); n != len(p) || err != nil {
			t.Fatalf("observer rejected/outcome changed outside scope: %d %v", n, err)
		}
	}
	if s := w.DiagnosticSnapshot(); s.Observed != 0 || len(s.Rows) != 0 {
		t.Fatal("ordinary traffic captured")
	}
	p := probeFragmentPacket(8192 | 172)
	p[20], p[21] = 0, 0 // Nonfirst fragments have no UDP source-port header.
	if _, err := w.WritePacket(p); err != nil {
		t.Fatal(err)
	}
	r := w.DiagnosticSnapshot().Rows[0]
	if r.Offset != 1376 || r.Sequence != -1 || r.UDPLength != 0 || !r.MoreFragments || calls != 7 {
		t.Fatalf("nonfirst fragment was not preserved: %+v calls=%d", r, calls)
	}
}

func TestProbeFragmentWriterBoundsDoNotShedBusiness(t *testing.T) {
	var calls int
	w := probeWriter(t, probePacketWriter(func(p []byte) (int, error) { calls++; return len(p), nil }))
	stamp := time.Unix(100, 0)
	w.clock = func() time.Time { return stamp }
	p := probeFragmentPacket(0)
	for i := 0; i < probeFragmentPendingCap+1; i++ {
		if n, err := w.WritePacket(p); n != len(p) || err != nil {
			t.Fatal("full metadata storage affected business")
		}
	}
	s := w.DiagnosticSnapshot()
	if s.QueueDropped != 1 || len(s.Rows) != probeFragmentPendingCap || s.Observed != probeFragmentPendingCap+1 {
		t.Fatalf("queue bound: %+v", s)
	}
	// Pin the lifetime cap without retaining 40000 rows or changing business.
	w.stats.Recorded = probeFragmentTotalCap
	w.WritePacket(p)
	if s = w.DiagnosticSnapshot(); s.LimitDropped != 1 || len(s.Rows) != 0 {
		t.Fatalf("total cap: %+v", s)
	}
	stamp = stamp.Add(probeFragmentWindow)
	w.WritePacket(p)
	if s = w.DiagnosticSnapshot(); s.WindowDropped != 1 || calls != probeFragmentPendingCap+3 {
		t.Fatalf("time cap affected submission: %+v calls=%d", s, calls)
	}
}

func TestProbeFragmentWriterSnapshotDoesNotHoldDriverWrite(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	w := probeWriter(t, probePacketWriter(func(p []byte) (int, error) {
		close(entered)
		<-release
		return len(p), nil
	}))
	go func() { defer close(done); w.WritePacket(probeFragmentPacket(0)) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("underlying submission never started")
	}
	snap := make(chan ProbeFragmentDiagnostic, 1)
	go func() { snap <- w.DiagnosticSnapshot() }()
	select {
	case <-snap:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("observer lock held across driver write")
	}
	close(release)
	<-done
}

func TestProbeFragmentWriterConcurrentDrainAndInvalidBinding(t *testing.T) {
	var calls atomic.Uint64
	w := probeWriter(t, probePacketWriter(func(p []byte) (int, error) { calls.Add(1); return len(p), nil }))
	var group sync.WaitGroup
	stop, drained := make(chan struct{}), make(chan int, 1)
	go func() {
		count := 0
		for {
			select {
			case <-stop:
				drained <- count
				return
			default:
				count += len(w.DiagnosticSnapshot().Rows)
				time.Sleep(time.Microsecond)
			}
		}
	}()
	p := probeFragmentPacket(0)
	for i := 0; i < 100; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 10; j++ {
				w.WritePacket(p)
			}
		}()
	}
	group.Wait()
	close(stop)
	count := <-drained
	s := w.DiagnosticSnapshot()
	if calls.Load() != 1000 || s.Observed != 1000 || s.Recorded != 1000 || count+len(s.Rows) != 1000 {
		t.Fatal("concurrent observation lost coverage")
	}
	for _, address := range []netip.Addr{{}, netip.MustParseAddr("::1")} {
		if _, err := NewProbeFragmentWriter(w.next, address); !errors.Is(err, ErrInvalidBinding) {
			t.Fatal("invalid destination accepted")
		}
	}
	if _, err := NewProbeFragmentWriter(nil, netip.MustParseAddr("10.66.0.7")); !errors.Is(err, ErrInvalidBinding) {
		t.Fatal("nil writer accepted")
	}
}

func TestProbeFragmentWriterHeaderOptionsAndUnknownSequence(t *testing.T) {
	w := probeWriter(t, probePacketWriter(func(p []byte) (int, error) { return len(p), nil }))
	// Independent IPv4 header vector: IHL=24, two NOP options, checksum=0x762c.
	p := append([]byte{0x46, 0, 0, 48, 0x12, 0x34, 0x20, 0, 64, 17, 0x76, 0x2c,
		198, 18, 0, 1, 10, 66, 0, 7, 1, 1, 0, 0}, probeFragmentPacket(8192)[20:]...)
	binary.LittleEndian.PutUint32(p[36:40], 0)
	w.WritePacket(p)
	r := w.DiagnosticSnapshot().Rows[0]
	if !r.HeaderChecksumOK || r.HeaderLength != 24 || r.FragmentPayloadLength != 24 || r.Sequence != 0 {
		t.Fatalf("header options or sequence zero misread: %+v", r)
	}
	p[32] = 'X' // Other controlled UDP echo content is not a probe identity.
	w.WritePacket(p)
	if r = w.DiagnosticSnapshot().Rows[0]; r.Sequence != -1 {
		t.Fatal("invented probe identity from unknown content")
	}
	for size := 0; size < 20; size++ {
		w.WritePacket(p[:size]) // Malformed inputs must still reach the real writer.
	}
	q := append([]byte(nil), p[:24]...)
	binary.BigEndian.PutUint16(q[2:4], 24)
	w.WritePacket(q) // A first fragment without a UDP header is not observable.
	q[0] = 0x4f      // Declared options extending beyond the provided bytes.
	w.WritePacket(q)
	if s := w.DiagnosticSnapshot(); s.Observed != 2 || len(s.Rows) != 0 {
		t.Fatal("malformed packet produced diagnostic metadata")
	}
}
