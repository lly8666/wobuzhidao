package faketcp

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestBootstrapRetransmitIsBoundedWithoutChangingLaterBackoff(t *testing.T) {
	base := time.Unix(100, 0)
	s := NewSender(1000, 10*time.Second)
	var bootstrapPending *Pending

	stream, err := NewBootstrapStream(5000, func(payload []byte) (uint32, error) {
		bootstrapPending = s.Enqueue(payload, base)
		return bootstrapPending.End, nil
	}, func(uint32, time.Time) error {
		return ErrBootstrapClosed
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("tls-bootstrap")); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("write err=%v want %v", err, ErrBootstrapClosed)
	}
	if bootstrapPending == nil || !bootstrapPending.Bootstrap {
		t.Fatalf("bootstrap pending not tagged: %#v", bootstrapPending)
	}

	if got := s.RetransmitDue(base.Add(bootstrapRetransmitCeiling - time.Millisecond)); got != nil {
		t.Fatal("bootstrap retransmit fired before ceiling")
	}
	if got := s.RetransmitDue(base.Add(bootstrapRetransmitCeiling)); got != bootstrapPending {
		t.Fatalf("first bootstrap retransmit=%#v want %#v", got, bootstrapPending)
	}
	if got := s.RTO(); got != 10*time.Second {
		t.Fatalf("bootstrap retransmit changed shared RTO: %v", got)
	}
	if got := s.RetransmitDue(base.Add(2 * bootstrapRetransmitCeiling)); got != bootstrapPending {
		t.Fatalf("second bootstrap retransmit=%#v want %#v", got, bootstrapPending)
	}
	if got := s.RTO(); got != 10*time.Second {
		t.Fatalf("bootstrap retransmit backed off shared RTO: %v", got)
	}
	if st := s.Stats(); st.RTOTransmits != 2 {
		t.Fatalf("bootstrap RTO stats=%#v", st)
	}

	ackAt := base.Add(2*bootstrapRetransmitCeiling + time.Millisecond)
	s.Ack(bootstrapPending.End, ackAt)
	normalSent := ackAt.Add(time.Second)
	normal := s.Enqueue([]byte("data"), normalSent)
	if normal.Bootstrap {
		t.Fatal("normal data inherited bootstrap marker")
	}
	if got := s.RetransmitDue(normalSent.Add(10 * time.Second)); got != normal {
		t.Fatalf("normal retransmit=%#v want %#v", got, normal)
	}
	if got := s.RTO(); got != 20*time.Second {
		t.Fatalf("normal RTO backoff=%v want 20s", got)
	}
}

func TestSenderWaitAckUnblocksOnCumulativeAck(t *testing.T) {
	base := time.Now()
	s := NewSender(100, 3*time.Second)
	p := s.Enqueue([]byte("abc"), base)

	done := make(chan error, 1)
	go func() {
		done <- s.WaitAck(p.End, time.Now().Add(time.Second))
	}()

	select {
	case err := <-done:
		t.Fatalf("WaitAck returned before ACK: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	s.Ack(p.End, time.Now())
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitAck did not unblock")
	}
}

func TestSenderWaitAckDeadline(t *testing.T) {
	s := NewSender(100, 3*time.Second)
	p := s.Enqueue([]byte("abc"), time.Now())
	err := s.WaitAck(p.End, time.Now().Add(5*time.Millisecond))
	if !errors.Is(err, ErrBootstrapTimeout) {
		t.Fatalf("err=%v want timeout", err)
	}
}

func TestBootstrapRetransmitKeepsSeqAndPayloadIdentical(t *testing.T) {
	base := time.Unix(200, 0)
	s := NewSender(1000, 30*time.Second)
	var pending *Pending
	stream, err := NewBootstrapStream(1, func(payload []byte) (uint32, error) {
		pending = s.Enqueue(payload, base)
		return pending.End, nil
	}, func(uint32, time.Time) error { return ErrBootstrapClosed }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("immutable-bootstrap-ciphertext")
	_, _ = stream.Write(original)
	if pending == nil {
		t.Fatal("no pending payload")
	}
	wantSeq := pending.Seq
	wantPayload := append([]byte(nil), pending.Payload...)

	got := s.RetransmitDue(base.Add(bootstrapRetransmitCeiling))
	if got != pending {
		t.Fatalf("got %#v want pending", got)
	}
	if got.Seq != wantSeq || !bytes.Equal(got.Payload, wantPayload) {
		t.Fatalf("retransmit changed seq/payload: seq=%d payload=%x", got.Seq, got.Payload)
	}
}

func TestSenderCumulativeAckAcrossSequenceWrap(t *testing.T) {
	s := NewSender(^uint32(0)-1, 2*time.Second)
	p := s.Enqueue([]byte{1, 2, 3}, time.Now())
	if p.End != 1 {
		t.Fatalf("wrapped end=%d want=1", p.End)
	}
	s.Ack(1, time.Now())
	if got := s.Pending(); got != 0 {
		t.Fatalf("pending=%d want=0", got)
	}
	if got := s.LastAck(); got != 1 {
		t.Fatalf("lastAck=%d want=1", got)
	}
}
