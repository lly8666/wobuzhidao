package faketcp

import (
	"testing"
	"time"
)

func TestReceiverDeliversOutOfOrderWithoutHOL(t *testing.T) {
	r := NewReceiver(100)
	if deliver, oo := r.Accept(110, 10); !deliver || !oo {
		t.Fatalf("later datagram should bypass hole: deliver=%v oo=%v", deliver, oo)
	}
	if r.Next() != 100 {
		t.Fatalf("cumulative ACK advanced across hole: %d", r.Next())
	}
	if deliver, oo := r.Accept(100, 10); !deliver || oo {
		t.Fatalf("missing datagram should deliver and close hole: deliver=%v oo=%v", deliver, oo)
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
	if _, oo := r.Accept(120, 10); !oo { t.Fatal("120..130 should be out of order") }
	if _, oo := r.Accept(110, 10); !oo { t.Fatal("110..120 should be out of order") }
	var blocks [4]SACKBlock
	n := r.SACKBlocks(&blocks)
	if n != 1 || blocks[0] != (SACKBlock{Start:110, End:130}) {
		t.Fatalf("merged SACK=%v n=%d", blocks[:n], n)
	}
	if _, oo := r.Accept(150, 10); !oo { t.Fatal("150..160 should be out of order") }
	n = r.SACKBlocks(&blocks)
	if n != 2 || blocks[0] != (SACKBlock{Start:150, End:160}) || blocks[1] != (SACKBlock{Start:110, End:130}) {
		t.Fatalf("persistent SACK order=%v n=%d", blocks[:n], n)
	}
	if _, oo := r.Accept(100, 10); oo { t.Fatal("100..110 closes first hole") }
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

func TestSenderSelectiveAckLeavesHolesRetransmittable(t *testing.T) {
	now := time.Unix(4, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100..110 missing
	p2 := s.Enqueue(make([]byte, 10), now) // 110..120 received
	_ = s.Enqueue(make([]byte, 10), now)   // 120..130 missing
	p4 := s.Enqueue(make([]byte, 10), now) // 130..140 received

	s.AckSelective(100, []SACKBlock{{Start: 110, End: 120}, {Start: 130, End: 140}}, now.Add(10*time.Millisecond))
	if !p2.SACKed || !p4.SACKed {
		t.Fatal("SACK scoreboard missing received segments")
	}
	if s.Pending() != 4 {
		t.Fatalf("all bytes must remain until cumulative ACK, pending=%d", s.Pending())
	}
	s.AckSelective(100, nil, now.Add(11*time.Millisecond))
	got := s.AckSelective(100, nil, now.Add(12*time.Millisecond))
	if got != p1 {
		t.Fatalf("expected SND.UNA fast retransmit, got %#v", got)
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
