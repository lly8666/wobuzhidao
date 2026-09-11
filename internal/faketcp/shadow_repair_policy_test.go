package faketcp

import (
	"bytes"
	"testing"
	"time"
)

func TestSACKRetirementFreesRepairWindowWithoutBreakingMergedSACK(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s := NewSenderWithRecovery(1000, 1500*time.Millisecond, RecoverySACKRACK)

	pending := make([]*Pending, 0, 6)
	for i := 0; i < 5; i++ {
		pending = append(pending, s.Enqueue(bytes.Repeat([]byte{byte(i + 1)}, 100), now))
	}
	if got := s.Pending(); got != 5 {
		t.Fatalf("initial repair pending=%d want=5", got)
	}

	repair := s.AckSelective(pending[0].Seq, []SACKBlock{{Start: pending[1].Seq, End: pending[4].End}}, now.Add(600*time.Millisecond))
	if repair != pending[0] {
		t.Fatalf("fast repair=%p want oldest hole=%p", repair, pending[0])
	}
	if got := s.Pending(); got != 1 {
		t.Fatalf("SACK-proven records still consume repair window: pending=%d want=1", got)
	}
	for i := 1; i < len(pending); i++ {
		p := s.Outstanding(pending[i].Seq)
		if p == nil || !p.Retired || p.Payload != nil {
			t.Fatalf("record %d not retained as payload-free SACK tombstone: %#v", i, p)
		}
	}

	fresh := s.Enqueue(bytes.Repeat([]byte{0x55}, 100), now.Add(700*time.Millisecond))
	if got := s.Pending(); got != 2 {
		t.Fatalf("fresh admission did not use released repair capacity: pending=%d want=2", got)
	}

	// The receiver may later merge a SACK range whose start is an already-retired
	// tombstone. Walking the retained seq/end chain must still reach and retire
	// the newly admitted record.
	s.AckSelective(pending[0].Seq, []SACKBlock{{Start: pending[1].Seq, End: fresh.End}}, now.Add(800*time.Millisecond))
	if !fresh.Retired || fresh.Payload != nil {
		t.Fatalf("merged SACK did not reach fresh record: %#v", fresh)
	}
	if got := s.Pending(); got != 1 {
		t.Fatalf("merged SACK left delivered fresh data in repair window: pending=%d want=1", got)
	}
}

func TestShadowRepairByteBudgetDefersExcessRetry(t *testing.T) {
	now := time.Unix(1700000100, 0)
	s := NewSenderWithRecovery(2000, time.Second, RecoverySACKRACK)
	p := s.Enqueue(bytes.Repeat([]byte{0x7f}, 64*1024), now)

	if !s.tryMarkRetry(p, now.Add(time.Second), false) {
		t.Fatal("first repair unexpectedly denied")
	}
	if !s.tryMarkRetry(p, now.Add(2*time.Second), false) {
		t.Fatal("second repair unexpectedly denied inside burst+share budget")
	}
	if s.tryMarkRetry(p, now.Add(3*time.Second), false) {
		t.Fatal("third repair should be deferred by cumulative repair-byte budget")
	}
	st := s.Stats()
	if st.RepairDeferred != 1 || st.RepairDeferredBytes != 64*1024 {
		t.Fatalf("defer stats=%+v", st)
	}
	if p.RepairNotBefore.IsZero() {
		t.Fatal("budget denial did not pace the next repair attempt")
	}
}

func TestHighLossAllowsOnlyOneFastRepairBeforeRTO(t *testing.T) {
	s := NewSenderWithRecovery(3000, time.Second, RecoverySACKRACK)
	p := s.Enqueue([]byte("payload"), time.Unix(1700000200, 0))
	s.stats.Enqueued = 200
	s.stats.LossMarked = 20
	p.Retries = 1
	p.WasRetried = true

	if !s.highLoss() {
		t.Fatal("test setup did not classify high loss")
	}
	if s.fastRepairAllowed(p) {
		t.Fatal("high-loss path should not repeatedly fast-repair the same hole")
	}
}
