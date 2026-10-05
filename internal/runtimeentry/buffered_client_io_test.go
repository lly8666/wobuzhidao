package runtimeentry

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func bufferedTestFlow() faketcp.ClientFlow {
	return faketcp.ClientFlow{LocalIP: [4]byte{192, 0, 2, 10}, PeerIP: [4]byte{192, 0, 2, 20}, LocalPort: 40000, PeerPort: 443}
}

func TestBufferedClientIODrainsWhileHandlerPausedAndOwnsPayload(t *testing.T) {
	flow := bufferedTestFlow()
	incoming := make(chan byte, 8)
	closed := make(chan struct{})
	var once sync.Once
	var reads atomic.Int64
	// Native capture APIs reuse their packet storage on the next read.
	scratch := []byte{0}
	base := SegmentIO{
		Read: func() (faketcp.Segment, error) {
			select {
			case value := <-incoming:
				scratch[0] = value
				reads.Add(1)
				return faketcp.Segment{SrcIP: flow.PeerIP, DstIP: flow.LocalIP, SrcPort: flow.PeerPort, DstPort: flow.LocalPort, Seq: uint32(value), Payload: scratch}, nil
			case <-closed:
				return faketcp.Segment{}, io.EOF
			}
		}, Emit: func(faketcp.Segment) error { return nil }, Close: func() error { once.Do(func() { close(closed) }); return nil },
	}
	queued, mux, err := BufferedClientIO(base, flow, true)
	if err != nil {
		t.Fatal(err)
	}
	defer queued.close()
	arrival := []byte{2, 0, 4, 1, 3}
	for _, value := range arrival {
		incoming <- value
	}
	deadline := time.Now().Add(time.Second)
	for reads.Load() < int64(len(arrival)) {
		if time.Now().After(deadline) {
			t.Fatal("capture reader waited for handler")
		}
		time.Sleep(time.Millisecond)
	}
	for _, want := range arrival {
		seg, err := queued.Read()
		if err != nil || len(seg.Payload) != 1 || seg.Payload[0] != want || seg.Seq != uint32(want) {
			t.Fatalf("arrival/ownership changed: got%+v err%v want%d", seg, err, want)
		}
	}
	snap := mux.DiagnosticSnapshot()
	if len(snap.Routes) != 1 || snap.Routes[0].OverflowDrops != 0 || snap.Routes[0].Capacity != segmentMuxRouteDepth {
		t.Fatalf("queue bounds/drop: %+v", snap)
	}
}

func TestBufferedClientIOPropagatesCaptureFailureAndClosesOnce(t *testing.T) {
	captureErr := errors.New("capture failed")
	trigger := make(chan struct{})
	closed := make(chan struct{})
	var closes atomic.Int64
	var once sync.Once
	base := SegmentIO{Read: func() (faketcp.Segment, error) {
		select {
		case <-trigger:
			return faketcp.Segment{}, captureErr
		case <-closed:
			return faketcp.Segment{}, io.EOF
		}
	}, Emit: func(faketcp.Segment) error { return nil }, Close: func() error { once.Do(func() { closes.Add(1); close(closed) }); return nil }}
	queued, _, err := BufferedClientIO(base, bufferedTestFlow(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer queued.close()
	done := make(chan error, 1)
	go func() { _, err := queued.Read(); done <- err }()
	close(trigger)
	select {
	case err := <-done:
		if !errors.Is(err, captureErr) {
			t.Fatalf("lost capture error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("failed capture left route waiting")
	}
	if err := queued.close(); err != nil {
		t.Fatal(err)
	}
	if closes.Load() != 1 {
		t.Fatalf("underlying closes=%d", closes.Load())
	}
}

func TestBufferedClientIOCloseWakesReader(t *testing.T) {
	closed := make(chan struct{})
	var once sync.Once
	base := SegmentIO{Read: func() (faketcp.Segment, error) { <-closed; return faketcp.Segment{}, io.EOF }, Emit: func(faketcp.Segment) error { return nil }, Close: func() error { once.Do(func() { close(closed) }); return nil }}
	queued, _, err := BufferedClientIO(base, bufferedTestFlow(), false)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := queued.Read(); done <- err }()
	if err := queued.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close left handler waiting")
	}
}
