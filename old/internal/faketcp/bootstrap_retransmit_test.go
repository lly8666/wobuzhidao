package faketcp

import (
	"errors"
	"testing"
	"time"
)

func TestBootstrapRetransmitIsBoundedWithoutChangingSteadyStateBackoff(t *testing.T) {
	base := time.Unix(100, 0)
	s := NewSenderWithRecovery(1000, 10*time.Second, RecoveryLegacy)
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
		t.Fatalf("bootstrap retransmit changed steady-state RTO: %v", got)
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
