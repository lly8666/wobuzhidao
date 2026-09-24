package runtimeentry

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestOfferLatestBoundedDropsOldestAndKeepsNewest(t *testing.T) {
	q := make(chan int, 3)
	q <- 1
	q <- 2
	q <- 3
	dropped, droppedOld, accepted := offerLatestBounded(q, 4)
	if !droppedOld || dropped != 1 || !accepted {
		t.Fatalf("drop=%d droppedOld=%v accepted=%v", dropped, droppedOld, accepted)
	}
	got := []int{<-q, <-q, <-q}
	want := []int{2, 3, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queue=%v want=%v", got, want)
		}
	}
}

func TestServerReadQueueIsBoundedBurstBuffer(t *testing.T) {
	if got, want := serverReadQueueDepth, 4096; got != want {
		t.Fatalf("serverReadQueueDepth=%d want=%d", got, want)
	}
	q := newServerReadQueue()
	if got, want := cap(q), serverReadQueueDepth; got != want {
		t.Fatalf("capacity=%d want=%d", got, want)
	}
	if cap(q) <= 1 {
		t.Fatalf("receive handoff did not gain burst capacity: %d", cap(q))
	}
	for i := 0; i < cap(q); i++ {
		select {
		case q <- segmentRead{}:
		default:
			t.Fatalf("queue blocked before capacity at item %d", i)
		}
	}
	select {
	case q <- segmentRead{}:
		t.Fatal("queue accepted beyond bounded capacity")
	default:
	}
	<-q
	select {
	case q <- segmentRead{}:
	default:
		t.Fatal("queue did not resume after one bounded slot was released")
	}
}

func TestSegmentMuxDiagnosticReportsBoundedRouteBackpressure(t *testing.T) {
	if got, want := segmentMuxRouteDepth, 4096; got != want {
		t.Fatalf("segmentMuxRouteDepth=%d want=%d", got, want)
	}
	flow := faketcp.ClientFlow{
		LocalIP: [4]byte{192, 0, 2, 10}, PeerIP: [4]byte{192, 0, 2, 20},
		LocalPort: 40000, PeerPort: 443,
	}
	incoming := make(chan faketcp.Segment, segmentMuxRouteDepth+2)
	closed := make(chan struct{})
	var closeOnce sync.Once
	base := SegmentIO{
		Read: func() (faketcp.Segment, error) {
			select {
			case seg := <-incoming:
				return seg, nil
			case <-closed:
				return faketcp.Segment{}, io.EOF
			}
		},
		Emit: func(faketcp.Segment) error { return nil },
		Close: func() error {
			closeOnce.Do(func() { close(closed) })
			return nil
		},
	}
	mux, err := NewSegmentMux(base)
	if err != nil {
		t.Fatal(err)
	}
	mux.SetTimingDiagnostics(true)
	defer mux.Close()
	lane, err := mux.Open(flow)
	if err != nil {
		t.Fatal(err)
	}
	defer lane.close()

	seg := faketcp.Segment{
		SrcIP: flow.PeerIP, DstIP: flow.LocalIP,
		SrcPort: flow.PeerPort, DstPort: flow.LocalPort,
		Payload: []byte{1, 2, 3, 4},
	}
	for i := 0; i < segmentMuxRouteDepth+1; i++ {
		incoming <- seg
	}

	deadline := time.Now().Add(time.Second)
	for {
		snap := mux.DiagnosticSnapshot()
		if len(snap.Routes) == 1 && snap.Routes[0].FullWaits > 0 {
			route := snap.Routes[0]
			if route.OverflowDrops == 0 {
				t.Fatalf("full route did not shed oldest entry: %+v", route)
			}
			if !snap.Enabled {
				t.Fatal("diagnostics unexpectedly disabled")
			}
			if route.Capacity != segmentMuxRouteDepth {
				t.Fatalf("route capacity=%d want=%d", route.Capacity, segmentMuxRouteDepth)
			}
			if route.Peak < uint64(segmentMuxRouteDepth) {
				t.Fatalf("route peak=%d want at least %d", route.Peak, segmentMuxRouteDepth)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("route did not report bounded overflow shedding: %+v", snap)
		}
		time.Sleep(time.Millisecond)
	}

	if _, err := lane.Read(); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		route := mux.DiagnosticSnapshot().Routes[0]
		if route.HandoffBlock.Samples > 0 && route.QueueAge.Samples > 0 {
			if route.BytesPeak == 0 {
				t.Fatal("bytes peak was not recorded")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timing samples not recorded: %+v", route)
		}
		time.Sleep(time.Millisecond)
	}
}
