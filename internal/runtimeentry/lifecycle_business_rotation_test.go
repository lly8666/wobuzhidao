package runtimeentry

import (
	"context"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestActiveBusinessDoesNotWaitForReplacementCandidate(t *testing.T) {
	h := newLifecycleAuditHarness(t, 1, 0, 0)
	h.sendForward(t, [4]byte{1, 1, 1, 1})
	entered := make(chan struct{})
	release := make(chan struct{})
	open := h.client.cfg.OpenLane
	h.client.cfg.OpenLane = func(id uint8, incarnation uint64) (SegmentIO, faketcp.ClientFlow, error) {
		close(entered)
		select {
		case <-release:
		case <-h.ctx.Done():
			return SegmentIO{}, faketcp.ClientFlow{}, h.ctx.Err()
		}
		return open(id, incarnation)
	}
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	defer close(release)
	rotation := make(chan error, 1)
	go func() { rotation <- h.client.RotateOldest(ctx) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement never entered candidate admission")
	}
	packet := ipv4Packet([4]byte{10, 66, 0, 31}, [4]byte{8, 8, 8, 8}, 17)
	sent := make(chan error, 1)
	go func() { sent <- h.client.SendPacket(ctx, packet, time.Now()) }()
	select {
	case err := <-sent:
		if err != nil { t.Fatal(err) }
	case <-time.After(3 * time.Second):
		t.Fatal("healthy business waited for unfinished replacement candidate")
	}
	if got := h.serverTUN.waitPacket(t, 3*time.Second); string(got) != string(packet) {
		t.Fatal("old healthy lane did not deliver during candidate admission")
	}
}
