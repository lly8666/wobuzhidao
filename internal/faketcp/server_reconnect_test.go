package faketcp

import (
	"testing"
	"time"
)

func TestClassifyPeerSYNSeparatesRetransmitFromNewIncarnation(t *testing.T) {
	const clientISN uint32 = 1000
	syn := muxSYN(24001, clientISN)
	a, err := NewServerAssociation(syn, 9000, RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.ClassifyPeerSYN(syn); got != PeerSYNRetransmit {
		t.Fatalf("original SYN class=%v want retransmit", got)
	}

	newSyn := syn
	newSyn.Seq = clientISN + 777
	if got := a.ClassifyPeerSYN(newSyn); got != PeerSYNNewIncarnation {
		t.Fatalf("new-ISN SYN class=%v want new incarnation", got)
	}

	ack := syn
	ack.Flags = FlagACK
	ack.Seq = clientISN + 1
	ack.Ack = 9001
	if err := a.HandleHandshakeACK(ack); err != nil {
		t.Fatal(err)
	}
	if got := a.ClassifyPeerSYN(syn); got != PeerSYNRetransmit {
		t.Fatalf("established delayed original SYN class=%v want retransmit", got)
	}
	if got := a.ClassifyPeerSYN(newSyn); got != PeerSYNNewIncarnation {
		t.Fatalf("established reconnect SYN class=%v want new incarnation", got)
	}
}

func TestClassifyPeerSYNRejectsOtherFlowAndNonSYN(t *testing.T) {
	syn := muxSYN(24002, 2000)
	a, err := NewServerAssociation(syn, 10000, RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	other := muxSYN(24003, 3000)
	if got := a.ClassifyPeerSYN(other); got != PeerSYNInvalid {
		t.Fatalf("other flow class=%v want invalid", got)
	}
	nonSyn := syn
	nonSyn.Flags = FlagACK
	if got := a.ClassifyPeerSYN(nonSyn); got != PeerSYNInvalid {
		t.Fatalf("non-SYN class=%v want invalid", got)
	}
}
