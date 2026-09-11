//go:build linux

package main

import (
	"testing"
	"time"
)

func TestMuxDataSessionValidTrafficRefreshesExpiryWindow(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	sess := &muxSession{stage: stageData, lastClientRX: base}

	if stale, _ := sess.staleDataAssociation(base.Add(staleDataAssociationTimeout - time.Nanosecond)); stale {
		t.Fatal("data session expired before server idle window")
	}
	if stale, _ := sess.staleDataAssociation(base.Add(staleDataAssociationTimeout)); !stale {
		t.Fatal("data session did not expire at server idle window")
	}

	// Model protocol-valid FakeTCP traffic being accepted every 30 seconds.
	// Each accepted segment refreshes lastClientRX; LINK keepalive traffic is
	// not required to keep a genuinely active association alive.
	first := base.Add(30 * time.Second)
	sess.noteClientRX(first)
	if stale, _ := sess.staleDataAssociation(first.Add(staleDataAssociationTimeout - time.Nanosecond)); stale {
		t.Fatal("valid FakeTCP traffic did not refresh server liveness")
	}
	second := first.Add(30 * time.Second)
	sess.noteClientRX(second)
	if stale, _ := sess.staleDataAssociation(second.Add(staleDataAssociationTimeout - time.Nanosecond)); stale {
		t.Fatal("repeated valid FakeTCP traffic did not keep server association alive")
	}
	if stale, _ := sess.staleDataAssociation(second.Add(staleDataAssociationTimeout)); !stale {
		t.Fatal("server association did not expire after traffic actually stopped")
	}
}

func TestMuxNonDataStagesAreNotRetiredByDataIdleTimer(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	for _, stage := range []sessionStage{stageHandshake, stageBootstrap, stageFallback, stageTransition} {
		sess := &muxSession{stage: stage, lastClientRX: base}
		if stale, _ := sess.staleDataAssociation(base.Add(10 * staleDataAssociationTimeout)); stale {
			t.Fatalf("stage %d incorrectly retired by data idle timer", stage)
		}
	}
}
