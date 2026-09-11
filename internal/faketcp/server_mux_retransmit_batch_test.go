package faketcp

import (
	"testing"
	"time"
)

func TestServerAssociationRetransmitDueBatchDrainsBoundedSweep(t *testing.T) {
	table, err := NewServerAssociationTable(1)
	if err != nil {
		t.Fatal(err)
	}
	a := handshakeMuxAssociation(t, table, 24001, 1000, 5000)

	sentAt := time.Unix(1_700_000_000, 0)
	for i := 0; i < 6; i++ {
		if _, err := a.EnqueueSteadyState([]byte{byte(i)}, sentAt); err != nil {
			t.Fatal(err)
		}
	}

	due := sentAt.Add(time.Second)
	first := a.RetransmitDueBatch(due, 4)
	if len(first) != 4 {
		t.Fatalf("first bounded RTO sweep returned %d packets, want 4", len(first))
	}
	second := a.RetransmitDueBatch(due, 4)
	if len(second) != 2 {
		t.Fatalf("second bounded RTO sweep returned %d packets, want 2", len(second))
	}
	if third := a.RetransmitDueBatch(due, 4); len(third) != 0 {
		t.Fatalf("drained RTO sweep returned %d extra packets", len(third))
	}
}
