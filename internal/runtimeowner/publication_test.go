package runtimeowner

import (
	"runtime"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// A business producer selects its ref from TunnelOwner before consulting the
// Runtime transport. Every published owner ref must already have a transport
// by the time that consumer acquires Runtime.mu; no retry/sleep hides a gap.
func TestReplacementPublicationIncludesFreshTransport(t *testing.T) {
	for iteration := 0; iteration < 64; iteration++ {
		lease := runtimeLease(t)
		owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 4)
		if err != nil { t.Fatal(err) }
		r, _ := New(owner, nil)
		cfg, _ := transportPair(func(faketcp.Segment) error { return nil }, func(faketcp.Segment) error { return nil }, 1, 1000)
		old, err := r.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 8), cfg)
		if err != nil { t.Fatal(err) }
		if err := r.BeginSameIDReplacement(old.Ref, runtimeLane(t, datapath.RoleClient, lease, 0, 8), cfg); err != nil { t.Fatal(err) }
		ready := make(chan struct{}); stop := make(chan struct{}); done := make(chan bool, 1)
		go func() {
			first := true
			for {
				for _, lane := range owner.ActiveLanes() {
					r.mu.Lock(); present := r.lanes[lane.Ref] != nil; r.mu.Unlock()
					if !present { if first { close(ready) }; done <- false; return }
				}
				if first { first = false; close(ready) }
				select { case <-stop: done <- true; return; default: runtime.Gosched() }
			}
		}()
		<-ready
		_, promotionErr := r.PromoteSameIDReplacement(old.Ref)
		close(stop)
		published := <-done
		r.Close()
		if promotionErr != nil { t.Fatal(promotionErr) }
		if !published { t.Fatal("business producer observed a published ref without a transport") }
	}
}
