package runtimeentry

import (
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestLifecycleRetiredAssociationControlDoesNotStopSharedListener(t *testing.T) {
	h := newLifecycleAuditHarness(t, 1, 0, 0)
	h.server.mu.Lock()
	var lane *serverLifecycleLane
	for _, candidate := range h.server.byFlow {
		lane = candidate
	}
	if lane == nil {
		h.server.mu.Unlock()
		t.Fatal("no published lane")
	}
	// Pin the interleaving before byFlow removal: DORMANT has committed, but
	// the reader can still retain the published lane and its closed association.
	lane.group.dormant = true
	lane.assoc.Close()
	h.server.mu.Unlock()
	flow := lane.flow
	seg := faketcp.Segment{SrcIP: flow.ClientIP, DstIP: flow.ServerIP,
		SrcPort: flow.ClientPort, DstPort: flow.ServerPort, Flags: faketcp.FlagACK}
	if err := h.server.handleSegment(h.ctx, seg, time.Now()); err != nil {
		t.Fatalf("retired control killed shared listener: %v", err)
	}
	select {
	case err := <-h.serverDone:
		t.Fatalf("shared listener exited: %v", err)
	default:
	}
}

func TestLifecycleRetiredAssociationMarkerDoesNotHideLiveOrWireErrors(t *testing.T) {
	a, err := faketcp.NewServerAssociation(faketcp.Segment{SrcIP: [4]byte{192, 0, 2, 1},
		DstIP: [4]byte{192, 0, 2, 2}, SrcPort: 42000, DstPort: 443, Seq: 100, Flags: faketcp.FlagSYN}, 200, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	group := &serverLifecycleTunnel{}
	lane := &serverLifecycleLane{flow: a.Flow(), assoc: a, group: group}
	s := &LifecycleServer{byFlow: map[faketcp.ServerFlow]*serverLifecycleLane{lane.flow: lane}}
	seg := faketcp.Segment{SrcIP: lane.flow.ClientIP, DstIP: lane.flow.ServerIP,
		SrcPort: lane.flow.ClientPort, DstPort: lane.flow.ServerPort, Flags: faketcp.FlagACK}
	_, liveErr := a.HandleSegment(seg, time.Now())
	if !errors.Is(liveErr, faketcp.ErrHandshakeState) || errors.Is(liveErr, faketcp.ErrAssociationRetired) {
		t.Fatalf("invalid live ACK marked retired: %v", liveErr)
	}
	a.Close()
	_, retiredErr := a.HandleSegment(seg, time.Now())
	if !errors.Is(retiredErr, faketcp.ErrHandshakeState) || !errors.Is(retiredErr, faketcp.ErrAssociationRetired) {
		t.Fatalf("closed control provenance missing: %v", retiredErr)
	}
	if s.retiredAssociationError(lane, a, retiredErr) {
		t.Fatal("still-published active failure ignored")
	}
	group.dormant = true
	if !s.retiredAssociationError(lane, a, retiredErr) {
		t.Fatal("committed dormant control not isolated")
	}
	for _, wireErr := range []error{faketcp.ErrHandshakeState, syscall.EPERM, syscall.EBADF, errors.New("wire failure")} {
		if s.retiredAssociationError(lane, a, wireErr) {
			t.Fatalf("wire/live failure hidden: %v", wireErr)
		}
	}
}
