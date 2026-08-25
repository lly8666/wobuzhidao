package faketcp

import (
	"testing"
	"time"
)

func TestReceiverDeliversOutOfOrderWithoutHOL(t *testing.T) {
	r := NewReceiver(100)
	if deliver, sack := r.Accept(110, 10); !deliver || !sack {
		t.Fatalf("later datagram should bypass hole: deliver=%v sack=%v", deliver, sack)
	}
	if r.Next() != 100 {
		t.Fatalf("cumulative ACK advanced across hole: %d", r.Next())
	}
	if deliver, sack := r.Accept(100, 10); !deliver || sack {
		t.Fatalf("missing datagram should deliver and close all holes: deliver=%v sack=%v", deliver, sack)
	}
	if r.Next() != 120 {
		t.Fatalf("expected cumulative ACK 120, got %d", r.Next())
	}
	if deliver, _ := r.Accept(110, 10); deliver {
		t.Fatal("retransmitted duplicate delivered twice")
	}
}

func TestReceiverSACKRangesMergeAndPersist(t *testing.T) {
	r := NewReceiver(100)
	if _, sack := r.Accept(120, 10); !sack { t.Fatal("120..130 should require SACK") }
	if _, sack := r.Accept(110, 10); !sack { t.Fatal("110..120 should require SACK") }
	var blocks [4]SACKBlock
	n := r.SACKBlocks(&blocks)
	if n != 1 || blocks[0] != (SACKBlock{Start:110, End:130}) {
		t.Fatalf("merged SACK=%v n=%d", blocks[:n], n)
	}
	if _, sack := r.Accept(150, 10); !sack { t.Fatal("150..160 should require SACK") }
	n = r.SACKBlocks(&blocks)
	if n != 2 || blocks[0] != (SACKBlock{Start:150, End:160}) || blocks[1] != (SACKBlock{Start:110, End:130}) {
		t.Fatalf("persistent SACK order=%v n=%d", blocks[:n], n)
	}
	// Repairing the first hole advances ACK through 110..130, but 150..160 is
	// still live. A real SACK TCP ACK must continue advertising that later data.
	if _, sack := r.Accept(100, 10); !sack { t.Fatal("later SACK state must survive partial hole repair") }
	if r.Next() != 130 { t.Fatalf("next=%d want 130", r.Next()) }
	n = r.SACKBlocks(&blocks)
	if n != 1 || blocks[0] != (SACKBlock{Start:150, End:160}) {
		t.Fatalf("consumed SACK range leaked: %v n=%d", blocks[:n], n)
	}
}

func TestSenderFastRetransmitAndAck(t *testing.T) {
	now := time.Unix(1, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue([]byte("0123456789"), now)
	_ = s.Enqueue([]byte("abcdefghij"), now)
	if got := s.Ack(100, now.Add(time.Millisecond)); got != nil {
		t.Fatal("fast retransmit too early")
	}
	if got := s.Ack(100, now.Add(2*time.Millisecond)); got != nil {
		t.Fatal("fast retransmit too early")
	}
	got := s.Ack(100, now.Add(3*time.Millisecond))
	if got != p1 || got.Retries != 1 {
		t.Fatalf("expected first segment fast retransmit, got %#v", got)
	}
	st := s.Stats()
	if st.Enqueued != 2 || st.EnqueuedBytes != 20 || st.LossMarked != 1 || st.LossMarkedBytes != 10 {
		t.Fatalf("unexpected first-loss accounting: %#v", st)
	}
	s.Ack(120, now.Add(30*time.Millisecond))
	if s.Pending() != 0 {
		t.Fatalf("pending=%d after cumulative ack", s.Pending())
	}
}

func TestSACKRetainsBytesUntilCumulativeAck(t *testing.T) {
	now := time.Unix(3, 0)
	s := NewSender(100, time.Second)
	_ = s.Enqueue(make([]byte, 10), now)
	p2 := s.Enqueue([]byte("abcdefghij"), now)
	s.AckSelective(100, []SACKBlock{{Start: 110, End: 120}}, now.Add(10*time.Millisecond))
	if !p2.SACKed {
		t.Fatal("segment not marked SACKed")
	}
	if len(p2.Payload) != 10 {
		t.Fatal("SACK incorrectly released retransmission bytes")
	}
	if s.Pending() != 2 {
		t.Fatalf("SACK must not reduce cumulatively-unacked pending count: %d", s.Pending())
	}
	s.AckSelective(120, nil, now.Add(20*time.Millisecond))
	if s.Pending() != 0 {
		t.Fatalf("pending=%d after cumulative ACK", s.Pending())
	}
	if p2.Payload != nil {
		t.Fatal("cumulative ACK did not release payload")
	}
}

func TestSenderSelectiveAckRepairsProvenHoleImmediately(t *testing.T) {
	now := time.Unix(4, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100..110 missing
	p2 := s.Enqueue(make([]byte, 10), now) // 110..120 received
	p3 := s.Enqueue(make([]byte, 10), now) // 120..130 received
	p4 := s.Enqueue(make([]byte, 10), now) // 130..140 received

	got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, now.Add(10*time.Millisecond))
	if !p2.SACKed || !p3.SACKed || !p4.SACKed {
		t.Fatal("SACK scoreboard missing received segments")
	}
	if got != p1 || got.Retries != 1 {
		t.Fatalf("three SACKed segments should infer first hole immediately, got %#v", got)
	}
	if s.Pending() != 4 {
		t.Fatalf("all bytes must remain until cumulative ACK, pending=%d", s.Pending())
	}
}

func TestSenderSACKRecoveryContinuesAfterCumulativeAdvance(t *testing.T) {
	now := time.Unix(6, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100 missing
	_ = s.Enqueue(make([]byte, 10), now)   // 110 received
	_ = s.Enqueue(make([]byte, 10), now)   // 120 received
	_ = s.Enqueue(make([]byte, 10), now)   // 130 received
	p5 := s.Enqueue(make([]byte, 10), now) // 140 missing
	_ = s.Enqueue(make([]byte, 10), now)   // 150 received
	_ = s.Enqueue(make([]byte, 10), now)   // 160 received
	_ = s.Enqueue(make([]byte, 10), now)   // 170 received

	got := s.AckSelective(100, []SACKBlock{{Start:110, End:140}, {Start:150, End:180}}, now.Add(10*time.Millisecond))
	if got != p1 {
		t.Fatalf("first loss recovery=%#v want p1", got)
	}
	// Once p1 arrives, the receiver cumulatively ACKs through 140 while still
	// advertising 150..180. The next SACK-proven hole must be repaired now, not
	// one or more exponentially backed-off RTOs later.
	got = s.AckSelective(140, []SACKBlock{{Start:150, End:180}}, now.Add(20*time.Millisecond))
	if got != p5 || got.Retries != 1 {
		t.Fatalf("second SACK-proven hole not chained after ACK advance: %#v", got)
	}
	if st := s.Stats(); st.FastRetransmits != 2 || st.LossMarked != 2 {
		t.Fatalf("unexpected chained recovery stats: %#v", st)
	}
}

func TestSenderRACKDetectsLostRetransmission(t *testing.T) {
	now := time.Unix(7, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100 lost, first repair will also be lost
	p2 := s.Enqueue(make([]byte, 10), now) // 110 lost, repair arrives later
	_ = s.Enqueue(make([]byte, 10), now)   // 120 received
	_ = s.Enqueue(make([]byte, 10), now)   // 130 received
	_ = s.Enqueue(make([]byte, 10), now)   // 140 received

	// Three later SACKed originals infer p1 at t=10ms.
	if got := s.AckSelective(100, []SACKBlock{{Start:120, End:150}}, now.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first scoreboard repair=%#v", got)
	}
	// Same persistent SACK evidence infers p2 at t=20ms; this transmission is
	// chronologically newer than p1's failed repair.
	if got := s.AckSelective(100, []SACKBlock{{Start:120, End:150}}, now.Add(20*time.Millisecond)); got != p2 {
		t.Fatalf("second scoreboard repair=%#v", got)
	}
	// p2's repair arrives and becomes a new SACK. Its LastSent timestamp is newer
	// than p1's repair, so after the conservative 10ms reordering window RACK
	// identifies the lost p1 retransmission without waiting for the 1s RTO.
	if got := s.AckSelective(100, []SACKBlock{{Start:110, End:150}}, now.Add(30*time.Millisecond)); got != p1 {
		t.Fatalf("RACK did not recover lost retransmission: %#v", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("p1 retries=%d want 2", p1.Retries)
	}
	if st := s.Stats(); st.FastRetransmits != 3 || st.RTOTransmits != 0 || st.LossMarked != 2 {
		t.Fatalf("unexpected RACK accounting: %#v", st)
	}
}

func TestSenderRTOBacksOffLikeTCP(t *testing.T) {
	now := time.Unix(2, 0)
	s := NewSender(7, time.Second)
	p := s.Enqueue([]byte{1, 2, 3}, now)
	if got := s.RetransmitDue(now.Add(999 * time.Millisecond)); got != nil {
		t.Fatal("early retransmit")
	}
	if got := s.RetransmitDue(now.Add(time.Second)); got != p {
		t.Fatal("expected first RTO retransmit")
	}
	if s.RTO() != 2*time.Second {
		t.Fatalf("RTO after first timeout=%v want 2s", s.RTO())
	}
	if got := s.RetransmitDue(now.Add(2 * time.Second)); got != nil {
		t.Fatal("retransmitted before backed-off RTO from last send")
	}
	if got := s.RetransmitDue(now.Add(3 * time.Second)); got != p {
		t.Fatal("expected second RTO retransmit")
	}
	if s.RTO() != 4*time.Second {
		t.Fatalf("RTO after second timeout=%v want 4s", s.RTO())
	}
	st := s.Stats()
	if st.LossMarked != 1 || st.LossMarkedBytes != 3 || st.RTOTransmits != 2 || st.RetransmitBytes != 6 {
		t.Fatalf("same lost segment must be marked once but retried twice: %#v", st)
	}
}

func TestRTTSampleDoesNotUseCumulativeSweepAcrossHole(t *testing.T) {
	now := time.Unix(5, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)
	p1.WasRetried = true
	s.AckSelective(130, nil, now.Add(5*time.Second))
	if got := s.RTO(); got != time.Second {
		t.Fatalf("cumulative sweep polluted RTO: %v", got)
	}
}
