package datapath

import (
	"testing"
	"time"
)

// Model a seal/decode operation that owns lane.mu. Metadata inspection must
// finish without acquiring that lock or withholding owner.mu from padding.
func TestActiveLaneSnapshotDoesNotAcquirePacketProcessingLock(t *testing.T) {
	owner, err := NewTunnelOwner(3, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	var held *Lane
	for id := uint8(1); id <= 3; id++ {
		lane := tunnelTestLane(t, RoleClient, 12, 93+id)
		if _, err := owner.AttachInitial(id, lane); err != nil {
			t.Fatal(err)
		}
		if id == 1 {
			held = lane
		}
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	done := make(chan []TunnelLaneSnapshot, 1)
	go func() { done <- owner.ActiveLanes() }()
	select {
	case snapshots := <-done:
		if len(snapshots) != 3 {
			t.Fatalf("snapshots=%d", len(snapshots))
		}
		for i, snapshot := range snapshots {
			if snapshot.Ref.ID != uint8(i+1) || snapshot.Role != RoleClient || snapshot.ParityShards != 12 {
				t.Fatalf("immutable snapshot=%+v", snapshot)
			}
		}
		if owner.Stats().ActiveLogicalLanes != 3 {
			t.Fatal("metadata read changed owner membership")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ActiveLanes waited for packet lock; padding callbacks can deadlock")
	}
}
