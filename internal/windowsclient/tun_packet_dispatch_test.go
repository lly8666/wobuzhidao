package windowsclient

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
)

type fakeTUNDirectSink struct {
	calls       int
	iface       uint32
	generation  uint64
	last        []byte
	fail        error
	mutateInput bool
}

func (s *fakeTUNDirectSink) ForwardDirectIPv4(p []byte, iface uint32, generation uint64) error {
	s.calls++
	s.iface = iface
	s.generation = generation
	s.last = append([]byte(nil), p...)
	if s.mutateInput {
		p[0] = 0
	}
	return s.fail
}

func n4TestDispatcher(t *testing.T, backend TUNDirectSink) (*TUNPacketDispatcher, *fakeOwner) {
	t.Helper()
	owner := &fakeOwner{
		lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.2"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 1},
	}
	r, err := NewRouter(owner, &fakeWriter{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewTUNScopedFlows(n4Policy(t, true))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewTUNPacketDispatcher(r, c, backend)
	if err != nil {
		t.Fatal(err)
	}
	return d, owner
}

func TestN4PacketDispatcherDirectLANAndProtectedOverseasAndDNS(t *testing.T) {
	sink := &fakeTUNDirectSink{mutateInput: true}
	d, owner := n4TestDispatcher(t, sink)
	now := time.Unix(1700000100, 0)

	direct := n4TestIPv4("10.66.0.2", "10.3.4.5", 17, 443, 0)
	original := append([]byte(nil), direct...)
	res, err := d.Dispatch(direct, 8, 113, now)
	if err != nil || res.Decision != TUNDirect || sink.calls != 1 ||
		sink.iface != 8 || sink.generation != 113 || owner.normalCalls != 0 ||
		len(res.Protected.Normal) != 0 || !bytes.Equal(direct, original) {
		t.Fatalf("direct branch violated: res=%+v err=%v calls=%d", res, err, sink.calls)
	}
	overseas := n4TestIPv4("10.66.0.2", "8.8.8.8", 17, 443, 0)
	res, err = d.Dispatch(overseas, 8, 113, now)
	if err != nil || res.Decision != TUNProtected || owner.normalCalls != 1 ||
		sink.calls != 1 || len(res.Protected.Normal) != 1 {
		t.Fatalf("protected branch violated: res=%+v err=%v", res, err)
	}
	dns := n4TestIPv4("10.66.0.2", "10.3.4.5", 17, 53, 0)
	res, err = d.Dispatch(dns, 8, 113, now)
	if err != nil || res.Decision != TUNDNSProtected || owner.normalCalls != 2 ||
		sink.calls != 1 {
		t.Fatalf("DNS hijack went direct: res=%+v err=%v", res, err)
	}
	underlay := n4TestIPv4("10.66.0.2", "198.18.0.6", 17, 443, 0)
	res, err = d.Dispatch(underlay, 8, 113, now)
	if err != nil || res.Decision != TUNDirect || sink.calls != 2 ||
		owner.normalCalls != 2 {
		t.Fatalf("underlay loop fence: res=%+v err=%v", res, err)
	}
}

func TestN4PacketDispatcherMissingDirectSinkFailsClosedAndNoTunnelFallback(t *testing.T) {
	d, owner := n4TestDispatcher(t, nil)
	p := n4TestIPv4("10.66.0.2", "10.4.5.6", 17, 443, 0)
	_, err := d.Dispatch(p, 5, 3, time.Unix(1700000150, 0))
	if !errors.Is(err, ErrTUNDirectBackendUnavailable) || owner.normalCalls != 0 {
		t.Fatalf("unbound physical stack leaked into encrypted lane: %v", err)
	}
	p = n4TestIPv4("10.66.0.3", "8.8.8.8", 17, 443, 0)
	_, err = d.Dispatch(p, 5, 3, time.Unix(1700000151, 0))
	if err == nil || owner.normalCalls != 0 {
		t.Fatalf("spoofed local TUN source reached owner: %v", err)
	}
	_, err = d.Dispatch(n4TestIPv4("10.66.0.2", "8.8.8.8", 17, 443, 0), 0, 3, time.Unix(1700000152, 0))
	if !errors.Is(err, ErrTUNFlowScope) {
		t.Fatalf("no OS-interface generation binding: %v", err)
	}
}

func TestN4PacketDispatcherPropagatesNativeErrorAndFragmentGenerationFence(t *testing.T) {
	nativeErr := errors.New("native system stack socket failed")
	sink := &fakeTUNDirectSink{fail: nativeErr}
	d, owner := n4TestDispatcher(t, sink)
	now := time.Unix(1700000200, 0)
	first := n4TestIPv4("10.66.0.2", "10.4.5.6", 17, 443, 0x2000)
	binary.BigEndian.PutUint16(first[4:6], 0x3101)
	later := append([]byte(nil), first[:20]...)
	binary.BigEndian.PutUint16(later[2:4], 20)
	binary.BigEndian.PutUint16(later[6:8], 1)
	_, err := d.Dispatch(later, 8, 123, now)
	if !errors.Is(err, ErrTUNFragmentUnmapped) || sink.calls != 0 {
		t.Fatalf("unknown fragment guessed direct: %v", err)
	}
	_, err = d.Dispatch(first, 8, 123, now)
	if !errors.Is(err, nativeErr) || sink.calls != 1 || owner.normalCalls != 0 {
		t.Fatalf("native transport error not propagated: %v", err)
	}
	_, err = d.Dispatch(later, 8, 123, now.Add(time.Second))
	if !errors.Is(err, nativeErr) || sink.calls != 2 {
		t.Fatalf("fragment failed safe native propagation: %v", err)
	}
	_, err = d.Dispatch(later, 8, 124, now.Add(2*time.Second))
	if !errors.Is(err, ErrTUNFragmentUnmapped) || sink.calls != 2 {
		t.Fatalf("cross reconnect fragment reused: %v", err)
	}
}

func TestN4PacketDispatcherRejectsClassifierOwnerLeaseMismatch(t *testing.T) {
	owner := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.7"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	r, err := NewRouter(owner, &fakeWriter{})
	if err != nil {
		t.Fatal(err)
	}
	flows, err := NewTUNScopedFlows(n4Policy(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTUNPacketDispatcher(r, flows, &fakeTUNDirectSink{}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("two different /32 leases accepted across native route boundary: %v", err)
	}
}
