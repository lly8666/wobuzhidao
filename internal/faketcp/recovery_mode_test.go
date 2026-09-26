package faketcp

import (
	"testing"
	"time"
)

func TestRecoveryOracleChangesOnlyShadowRetransmission(t *testing.T) {
	now := time.Unix(10, 0)
	legacy := NewSenderWithRecovery(1000, time.Second, RecoveryLegacy)
	advanced := NewSenderWithRecovery(1000, time.Second, RecoverySACKRACK)
	for i := 0; i < 4; i++ {
		p1 := legacy.Enqueue(make([]byte, 100), now.Add(time.Duration(i)*time.Millisecond))
		p2 := advanced.Enqueue(make([]byte, 100), now.Add(time.Duration(i)*time.Millisecond))
		if p1 == nil || p2 == nil || p1.Seq != p2.Seq {
			t.Fatal("recovery mode changed immediate enqueue semantics")
		}
	}
	sacks := []SACKBlock{{Start: 1100, End: 1400}}
	if got := legacy.AckSelective(1000, sacks, now.Add(20*time.Millisecond)); got != nil {
		t.Fatalf("legacy unexpectedly used SACK/RACK recovery: %+v", got)
	}
	if got := advanced.AckSelective(1000, sacks, now.Add(20*time.Millisecond)); got == nil || got.Seq != 1000 {
		t.Fatalf("advanced did not infer first hole: %+v", got)
	}
	// Classic third duplicate ACK remains available in the legacy oracle.
	if got := legacy.AckSelective(1000, sacks, now.Add(21*time.Millisecond)); got != nil {
		t.Fatalf("legacy retransmitted before third duplicate ACK: %+v", got)
	}
	if got := legacy.AckSelective(1000, sacks, now.Add(22*time.Millisecond)); got == nil || got.Seq != 1000 {
		t.Fatalf("legacy third-dup-ACK retransmit=%+v", got)
	}
}
