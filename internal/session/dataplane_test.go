package session

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/fec"
)

func offConfig() control.LinkConfig {
	return control.LinkConfig{
		FECMode:   control.FECOff,
		Scheduler: control.FECSchedulerNone,
		MTU:       1400,
		LaneCount: 1,
	}
}

func fixedConfig() control.LinkConfig {
	return control.LinkConfig{
		FECMode:      control.FECFixed,
		Scheduler:    control.FECSchedulerTailRS,
		DataShards:   20,
		ParityShards: 20,
		FlushMillis:  8,
		MTU:          1400,
		LaneCount:    1,
	}
}

func TestDataPlaneSameAccountIndependentOffPaths(t *testing.T) {
	d, err := NewDataPlane(4, 64)
	if err != nil {
		t.Fatal(err)
	}
	var a, b LiveID
	a[0], b[0] = 1, 2
	if err := d.Reserve("solo", a, "127.0.0.1:41001", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := d.Reserve("solo", b, "127.0.0.1:41002", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := d.Activate(a, offConfig()); err != nil {
		t.Fatal(err)
	}
	if err := d.Activate(b, offConfig()); err != nil {
		t.Fatal(err)
	}

	peerA, wireA, err := d.Outbound(a, []byte("device-a"), time.Now())
	if err != nil || peerA != "127.0.0.1:41001" || len(wireA) != 1 {
		t.Fatalf("outbound a peer=%q wire=%d err=%v", peerA, len(wireA), err)
	}
	peerB, wireB, err := d.Outbound(b, []byte("device-b"), time.Now())
	if err != nil || peerB != "127.0.0.1:41002" || len(wireB) != 1 {
		t.Fatalf("outbound b peer=%q wire=%d err=%v", peerB, len(wireB), err)
	}

	idA, packetsA, err := d.Inbound(peerA, wireA[0])
	if err != nil || idA != a || len(packetsA) != 1 || string(packetsA[0]) != "device-a" {
		t.Fatalf("inbound a id=%x packets=%q err=%v", idA, packetsA, err)
	}
	idB, packetsB, err := d.Inbound(peerB, wireB[0])
	if err != nil || idB != b || len(packetsB) != 1 || string(packetsB[0]) != "device-b" {
		t.Fatalf("inbound b id=%x packets=%q err=%v", idB, packetsB, err)
	}
	if d.Len() != 2 {
		t.Fatalf("len=%d want=2", d.Len())
	}
}

func TestDataPlaneRejectsDataBeforeImmutableActivation(t *testing.T) {
	d, _ := NewDataPlane(1, 64)
	var id LiveID
	id[0] = 9
	if err := d.Reserve("solo", id, "127.0.0.1:42001", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Outbound(id, []byte("x"), time.Now()); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("outbound before LINK_ACCEPT err=%v", err)
	}
	if _, _, err := d.Inbound("127.0.0.1:42001", []byte("x")); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("inbound before LINK_ACCEPT err=%v", err)
	}
	if err := d.Activate(id, offConfig()); err != nil {
		t.Fatal(err)
	}
	if err := d.Activate(id, fixedConfig()); !errors.Is(err, ErrSessionAlreadyActive) {
		t.Fatalf("second immutable activation err=%v", err)
	}
}

func TestDataPlaneFixedFECStateIsPerSession(t *testing.T) {
	d, err := NewDataPlane(2, 64)
	if err != nil {
		t.Fatal(err)
	}
	var a, b LiveID
	a[0], b[0] = 11, 12
	if err := d.Reserve("solo", a, "127.0.0.1:43001", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := d.Reserve("solo", b, "127.0.0.1:43002", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := d.Activate(a, fixedConfig()); err != nil {
		t.Fatal(err)
	}
	if err := d.Activate(b, fixedConfig()); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	peerA, wireA, err := d.Outbound(a, []byte("A-first"), now)
	if err != nil || len(wireA) == 0 {
		t.Fatalf("a first systematic wire=%d err=%v", len(wireA), err)
	}
	peerB, wireB, err := d.Outbound(b, []byte("B-first"), now)
	if err != nil || len(wireB) == 0 {
		t.Fatalf("b first systematic wire=%d err=%v", len(wireB), err)
	}
	_, gotA, err := d.Inbound(peerA, wireA[0])
	if err != nil || len(gotA) != 1 || string(gotA[0]) != "A-first" {
		t.Fatalf("a decoded=%q err=%v", gotA, err)
	}
	_, gotB, err := d.Inbound(peerB, wireB[0])
	if err != nil || len(gotB) != 1 || string(gotB[0]) != "B-first" {
		t.Fatalf("b decoded=%q err=%v", gotB, err)
	}
}

func TestDataPlaneFixedFECWireOwnershipSurvivesEncoderReuse(t *testing.T) {
	for _, tc := range []struct {
		name       string
		firstSize  int
		secondSize int
	}{
		{name: "large_then_small", firstSize: 1364, secondSize: 96},
		{name: "small_then_large", firstSize: 160, secondSize: 1364},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := NewDataPlane(1, 64)
			if err != nil {
				t.Fatal(err)
			}
			var id LiveID
			id[0] = 21
			if err := d.Reserve("solo", id, "127.0.0.1:44001", time.Now()); err != nil {
				t.Fatal(err)
			}
			if err := d.Activate(id, fixedConfig()); err != nil {
				t.Fatal(err)
			}

			now := time.Now()
			if _, _, err := d.Outbound(id, bytes.Repeat([]byte{0xa5}, tc.firstSize), now); err != nil {
				t.Fatal(err)
			}
			_, held, err := d.Flush(id)
			if err != nil || len(held) != 1 {
				t.Fatalf("first flush wire=%d err=%v", len(held), err)
			}
			frame := held[0]
			if got, want := len(frame), fec.HeaderSize+tc.firstSize; got != want {
				t.Fatalf("first parity len=%d want=%d", got, want)
			}
			snapshot := append([]byte(nil), frame...)

			if _, _, err := d.Outbound(id, bytes.Repeat([]byte{0x5a}, tc.secondSize), now.Add(time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			_, next, err := d.Flush(id)
			if err != nil || len(next) != 1 {
				t.Fatalf("second flush wire=%d err=%v", len(next), err)
			}
			if got, want := len(next[0]), fec.HeaderSize+tc.secondSize; got != want {
				t.Fatalf("second parity len=%d want=%d", got, want)
			}
			if !bytes.Equal(frame, snapshot) {
				h, parseErr := fec.ParseBlockHeader(frame[:fec.HeaderSize])
				if parseErr != nil {
					t.Fatalf("returned FEC wire mutated after encoder reuse: frame_len=%d parse_header=%v first_shard=%d second_shard=%d", len(frame), parseErr, tc.firstSize, tc.secondSize)
				}
				t.Fatalf("returned FEC wire mutated after encoder reuse: frame_len=%d declared_shard_bytes=%d expected_wire_bytes=%d first_shard=%d second_shard=%d", len(frame), h.ShardSize, fec.HeaderSize+int(h.ShardSize), tc.firstSize, tc.secondSize)
			}
		})
	}
}

func TestDataPlaneConcurrentSharedAccountDemux(t *testing.T) {
	const n = 32
	d, err := NewDataPlane(n, 64)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var id LiveID
			id[0] = byte(i + 1)
			peer := "peer-" + itoa(i+1)
			if err := d.Reserve("solo", id, peer, time.Now()); err != nil {
				errCh <- err
				return
			}
			if err := d.Activate(id, offConfig()); err != nil {
				errCh <- err
				return
			}
			payload := []byte{byte(i + 1)}
			gotPeer, wire, err := d.Outbound(id, payload, time.Now())
			if err != nil {
				errCh <- err
				return
			}
			if gotPeer != peer || len(wire) != 1 {
				errCh <- errors.New("wrong peer/wire")
				return
			}
			gotID, packets, err := d.Inbound(peer, wire[0])
			if err != nil || gotID != id || len(packets) != 1 || len(packets[0]) != 1 || packets[0][0] != byte(i+1) {
				if err != nil {
					errCh <- err
				} else {
					errCh <- errors.New("cross-session packet routing")
				}
				return
			}
			errCh <- nil
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if d.Len() != n {
		t.Fatalf("len=%d want=%d", d.Len(), n)
	}
}
