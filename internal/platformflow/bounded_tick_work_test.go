package platformflow

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedTickWorkNeverQueuesAndCloseWaits(t *testing.T) {
	var w boundedTickWork
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int64
	if !w.tryRun(time.Unix(1, 0), func(time.Time) {
		runs.Add(1)
		close(started)
		<-release
	}) {
		t.Fatal("first tick not scheduled")
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("maintenance worker never started")
	}
	for i := 0; i < 100; i++ {
		if w.tryRun(time.Unix(int64(i+2), 0), func(time.Time) {
			runs.Add(1000)
		}) {
			t.Fatal("busy worker accepted another tick instead of coalescing")
		}
	}
	closeEntered := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		close(closeEntered)
		w.close()
		close(closed)
	}()
	<-closeEntered
	select {
	case <-closed:
		t.Fatal("close returned before running maintenance completed")
	default:
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("maintenance close stuck")
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("ticks executed=%d, want exactly one", got)
	}
	if w.tryRun(time.Now(), func(time.Time) { runs.Add(1) }) {
		t.Fatal("worker accepted tick after close")
	}
	// Concurrent, idempotent close never races a new WaitGroup.Add.
	w.close()
}

func TestBoundedTickWorkAtMostOneConcurrentWorker(t *testing.T) {
	var w boundedTickWork
	started := make(chan struct{})
	release := make(chan struct{})
	var active, maxActive, accepted atomic.Int64
	work := func(time.Time) {
		n := active.Add(1)
		for {
			old := maxActive.Load()
			if old >= n || maxActive.CompareAndSwap(old, n) { break }
		}
		close(started)
		<-release
		active.Add(-1)
	}
	const contenders = 64
	var wg sync.WaitGroup
	wg.Add(contenders)
	for i := 0; i < contenders; i++ {
		go func() {
			defer wg.Done()
			if w.tryRun(time.Now(), work) { accepted.Add(1) }
		}()
	}
	wg.Wait()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("no worker started")
	}
	if accepted.Load() != 1 || maxActive.Load() != 1 {
		t.Fatalf("worker count accepted=%d max=%d", accepted.Load(), maxActive.Load())
	}
	close(release)
	w.close()
}

func TestGameServiceTickDoesNotWaitForTCPMaintenance(t *testing.T) {
	// Emulate a slow flow lock in the TCP maintenance path. In Game,
	// Server.Tick must return independently of this flow; Normal must
	// retain the original synchronous method instead.
	flow := &tcpServerFlow{closed: true}
	flow.mu.Lock()
	s := &Server{
		udp: &UDPServer{flows: make(map[uint64]*udpServerState)},
		tcp: &TCPServer{flows: map[uint64]*tcpServerFlow{7: flow}},
	}
	s.EnableGameTCPMaintenance()
	returned := make(chan struct{})
	go func() {
		s.Tick(time.Unix(100, 0))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		flow.mu.Unlock()
		t.Fatal("Game Tick blocked on a TCP flow instead of offloading maintenance")
	}
	if s.tcp.ScheduleTick(time.Unix(101, 0)) {
		flow.mu.Unlock()
		t.Fatal("concurrent Game maintenance admitted more than one worker")
	}
	done := make(chan struct{})
	go func() { s.tcp.Close(); close(done) }()
	select {
	case <-done:
		flow.mu.Unlock()
		t.Fatal("TCP server Close returned while maintenance held a flow")
	default:
	}
	flow.mu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("TCP server Close did not wait for maintenance")
	}
}
