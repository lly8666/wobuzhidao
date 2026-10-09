package runtimeowner

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// Unlike synthetic-time Runtime tests, this opt-in scheduler test uses the
// real monotonic wall clock. It proves a sparse systematic source still emits
// immediately while one owner-level reusable timer emits partial parity even
// without another packet or a 100ms lifecycle Tick.
func TestFECDeadlineSchedulerSparseFirstAndStop(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	wires := make(chan faketcp.Segment, 16)
	emit := func(seg faketcp.Segment) error {
		select {
		case wires <- seg:
			return nil
		default:
			t.Fatal("unexpected unbounded burst on sparse path")
			return nil
		}
	}
	cfg, _ := transportPair(emit, nil, 1, 1000)
	if _, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 20, 9), cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { rt.RunFECDeadlineSchedule(ctx); close(done) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("owner-level deadline scheduler did not stop")
		}
	}()
	src, _ := lease.Config.LeaseIPv4()
	now := time.Now()
	packet := runtimeIPv4(src, netip.MustParseAddr("1.1.1.1"), []byte("sparse"))
	records, err := owner.NormalOutbound(packet, now)
	if err != nil || len(records) != 1 {
		t.Fatalf("systematic now: records=%d err=%v", len(records), err)
	}
	if err := rt.SendNormal(records, now); err != nil {
		t.Fatal(err)
	}
	select {
	case first := <-wires:
		if len(first.Payload) == 0 {
			t.Fatal("immediate systematic empty")
		}
	case <-time.After(time.Second):
		t.Fatal("fresh packet blocked by parity timer")
	}
	select {
	case parity := <-wires:
		if len(parity.Payload) == 0 {
			t.Fatal("partial parity empty")
		}
	case <-time.After(350 * time.Millisecond):
		t.Fatal("pending parity did not flush while application was idle")
	}
	if due := owner.NextActiveFlushDeadline(); !due.IsZero() {
		t.Fatalf("expired group still scheduled: %v", due)
	}
}

// FEC-off and an empty active group are completely timer-idle. The scheduler
// is canceled independently of the client's raw IO or 100ms maintenance tick.
func TestFECDeadlineSchedulerIdleCancel(t *testing.T) {
	owner, err := datapath.NewTunnelOwner(1, 8)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { rt.RunFECDeadlineSchedule(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle deadline timer leaked after cancel")
	}
	rt.Close()
	rt.Close()
}
