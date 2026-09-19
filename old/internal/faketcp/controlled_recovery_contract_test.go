package faketcp

import (
	"bytes"
	"testing"
	"time"
)

func TestControlledRecoveryCompletesBeforeForgiveness(t *testing.T) {
	for _, mode := range []RecoveryMode{RecoveryLegacy, RecoverySACKRACK} {
		t.Run(recoveryModeName(mode), func(t *testing.T) {
			const (
				start  = uint32(7000)
				record = 200
				count  = 8
			)
			now := time.Unix(40, 0)
			s := NewSenderWithRecovery(start, time.Second, mode)
			r := NewReceiver(start)
			r.EnableSteadyStateDelivery()

			pending := make([]*Pending, 0, count)
			for i := 0; i < count; i++ {
				payload := bytes.Repeat([]byte{byte(i + 1)}, record)
				p, err := s.EnqueueSteadyState(payload, now)
				if err != nil {
					t.Fatal(err)
				}
				pending = append(pending, p)
			}

			var repair *Pending
			deliver := func(p *Pending, at time.Time) {
				t.Helper()
				if p == nil || len(p.Payload) == 0 {
					t.Fatal("repair ownership was released before delivery")
				}
				ok, sackNeeded := r.AcceptAt(p.Seq, len(p.Payload), at, 500*time.Millisecond)
				if !ok {
					t.Fatalf("first arrival seq=%d was not delivered", p.Seq)
				}
				var blocks [4]SACKBlock
				n := 0
				if sackNeeded {
					n = r.SACKBlocks(&blocks)
				}
				if candidate := s.AckSelective(r.Next(), blocks[:n], at); candidate != nil {
					repair = candidate
				}
			}

			// Deliver the first record, lose the second, then deliver enough later
			// records to produce explicit SACK/dup-ACK recovery evidence. The trace is
			// far below both adaptive and emergency forgiveness horizons.
			deliver(pending[0], now.Add(time.Millisecond))
			for i := 2; i < count; i++ {
				deliver(pending[i], now.Add(time.Duration(i)*time.Millisecond))
			}
			if repair == nil || repair.Seq != pending[1].Seq {
				t.Fatalf("mode=%v repair=%#v want seq=%d", mode, repair, pending[1].Seq)
			}
			if !bytes.Equal(repair.Payload, bytes.Repeat([]byte{2}, record)) {
				t.Fatal("retransmission payload ownership/data changed before repair")
			}
			if st := r.Stats(); st.ForgivenGaps != 0 {
				t.Fatalf("controlled recovery incorrectly used forgiveness: %+v", st)
			}

			// Explicitly deliver the selected repair. All records must then become
			// cumulatively acknowledged; finite recovery is not an excuse for loss
			// when the repair budget and timing are sufficient.
			deliver(repair, now.Add(20*time.Millisecond))
			if got, want := r.Next(), s.NextSeq(); got != want {
				t.Fatalf("receiver next=%d sender next=%d", got, want)
			}
			if got := s.Pending(); got != 0 {
				t.Fatalf("pending repair state after complete recovery=%d", got)
			}
			if got := r.Stats().Delivered; got != count {
				t.Fatalf("delivered=%d want=%d", got, count)
			}
		})
	}
}
