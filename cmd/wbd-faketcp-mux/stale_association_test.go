//go:build linux

package main

import (
	"testing"
	"time"
)

func TestStaleDataAssociationTimeout(t *testing.T) {
	now := time.Unix(1000, 0)
	sess := &muxSession{stage: stageData, lastClientRX: now.Add(-staleDataAssociationTimeout)}
	stale, idle := sess.staleDataAssociation(now)
	if !stale || idle != staleDataAssociationTimeout {
		t.Fatalf("stale=%t idle=%s want stale at %s", stale, idle, staleDataAssociationTimeout)
	}

	sess.lastClientRX = now.Add(-staleDataAssociationTimeout + time.Millisecond)
	if stale, _ := sess.staleDataAssociation(now); stale {
		t.Fatal("association expired before timeout")
	}

	sess.stage = stageTransition
	sess.lastClientRX = now.Add(-10 * staleDataAssociationTimeout)
	if stale, _ := sess.staleDataAssociation(now); stale {
		t.Fatal("non-data association must not use data idle expiry")
	}
}

func TestClientReceiveRefreshesDataAssociation(t *testing.T) {
	now := time.Unix(2000, 0)
	sess := &muxSession{stage: stageData, lastClientRX: now.Add(-staleDataAssociationTimeout)}
	sess.noteClientRX(now)
	if stale, idle := sess.staleDataAssociation(now.Add(time.Second)); stale || idle != time.Second {
		t.Fatalf("stale=%t idle=%s want healthy after receive", stale, idle)
	}
}
