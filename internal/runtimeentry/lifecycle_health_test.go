package runtimeentry

import (
	"testing"
	"time"
)

func TestLifecycleHealthBlackholeFailedCandidatesAndStableRecovery(t *testing.T) {
	h := newLifecycleAuditHarness(t, 1, 200*time.Millisecond, 0)
	h.sendForward(t, [4]byte{1, 1, 1, 1})
	owner := h.client.Owner()
	before := laneGenerations(owner.ActiveLanes())
	h.failOpen.Store(true)
	// Blackhole both directions of the old 4-tuple permanently. New source
	// ports remain reachable after injected admission failures are lifted.
	h.dropPortsThrough.Store(43001)
	packet := ipv4Packet([4]byte{10, 66, 0, 31}, [4]byte{8, 8, 8, 8}, 17)
	until := time.Now().Add(4500 * time.Millisecond)
	for time.Now().Before(until) {
		if err := h.client.SendPacket(h.ctx, packet, time.Now()); err != nil {
			t.Fatal(err)
		}
		if h.client.IsDormant() {
			t.Fatal("lost keepalives converted ongoing business into idle")
		}
		select {
		case err := <-h.client.Errors():
			t.Fatalf("retryable failure escaped as terminal: %v", err)
		default:
		}
		time.Sleep(40 * time.Millisecond)
	}
	stats := h.client.LifecycleStats()
	if stats.RecoveryFailed == 0 || stats.RecoveryAttempts > 4 {
		t.Fatalf("missing recovery or retry storm: %+v", stats)
	}
	h.failOpen.Store(false)
	waitLifecycle(t, 6*time.Second, func() bool {
		// Continue offered demand without requiring one high-loss attempt to work.
		h.client.noteBusiness(time.Now())
		after := laneGenerations(owner.ActiveLanes())
		return after[1] > before[1] && h.client.LifecycleStats().RecoverySucceeded > 0
	})
	h.sendForward(t, [4]byte{9, 9, 9, 9})
	if h.client.Owner() != owner {
		t.Fatal("reconnect replaced stable logical tunnel")
	}
	lease, ok := owner.Lease()
	if !ok || lease.Config.Address4 != h.lease.Config.Address4 || lease.Config.TunnelID != h.lease.Config.TunnelID {
		t.Fatal("lease changed")
	}
}

func TestLifecycleIdleSnapshotCannotCloseNewDemand(t *testing.T) {
	h := newLifecycleAuditHarness(t, 1, 0, 0)
	h.client.mu.Lock()
	snapshot := h.client.lastPayload
	h.client.mu.Unlock()
	h.client.noteBusiness(snapshot.Add(time.Millisecond))
	if err := h.client.dormantIfIdle(snapshot); err != nil {
		t.Fatal(err)
	}
	if h.client.IsDormant() {
		t.Fatal("stale idle observation closed new demand")
	}
}

func TestLifecycleHealthConfigurationBounds(t *testing.T) {
	cfg := TunnelClientConfig{}
	if err := normalizeClientHealth(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.KeepaliveInterval != 15*time.Second || cfg.DeadAfter != 90*time.Second {
		t.Fatal("defaults drifted")
	}
	for _, bad := range []TunnelClientConfig{
		{KeepaliveInterval: -time.Second}, {KeepaliveInterval: time.Hour + time.Second},
		{KeepaliveInterval: time.Second, DeadAfter: 2 * time.Second},
		{ReconnectMin: time.Minute, ReconnectMax: time.Second},
	} {
		if normalizeClientHealth(&bad) == nil {
			t.Fatalf("accepted invalid lifecycle config: %+v", bad)
		}
	}
}
