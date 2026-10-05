package runtimeentry

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestRetryableBusinessWakeDoesNotHideWireOrBindingFailures(t *testing.T) {
	for _, err := range []error{ErrLifecycleRetryBackoff, fmt.Errorf("%w: %w", ErrBusinessWakeRetryable, context.DeadlineExceeded)} {
		if !RetryableBusinessWake(err) {
			t.Fatalf("wake/backoff should keep platform reader alive: %v", err)
		}
	}
	for _, err := range []error{nil, context.DeadlineExceeded, syscall.EAGAIN, syscall.ENETDOWN, ErrClientLeaseChanged, ErrClientRuntimeStopped, context.Canceled} {
		if RetryableBusinessWake(err) {
			t.Fatalf("unclassified wire/binding/shutdown error hidden: %v", err)
		}
		if err != nil && RetryableBusinessWake(fmt.Errorf("%w: %w", ErrBusinessWakeRetryable, err)) &&
			(errors.Is(err, ErrClientLeaseChanged) || errors.Is(err, ErrClientRuntimeStopped) || errors.Is(err, context.Canceled)) {
			t.Fatalf("terminal error lost through wrapping: %v", err)
		}
	}
}

func TestBusinessDemandAfterFailedDormantWakeRetriesWithoutClosingOwner(t *testing.T) {
	for _, lanes := range []int{1, 4} {
		t.Run(fmt.Sprint(lanes), func(t *testing.T) {
			h := newLifecycleAuditHarness(t, lanes, 0, 0)
			owner := h.client.Owner()
			if err := h.client.Dormant(); err != nil {
				t.Fatal(err)
			}
			if err := h.server.DormantTunnel(h.tunnelID); err != nil {
				t.Fatal(err)
			}
			// Game fails its second attachment, exercising cleanup of a
			// partially successful wake as well as Normal's first-lane failure.
			var failWake atomic.Bool
			failWake.Store(true)
			failLane := uint8(1)
			if lanes == 4 {
				failLane = 2
			}
			open := h.client.cfg.OpenLane
			h.client.cfg.OpenLane = func(id uint8, incarnation uint64) (SegmentIO, faketcp.ClientFlow, error) {
				if id == failLane && failWake.Load() {
					return SegmentIO{}, faketcp.ClientFlow{}, errors.New("injected business wake failure")
				}
				return open(id, incarnation)
			}
			packet := ipv4Packet([4]byte{10, 66, 0, 31}, [4]byte{8, 8, 8, 8}, 17)
			err := h.client.SendPacket(h.ctx, packet, time.Now())
			if !RetryableBusinessWake(err) || !errors.Is(err, ErrBusinessWakeRetryable) {
				t.Fatalf("failed pre-emission wake is terminal: %v", err)
			}
			stats := h.client.LifecycleStats()
			if stats.RecoveryFailed != 1 || stats.RetryableErrors != 1 || stats.NextRetry.IsZero() || stats.LastError == "" {
				t.Fatalf("missing failed-wake evidence/backoff: %+v", stats)
			}
			h.client.mu.Lock()
			h.client.retryAt = time.Now().Add(time.Minute)
			h.client.mu.Unlock()
			for i := 0; i < 32; i++ {
				if err := h.client.SendPacket(h.ctx, packet, time.Now()); !errors.Is(err, ErrLifecycleRetryBackoff) || !RetryableBusinessWake(err) {
					t.Fatalf("backoff demand: %v", err)
				}
			}
			if got := h.client.LifecycleStats(); got.RecoveryAttempts != stats.RecoveryAttempts || got.RetryableErrors != stats.RetryableErrors {
				t.Fatalf("backoff causes attempt/error amplification: %+v", got)
			}
			if !h.client.IsDormant() || len(owner.ActiveLanes()) != 0 {
				t.Fatal("failed wake exposed a lane")
			}
			select {
			case err := <-h.client.Errors():
				t.Fatalf("retryable wake reported as terminal: %v", err)
			default:
			}
			failWake.Store(false)
			h.client.mu.Lock()
			h.client.retryAt = time.Time{}
			h.client.mu.Unlock()
			h.sendForward(t, [4]byte{9, 9, 9, 9})
			if h.client.Owner() != owner || h.client.IsDormant() || len(owner.ActiveLanes()) != lanes {
				t.Fatal("retry did not recover the same owner")
			}
			lease, ok := owner.Lease()
			if !ok || lease.Config.Address4 != h.lease.Config.Address4 || lease.Config.TunnelID != h.lease.Config.TunnelID {
				t.Fatal("wake retry changed logical binding")
			}
		})
	}
}

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
