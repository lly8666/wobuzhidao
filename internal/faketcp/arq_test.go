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

func TestSenderFastRetransmitAndAck(t *testing.T) {
	now := time.Unix(1, 0)
	s := NewSender(100, 100*time.Millisecond)
	p1 := s.Enqueue([]byte("0123456789"), now)
	_ = s.Enqueue([]byte("abcdefghij"), now)
	if got := s.Ack(100, now.Add(time.Millisecond)); got != nil { t.Fatal("fast retransmit too early") }
	if got := s.Ack(100, now.Add(2*time.Millisecond)); got != nil { t.Fatal("fast retransmit too early") }
	got := s.Ack(100, now.Add(3*time.Millisecond))
	if got != p1 || got.Retries != 1 {
		t.Fatalf("expected first segment fast retransmit, got %#v", got)
	}
	s.Ack(120, now.Add(30*time.Millisecond))
	if s.Pending() != 0 {
		t.Fatalf("pending=%d after cumulative ack", s.Pending())
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
