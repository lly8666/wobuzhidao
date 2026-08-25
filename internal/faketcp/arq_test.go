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
	if r.Next() != 100 { t.Fatalf("cumulative ACK advanced across hole: %d", r.Next()) }
	if deliver, oo := r.Accept(100, 10); !deliver || oo {
		t.Fatalf("missing datagram should deliver and close hole: deliver=%v oo=%v", deliver, oo)
	}
	if r.Next() != 120 { t.Fatalf("expected cumulative ACK 120, got %d", r.Next()) }
	if deliver, _ := r.Accept(110, 10); deliver { t.Fatal("retransmitted duplicate delivered twice") }
}

func TestSenderFastRetransmitAndAck(t *testing.T) {
	now := time.Unix(1, 0)
	s := NewSender(100, 100*time.Millisecond)
	p1 := s.Enqueue([]byte("0123456789"), now)
	_ = s.Enqueue([]byte("abcdefghij"), now)
	if got := s.Ack(100, now.Add(time.Millisecond)); got != nil { t.Fatal("fast retransmit too early") }
	if got := s.Ack(100, now.Add(2*time.Millisecond)); got != nil { t.Fatal("fast retransmit too early") }
	got := s.Ack(100, now.Add(3*time.Millisecond))
	if got != p1 || got.Retries != 1 { t.Fatalf("expected first segment fast retransmit, got %#v", got) }
	s.Ack(120, now.Add(30*time.Millisecond))
	if s.Pending() != 0 { t.Fatalf("pending=%d after cumulative ack", s.Pending()) }
}

func TestSenderSelectiveAckLeavesOnlyHoles(t *testing.T) {
	now := time.Unix(3, 0)
	s := NewSender(100, 100*time.Millisecond)
	p1 := s.Enqueue(make([]byte, 10), now) // 100..110 missing
	_ = s.Enqueue(make([]byte, 10), now)   // 110..120 received
	p3 := s.Enqueue(make([]byte, 10), now) // 120..130 missing
	_ = s.Enqueue(make([]byte, 10), now)   // 130..140 received

	s.AckSelective(100, []SACKBlock{{Start:110, End:120}}, now.Add(10*time.Millisecond))
	got := s.AckSelective(100, []SACKBlock{{Start:130, End:140}}, now.Add(11*time.Millisecond))
	// Two duplicate ACKs so far; third triggers the actual first hole below.
	if got != nil { t.Fatal("fast retransmit too early") }
	got = s.AckSelective(100, nil, now.Add(12*time.Millisecond))
	if got != p1 { t.Fatalf("expected first hole, got %#v", got) }
	if s.Pending() != 2 { t.Fatalf("expected only two holes pending, got %d", s.Pending()) }

	// Once first hole is repaired the cumulative ACK lands on the second hole;
	// all already SACKed tail datagrams stay removed.
	s.AckSelective(120, nil, now.Add(20*time.Millisecond))
	if s.Pending() != 1 { t.Fatalf("expected one remaining hole, got %d", s.Pending()) }
	if got := s.RetransmitDue(now.Add(200*time.Millisecond)); got != p3 {
		t.Fatalf("expected second hole RTO, got %#v", got)
	}
}

func TestSenderRTO(t *testing.T) {
	now := time.Unix(2, 0)
	s := NewSender(7, 50*time.Millisecond)
	p := s.Enqueue([]byte{1,2,3}, now)
	if got := s.RetransmitDue(now.Add(49*time.Millisecond)); got != nil { t.Fatal("early retransmit") }
	if got := s.RetransmitDue(now.Add(50*time.Millisecond)); got != p { t.Fatal("expected RTO retransmit") }
	if p.Retries != 1 || !p.WasRetried { t.Fatalf("retry state %#v", p) }
}
