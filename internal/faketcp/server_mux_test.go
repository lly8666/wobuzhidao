package faketcp

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func muxSYN(clientPort uint16, seq uint32) Segment {
	return Segment{
		SrcIP: [4]byte{10, 0, 0, byte(clientPort%250 + 1)},
		DstIP: [4]byte{10, 0, 1, 1},
		SrcPort: clientPort,
		DstPort: 443,
		Seq: seq,
		Flags: FlagSYN,
	}
}

func handshakeMuxAssociation(t *testing.T, table *ServerAssociationTable, clientPort uint16, clientISN, serverISN uint32) *ServerAssociation {
	t.Helper()
	syn := muxSYN(clientPort, clientISN)
	a, err := table.AddSYN(syn, serverISN, RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	seq, ack, err := a.SYNACK()
	if err != nil || seq != serverISN || ack != clientISN+1 {
		t.Fatalf("synack seq=%d ack=%d err=%v", seq, ack, err)
	}
	ackSeg := syn
	ackSeg.Flags = FlagACK
	ackSeg.Seq = clientISN + 1
	ackSeg.Ack = serverISN + 1
	if err := a.HandleHandshakeACK(ackSeg); err != nil {
		t.Fatal(err)
	}
	if a.State() != ServerAssociationEstablished {
		t.Fatal("association did not establish")
	}
	return a
}

func TestServerAssociationTableAllowsManyClientsOnOnePublicPort(t *testing.T) {
	const n = 32
	table, err := NewServerAssociationTable(n)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			syn := muxSYN(uint16(20000+i), uint32(1000+i*100))
			a, err := table.AddSYN(syn, uint32(50000+i*100), RecoveryLegacy, time.Second)
			if err != nil {
				errCh <- err
				return
			}
			ack := syn
			ack.Flags = FlagACK
			ack.Seq = syn.Seq + 1
			ack.Ack = uint32(50000+i*100) + 1
			errCh <- a.HandleHandshakeACK(ack)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if table.Len() != n {
		t.Fatalf("len=%d want=%d", table.Len(), n)
	}
	for i := 0; i < n; i++ {
		syn := muxSYN(uint16(20000+i), uint32(1000+i*100))
		a, ok := table.GetSegment(syn)
		if !ok || a.State() != ServerAssociationEstablished {
			t.Fatalf("flow %d missing/invalid", i)
		}
	}
}

func TestServerAssociationAcceptsDataBearingFinalACK(t *testing.T) {
	const clientISN uint32 = 1200
	const serverISN uint32 = 9000
	table, _ := NewServerAssociationTable(1)
	syn := muxSYN(20501, clientISN)
	a, err := table.AddSYN(syn, serverISN, RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// The pure third ACK may be lost. TCP permits the first application data to
	// acknowledge the SYN-ACK, so the WBD server must establish instead of
	// rejecting every retransmission of the first DTLS ClientHello payload.
	first := syn
	first.Flags = FlagACK | FlagPSH
	first.Seq = clientISN + 1
	first.Ack = serverISN + 1
	first.Payload = []byte("dtls-client-hello")
	if err := a.HandleHandshakeACK(first); err != nil {
		t.Fatalf("data-bearing final ACK rejected: %v", err)
	}
	if a.State() != ServerAssociationEstablished {
		t.Fatal("data-bearing final ACK did not establish association")
	}

	// The outer mux in the compatibility path may have consumed the handshake
	// segment before delivery. Normal FakeTCP ARQ retransmission must then be
	// accepted and delivered, rather than leaving DTLS to time out forever.
	res, err := a.HandleSegment(first, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Deliver) != string(first.Payload) || !res.AckNeeded {
		t.Fatalf("retransmitted first payload deliver=%q ack_needed=%v", res.Deliver, res.AckNeeded)
	}
	if want := clientISN + 1 + uint32(len(first.Payload)); res.Ack != want {
		t.Fatalf("ack=%d want=%d", res.Ack, want)
	}
}

func TestServerAssociationsKeepSequenceAndHOLStateIndependent(t *testing.T) {
	table, _ := NewServerAssociationTable(2)
	a := handshakeMuxAssociation(t, table, 21001, 1000, 5000)
	b := handshakeMuxAssociation(t, table, 21002, 2000, 9000)

	// Client A sends its second datagram before its first. It must still be
	// delivered immediately without advancing A's cumulative ACK across the hole.
	segA := muxSYN(21001, 0)
	segA.Flags = FlagACK | FlagPSH
	segA.Seq = 1011
	segA.Ack = 5001
	segA.Payload = []byte("A-later")
	resA, err := a.HandleSegment(segA, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if string(resA.Deliver) != "A-later" || resA.Ack != 1001 || resA.SACKN == 0 {
		t.Fatalf("A result deliver=%q ack=%d sacks=%d", resA.Deliver, resA.Ack, resA.SACKN)
	}

	// Client B remains completely independent and advances normally.
	segB := muxSYN(21002, 0)
	segB.Flags = FlagACK | FlagPSH
	segB.Seq = 2001
	segB.Ack = 9001
	segB.Payload = []byte("B-first")
	resB, err := b.HandleSegment(segB, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if string(resB.Deliver) != "B-first" || resB.Ack != 2001+uint32(len(segB.Payload)) || resB.SACKN != 0 {
		t.Fatalf("B result deliver=%q ack=%d sacks=%d", resB.Deliver, resB.Ack, resB.SACKN)
	}
	if a.ReceiverNext() != 1001 {
		t.Fatalf("A cumulative ack contaminated=%d", a.ReceiverNext())
	}
	if b.ReceiverNext() == a.ReceiverNext() {
		t.Fatal("two associations unexpectedly share receiver state")
	}

	pa, err := a.Enqueue([]byte("to-A"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pb, err := b.Enqueue([]byte("to-B"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if pa.Seq != 5001 || pb.Seq != 9001 {
		t.Fatalf("independent send seqs A=%d B=%d", pa.Seq, pb.Seq)
	}
}

func TestServerAssociationTableRejectsDuplicateAndCap(t *testing.T) {
	table, _ := NewServerAssociationTable(1)
	syn := muxSYN(22001, 100)
	if _, err := table.AddSYN(syn, 1000, RecoveryLegacy, time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := table.AddSYN(syn, 2000, RecoveryLegacy, time.Second); !errors.Is(err, ErrAssociationExists) {
		t.Fatalf("duplicate flow err=%v", err)
	}
	if _, err := table.AddSYN(muxSYN(22002, 200), 3000, RecoveryLegacy, time.Second); !errors.Is(err, ErrMuxFull) {
		t.Fatalf("full table err=%v", err)
	}
	if !table.Remove(ServerFlowFromSegment(syn)) || table.Len() != 0 {
		t.Fatal("remove failed")
	}
	if _, err := table.AddSYN(muxSYN(22002, 200), 3000, RecoveryLegacy, time.Second); err != nil {
		t.Fatalf("slot not reusable after remove: %v", err)
	}
}

func TestServerAssociationDoesNotAcceptOtherFlowHandshake(t *testing.T) {
	table, _ := NewServerAssociationTable(2)
	syn := muxSYN(23001, 500)
	a, err := table.AddSYN(syn, 900, RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wrong := muxSYN(23002, 500)
	wrong.Flags = FlagACK
	wrong.Ack = 901
	if err := a.HandleHandshakeACK(wrong); !errors.Is(err, ErrHandshakeState) {
		t.Fatalf("cross-flow handshake err=%v", err)
	}
	if a.State() != ServerAssociationAwaitACK {
		t.Fatal("bad peer changed association state")
	}
}
