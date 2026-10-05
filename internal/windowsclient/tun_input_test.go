package windowsclient

import (
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func TestTUNRejectedInputDoesNotDispatchOrPreventNextValidPacket(t *testing.T) {
	owner := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.7"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	router, err := NewRouter(owner, &fakeWriter{})
	if err != nil {
		t.Fatal(err)
	}
	in := NewTUNInput(router)
	src, dst := netip.MustParseAddr("10.66.0.7"), netip.MustParseAddr("1.1.1.1")
	oversize := ipv4Packet(src, dst, make([]byte, logicaltunnel.MaxLeasedIPv4PacketLen-19))
	spoof := ipv4Packet(netip.MustParseAddr("10.66.0.8"), dst, nil)
	for _, packet := range [][]byte{oversize, {}, spoof} {
		ok, err := in.Accept(packet)
		if ok || err != nil {
			t.Fatalf("rejected ok=%v err=%v", ok, err)
		}
	}
	if owner.normalCalls != 0 || owner.gameCalls != 0 {
		t.Fatal("rejected input dispatched")
	}
	valid := ipv4Packet(src, dst, make([]byte, logicaltunnel.MaxLeasedIPv4PacketLen-20))
	ok, err := in.Accept(valid)
	if !ok || err != nil {
		t.Fatalf("max valid ok=%v err=%v", ok, err)
	}
	if _, err = router.RouteFromTUN(valid, time.Now()); err != nil {
		t.Fatal(err)
	}
	if owner.normalCalls != 1 {
		t.Fatalf("next valid calls=%d", owner.normalCalls)
	}
	s := in.DiagnosticSnapshot()
	if s.RejectedOversize != 1 || s.RejectedInvalidIPv4 != 1 || s.RejectedSource != 1 {
		t.Fatalf("stats=%+v", s)
	}
}

func TestTUNInputInvalidBindingIsNotSilentlyDropped(t *testing.T) {
	for _, in := range []*TUNInput{nil, NewTUNInput(nil)} {
		ok, err := in.Accept(nil)
		if ok || !errors.Is(err, ErrInvalidBinding) {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	}
}

func TestTUNInputDiagnosticConcurrentWithRejectedInput(t *testing.T) {
	owner := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.7"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	router, _ := NewRouter(owner, &fakeWriter{})
	in := NewTUNInput(router)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			in.Accept(nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			in.DiagnosticSnapshot()
		}
	}()
	wg.Wait()
	if in.DiagnosticSnapshot().RejectedInvalidIPv4 != 1000 {
		t.Fatal("lost rejection counter")
	}
}
