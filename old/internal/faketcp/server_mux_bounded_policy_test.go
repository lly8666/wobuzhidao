package faketcp

import (
	"testing"
	"time"
)

func TestServerMuxUsesSameBoundedSteadyStatePolicyForAllSenderModes(t *testing.T) {
	for _, mode := range []RecoveryMode{RecoveryLegacy, RecoverySACKRACK} {
		t.Run(recoveryModeName(mode), func(t *testing.T) {
			const (
				clientISN = uint32(3000)
				serverISN = uint32(9000)
				record    = 10
			)
			table, err := NewServerAssociationTable(1)
			if err != nil {
				t.Fatal(err)
			}
			syn := muxSYN(24001+uint16(mode), clientISN)
			a, err := table.AddSYN(syn, serverISN, mode, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			ack := syn
			ack.Flags = FlagACK
			ack.Seq = clientISN + 1
			ack.Ack = serverISN + 1
			if err := a.HandleHandshakeACK(ack); err != nil {
				t.Fatal(err)
			}
			a.EnableSteadyStateDelivery()

			start := clientISN + 1
			now := time.Unix(50, 0)
			for i := 1; i <= PartialReliabilityEmergencyLimit; i++ {
				seg := syn
				seg.Flags = FlagACK | FlagPSH
				seg.Seq = start + uint32(i*record)
				seg.Ack = serverISN + 1
				seg.Payload = make([]byte, record)
				res, err := a.HandleSegment(seg, now)
				if err != nil {
					t.Fatalf("record %d: %v", i, err)
				}
				if len(res.Deliver) != record {
					t.Fatalf("record %d was not immediately delivered", i)
				}
				now = now.Add(time.Millisecond)
			}
			st := a.ReceiverStats()
			if st.ForgivenGaps == 0 || st.EmergencyForgivenGaps == 0 {
				t.Fatalf("mux mode=%v did not use bounded steady-state forgiveness: %+v", mode, st)
			}
			if a.ReceiverNext() <= start {
				t.Fatalf("mux mode=%v did not advance past abandoned gap", mode)
			}
		})
	}
}
