package datapath

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func holdLaneDirection(t *testing.T, mu *sync.Mutex) func() {
	t.Helper()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		mu.Lock()
		close(entered)
		<-release
		mu.Unlock()
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("direction lock was not acquired")
	}
	var once sync.Once
	return func() {
		once.Do(func() { close(release) })
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("direction lock did not release")
		}
	}
}

func TestLaneBlockedTXDoesNotHoldReceiveTransitionOrExpiry(t *testing.T) {
	for _, parity := range []int{0, 12, 20} {
		t.Run(fmt.Sprintf("parity%d", parity), func(t *testing.T) {
			client, server := lanePair(t, parity)
			defer client.Close()
			defer server.Close()
			now := time.Now()
			first, second, outgoing := []byte("fresh reverse first"), []byte("fresh reverse transition"), []byte("blocked forward")
			incoming, err := server.Outbound(first, now)
			if err != nil {
				t.Fatal(err)
			}
			transition, err := server.Outbound(second, now)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseTX := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseTX()
			type sendResult struct {
				records []WireRecord
				err     error
			}
			txDone := make(chan sendResult, 1)
			go func() {
				records, err := client.outboundWithPaddingSelector(outgoing, now, func(_ int, _ bool) (paddingSelection, error) {
					close(entered)
					<-release
					return paddingSelection{}, nil
				})
				txDone <- sendResult{records, err}
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("TX did not reach controlled seal barrier")
			}
			type receiveResult struct {
				first, second InboundResult
				err           error
			}
			rxDone := make(chan receiveResult, 1)
			go func() {
				result, err := client.InboundPayload(incoming[0].Wire, now)
				if err != nil {
					rxDone <- receiveResult{err: err}
					return
				}
				transitionResult, err := client.InboundTransition([]faketcp.TransitionPacket{{Payload: transition[0].Wire}}, now)
				if err == nil {
					err = client.Expire(now.Add(4 * time.Second))
				}
				rxDone <- receiveResult{result, transitionResult, err}
			}()
			select {
			case result := <-rxDone:
				if result.err != nil || len(result.first.RecordErrors)+len(result.first.PathErrors)+len(result.second.RecordErrors)+len(result.second.PathErrors) != 0 {
					t.Fatalf("receive/expiry failed: %+v", result)
				}
				if len(result.first.Datagrams) != 1 || !bytes.Equal(result.first.Datagrams[0], first) ||
					len(result.second.Datagrams) != 1 || !bytes.Equal(result.second.Datagrams[0], second) {
					t.Fatalf("receive content changed: %+v", result)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("blocked TX prevented independent RX/transition/expiry")
			}
			releaseTX()
			select {
			case result := <-txDone:
				if result.err != nil {
					t.Fatal(result.err)
				}
				got := deliverRecords(t, server, result.records, now)
				if len(got) != 1 || !bytes.Equal(got[0], outgoing) {
					t.Fatalf("TX content/ownership changed after barrier: %q", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("TX did not resume after barrier release")
			}
		})
	}
}

func TestLaneBlockedRXDoesNotHoldTXFlushOrHealthPN(t *testing.T) {
	client, server := lanePair(t, 12)
	defer client.Close()
	defer server.Close()
	owner, err := NewTunnelOwner(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	binding, err := owner.AttachInitial(1, client)
	if err != nil {
		t.Fatal(err)
	}
	releaseRX := holdLaneDirection(t, &client.rxMu)
	defer releaseRX()
	now := time.Now()
	payload := []byte("forward continues while receive is blocked")
	type result struct {
		first, parity []WireRecord
		health        WireRecord
		err           error
	}
	done := make(chan result, 1)
	go func() {
		first, err := client.Outbound(payload, now)
		var parity []WireRecord
		var health WireRecord
		if err == nil {
			parity, err = client.FlushDue(now.Add(time.Second))
		}
		if err == nil {
			health, err = owner.HealthRecord(binding.Ref, 0)
		}
		done <- result{first, parity, health, err}
	}()
	select {
	case sent := <-done:
		if sent.err != nil || len(sent.first) != 1 || len(sent.parity) == 0 || !sent.health.Control {
			t.Fatalf("TX/flush/health failed: %+v", sent)
		}
		records := append(append(sent.first, sent.parity...), sent.health)
		for i := 1; i < len(records); i++ {
			if records[i].PN != records[i-1].PN+1 {
				t.Fatalf("TX/health PN space changed at %d: %d→%d", i, records[i-1].PN, records[i].PN)
			}
		}
		got := deliverRecords(t, server, records, now)
		if len(got) != 1 || !bytes.Equal(got[0], payload) {
			t.Fatalf("TX/health content changed: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked RX prevented independent TX/flush/health")
	}
}

func TestLaneDirectionCloseAndSnapshotConcurrent(t *testing.T) {
	client, server := lanePair(t, 12)
	defer client.Close()
	defer server.Close()
	client.SetTimingDiagnostics(true)
	now := time.Now()
	const operations = 128
	incoming := make([][]byte, operations)
	for i := range incoming {
		records, err := server.Outbound([]byte{byte(i), 1, 2, 3}, now)
		if err != nil {
			t.Fatal(err)
		}
		incoming[i] = records[0].Wire
	}
	ready := make(chan struct{}, 4)
	continueOps := make(chan struct{})
	done := make(chan error, 4)
	launch := func(operation func(int) error) {
		go func() {
			var firstErr error
			for i := 0; i < operations; i++ {
				if err := operation(i); err != nil && !errors.Is(err, ErrLaneClosed) && firstErr == nil {
					firstErr = err
				}
				if i == 0 {
					ready <- struct{}{}
					<-continueOps
				}
			}
			done <- firstErr
		}()
	}
	launch(func(i int) error {
		_, err := client.Outbound([]byte{byte(i), 4, 5, 6}, now)
		return err
	})
	launch(func(i int) error {
		result, err := client.InboundPayload(incoming[i], now)
		if len(result.RecordErrors)+len(result.PathErrors) != 0 {
			return fmt.Errorf("decode integrity: %v/%v", result.RecordErrors, result.PathErrors)
		}
		return err
	})
	launch(func(_ int) error { return client.Expire(now) })
	launch(func(_ int) error {
		snapshot := client.Stats()
		if snapshot.OutboundRecords < snapshot.OutboundDatagrams || snapshot.InboundRecords > snapshot.InboundPayloads {
			return fmt.Errorf("torn snapshot: %+v", snapshot)
		}
		return nil
	})
	for i := 0; i < 4; i++ {
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			close(continueOps)
			t.Fatal("initial concurrent operations did not complete")
		}
	}
	closed := make(chan struct{})
	go func() {
		<-continueOps
		client.Close()
		close(closed)
	}()
	close(continueOps)
	for i := 0; i < 4; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("directional operation/snapshot deadlocked with Close")
		}
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not join both directions")
	}
	if !client.Stats().Closed {
		t.Fatal("Close state was not published")
	}
	if _, err := client.Outbound([]byte("after close"), now); !errors.Is(err, ErrLaneClosed) {
		t.Fatalf("TX after Close=%v", err)
	}
	if _, err := client.InboundPayload(incoming[0], now); !errors.Is(err, ErrLaneClosed) {
		t.Fatalf("RX after Close=%v", err)
	}
	if err := client.Expire(now); !errors.Is(err, ErrLaneClosed) {
		t.Fatalf("expiry after Close=%v", err)
	}
}
