package faketcp

import (
	"testing"
	"time"
)

func TestDetachedAssociationLeavesSteadyACKSequenceToRuntime(t *testing.T) {
	syn := Segment{
		SrcIP: [4]byte{10, 0, 0, 1}, DstIP: [4]byte{10, 0, 0, 2},
		SrcPort: 40000, DstPort: 443,
		Seq: 100, Flags: FlagSYN, Window: 65535,
		MSS: DefaultMSS, MSSSet: true, SACKPermitted: true,
		WindowScale: DefaultWindowScale, WindowScaleSet: true,
	}
	assoc, err := NewServerAssociation(syn, 9000, time.Second, func(Segment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer assoc.Close()

	ack := Segment{
		SrcIP: syn.SrcIP, DstIP: syn.DstIP,
		SrcPort: syn.SrcPort, DstPort: syn.DstPort,
		Seq: syn.Seq + 1, Ack: 9001, Flags: FlagACK, Window: 65535,
	}
	if err := assoc.HandleHandshakeACK(ack); err != nil {
		t.Fatal(err)
	}
	boundary, err := assoc.PrepareTransition(1500)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assoc.DetachTransition(); err != nil {
		t.Fatal(err)
	}

	steadyAck := ack
	steadyAck.Ack = assoc.SenderNext() + 100
	result, err := assoc.HandleSegment(steadyAck, time.Unix(100, 0))
	if err != nil {
		t.Fatalf("steady ACK was rejected by bootstrap sender: %v", err)
	}
	if result.Disposition != RouteAckOnly {
		t.Fatalf("steady ACK disposition=%v", result.Disposition)
	}

	steady := ack
	steady.Seq = boundary
	steady.Ack = assoc.SenderNext() + 200
	steady.Flags = FlagACK | FlagPSH
	steady.Payload = []byte("post-detach-record")
	result, err = assoc.HandleSegment(steady, time.Unix(101, 0))
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != RouteRecord || result.Record == nil ||
		result.Record.Seq != steady.Seq || string(result.Record.Payload) != string(steady.Payload) {
		t.Fatalf("steady record result=%+v", result)
	}
}
