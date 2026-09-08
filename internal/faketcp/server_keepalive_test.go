package faketcp

import (
	"testing"
	"time"
)

func establishedKeepaliveAssociation(t *testing.T) *ServerAssociation {
	t.Helper()
	clientIP := [4]byte{10, 92, 0, 2}
	serverIP := [4]byte{10, 92, 0, 1}
	syn := Segment{SrcIP: clientIP, DstIP: serverIP, SrcPort: 41001, DstPort: 40000, Seq: 1000, Flags: FlagSYN}
	a, err := NewServerAssociation(syn, 7000, RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ack := Segment{SrcIP: clientIP, DstIP: serverIP, SrcPort: 41001, DstPort: 40000, Seq: 1001, Ack: 7001, Flags: FlagACK}
	if err := a.HandleHandshakeACK(ack); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestServerAssociationACKsKeepaliveProbeWithoutAdvancingSequence(t *testing.T) {
	a := establishedKeepaliveAssociation(t)
	beforeRX := a.ReceiverNext()
	beforeTX := a.SenderNext()
	flow := a.Flow()
	probe := Segment{
		SrcIP: flow.ClientIP, DstIP: flow.ServerIP,
		SrcPort: flow.ClientPort, DstPort: flow.ServerPort,
		Seq: beforeRX - 1, Ack: beforeTX, Flags: FlagACK,
	}
	res, err := a.HandleSegment(probe, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !res.AckNeeded || res.Ack != beforeRX {
		t.Fatalf("keepalive response AckNeeded=%t Ack=%d want true/%d", res.AckNeeded, res.Ack, beforeRX)
	}
	if len(res.Deliver) != 0 {
		t.Fatalf("keepalive leaked %d bytes into payload delivery", len(res.Deliver))
	}
	if got := a.ReceiverNext(); got != beforeRX {
		t.Fatalf("keepalive consumed receive sequence: got=%d want=%d", got, beforeRX)
	}
	if got := a.SenderNext(); got != beforeTX {
		t.Fatalf("keepalive changed send sequence: got=%d want=%d", got, beforeTX)
	}
}

func TestServerAssociationOrdinaryACKDoesNotPingPong(t *testing.T) {
	a := establishedKeepaliveAssociation(t)
	flow := a.Flow()
	seg := Segment{
		SrcIP: flow.ClientIP, DstIP: flow.ServerIP,
		SrcPort: flow.ClientPort, DstPort: flow.ServerPort,
		Seq: a.ReceiverNext(), Ack: a.SenderNext(), Flags: FlagACK,
	}
	res, err := a.HandleSegment(seg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.AckNeeded {
		t.Fatal("ordinary ACK-only segment triggered ACK ping-pong")
	}
}
