package faketcp

import (
	"testing"
	"time"
)

func TestSenderRACKRequiresFreshDeliveryEvidenceForEveryRepair(t *testing.T) {
	now := time.Unix(9, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100..110 stays missing
	_ = s.Enqueue(make([]byte, 10), now)   // 110..120 delivered
	_ = s.Enqueue(make([]byte, 10), now)   // 120..130 delivered
	_ = s.Enqueue(make([]byte, 10), now)   // 130..140 delivered

	// Three later delivered segments prove the cumulative hole and authorize the
	// first scoreboard repair. That repair refreshes p1.LastSent to t=10ms.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, now.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first repair=%#v want p1", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("retries after first repair=%d want 1", p1.Retries)
	}

	// Replaying exactly the same SACK evidence cannot authorize another repair:
	// rackLatestTx still refers to a transmission sent before p1's last repair.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, now.Add(20*time.Millisecond)); got != nil {
		t.Fatalf("stale SACK self-triggered repair: %#v", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("stale SACK changed retries=%d want 1", p1.Retries)
	}

	// A newly sent segment is then delivered above the same cumulative hole. Its
	// newer transmit timestamp is fresh delivery evidence, so RACK may repair p1
	// again after the reordering window.
	_ = s.Enqueue(make([]byte, 10), now.Add(30*time.Millisecond)) // 140..150
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 150}}, now.Add(40*time.Millisecond)); got != p1 {
		t.Fatalf("second repair without RTO=%#v want p1", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("retries after second repair=%d want 2", p1.Retries)
	}

	// The evidence used for the second repair is stale immediately afterward.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 150}}, now.Add(55*time.Millisecond)); got != nil {
		t.Fatalf("second stale SACK self-triggered repair: %#v", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("second stale SACK changed retries=%d want 2", p1.Retries)
	}

	// There is no arbitrary two-repair ceiling: another genuinely newer
	// delivered transmission authorizes a third fast repair under the same fence.
	_ = s.Enqueue(make([]byte, 10), now.Add(60*time.Millisecond)) // 150..160
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 160}}, now.Add(70*time.Millisecond)); got != p1 {
		t.Fatalf("third fresh-evidence repair=%#v want p1", got)
	}
	if p1.Retries != 3 {
		t.Fatalf("retries after third repair=%d want 3", p1.Retries)
	}
	if st := s.Stats(); st.FastRetransmits != 3 || st.RTOTransmits != 0 || st.LossMarked != 1 {
		t.Fatalf("unexpected repeated fresh-evidence accounting: %#v", st)
	}
}
