package faketcp

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func p2SYN(clientPort uint16, seq uint32) Segment {
	return Segment{
		SrcIP: [4]byte{10, 0, 0, byte(clientPort%250 + 1)},
		DstIP: [4]byte{10, 0, 1, 1},
		SrcPort: clientPort,
		DstPort: 443,
		Seq: seq,
		Flags: FlagSYN,
		Window: 65535,
		MSS: DefaultMSS,
		MSSSet: true,
		SACKPermitted: true,
		WindowScale: DefaultWindowScale,
		WindowScaleSet: true,
	}
}

func establishP2(t *testing.T, emit SegmentEmitter) (*ServerAssociation, Segment) {
	t.Helper()
	syn := p2SYN(21001, 1000)
	a, err := NewServerAssociation(syn, 5000, 10*time.Second, emit)
	if err != nil {
		t.Fatal(err)
	}
	synack, err := a.SYNACKSegment()
	if err != nil {
		t.Fatal(err)
	}
	if synack.Seq != 5000 || synack.Ack != 1001 ||
		synack.SrcPort != 443 || synack.DstPort != 21001 ||
		synack.Flags != FlagSYN|FlagACK {
		t.Fatalf("bad SYNACK %#v", synack)
	}
	ack := syn
	ack.Flags = FlagACK
	ack.Seq = 1001
	ack.Ack = 5001
	if _, err := a.HandleSegment(ack, time.Now()); err != nil {
		t.Fatal(err)
	}
	if a.State() != ServerAssociationEstablished {
		t.Fatal("association did not establish")
	}
	return a, syn
}

func TestServerAssociationRejectsNonWBDAndCrossFlowHandshake(t *testing.T) {
	bad := p2SYN(21001, 1000)
	bad.MSS = 1460
	if _, err := NewServerAssociation(bad, 5000, time.Second, nil); !errors.Is(err, ErrBadServerSYN) {
		t.Fatalf("bad fingerprint err=%v", err)
	}

	a, syn := establishP2(t, func(Segment) error { return nil })
	wrong := p2SYN(21002, syn.Seq)
	wrong.Flags = FlagACK
	wrong.Seq = syn.Seq + 1
	wrong.Ack = 5001
	if _, err := a.HandleSegment(wrong, time.Now()); !errors.Is(err, ErrHandshakeState) {
		t.Fatalf("cross-flow err=%v", err)
	}
}

func TestDataBearingFinalACKFeedsFirstTLSBytes(t *testing.T) {
	syn := p2SYN(20501, 1200)
	a, err := NewServerAssociation(syn, 9000, time.Second, func(Segment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	first := syn
	first.Flags = FlagACK | FlagPSH
	first.Seq = 1201
	first.Ack = 9001
	first.Payload = []byte("tls-client-hello")

	res, err := a.HandleSegment(first, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Disposition != RouteBootstrap || !res.AckNeeded {
		t.Fatalf("result=%#v", res)
	}
	if want := uint32(1201 + len(first.Payload)); res.Ack != want {
		t.Fatalf("ack=%d want=%d", res.Ack, want)
	}

	got := make([]byte, len(first.Payload))
	if _, err := io.ReadFull(a.BootstrapConn(), got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, first.Payload) {
		t.Fatalf("bootstrap=%q want=%q", got, first.Payload)
	}
}

func TestBootstrapWriteUsesSameAssociationSequenceAndACKWait(t *testing.T) {
	var mu sync.Mutex
	var emitted []Segment
	emitCh := make(chan Segment, 4)
	a, _ := establishP2(t, func(seg Segment) error {
		mu.Lock()
		emitted = append(emitted, seg)
		mu.Unlock()
		emitCh <- seg
		return nil
	})

	writeDone := make(chan error, 1)
	go func() {
		_, err := a.BootstrapConn().Write([]byte("server-tls"))
		writeDone <- err
	}()

	var sent Segment
	select {
	case sent = <-emitCh:
	case <-time.After(time.Second):
		t.Fatal("bootstrap write emitted no segment")
	}
	if sent.Seq != 5001 || sent.Ack != 1001 ||
		sent.SrcPort != 443 || sent.DstPort != 21001 ||
		sent.Flags != FlagACK|FlagPSH ||
		!bytes.Equal(sent.Payload, []byte("server-tls")) {
		t.Fatalf("emitted segment %#v", sent)
	}

	select {
	case err := <-writeDone:
		t.Fatalf("write returned before ACK: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	ack := p2SYN(21001, 0)
	ack.Flags = FlagACK
	ack.Seq = 1001
	ack.Ack = sent.Seq + uint32(len(sent.Payload))
	if _, err := a.HandleSegment(ack, time.Now()); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap write did not unblock after ACK")
	}
	if got := a.SenderLastAck(); got != ack.Ack {
		t.Fatalf("lastAck=%d want=%d", got, ack.Ack)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("emitted=%d want=1", len(emitted))
	}
}

func TestAssociationPrepareQueuesEarlyRecordAndDetachTransfersIt(t *testing.T) {
	a, syn := establishP2(t, func(Segment) error { return nil })

	clientTLS := syn
	clientTLS.Flags = FlagACK | FlagPSH
	clientTLS.Seq = 1001
	clientTLS.Ack = 5001
	clientTLS.Payload = []byte("final-request")
	res, err := a.HandleSegment(clientTLS, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Disposition != RouteBootstrap {
		t.Fatalf("bootstrap disposition=%v", res.Disposition)
	}

	buf := make([]byte, len(clientTLS.Payload))
	if _, err := io.ReadFull(a.BootstrapConn(), buf); err != nil {
		t.Fatal(err)
	}
	boundary, err := a.PrepareTransition(64)
	if err != nil {
		t.Fatal(err)
	}
	if want := clientTLS.Seq + uint32(len(clientTLS.Payload)); boundary != want {
		t.Fatalf("boundary=%d want=%d", boundary, want)
	}

	early := clientTLS
	early.Seq = boundary
	early.Payload = []byte("tlslike-record")
	res, err = a.HandleSegment(early, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Disposition != RouteQueuedRecord || res.Ack != boundary {
		t.Fatalf("early result=%#v", res)
	}

	queued, err := a.DetachTransition()
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Seq != boundary ||
		!bytes.Equal(queued[0].Payload, early.Payload) {
		t.Fatalf("queued=%#v", queued)
	}

	post := early
	post.Seq += uint32(len(early.Payload))
	post.Payload = []byte("record-2")
	res, err = a.HandleSegment(post, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Disposition != RouteRecord || res.Record == nil ||
		!bytes.Equal(res.Record.Payload, post.Payload) {
		t.Fatalf("post-detach result=%#v", res)
	}
}

func TestAssociationPureACKAlwaysStaysInFakeTCPDuringTransition(t *testing.T) {
	a, syn := establishP2(t, func(Segment) error { return nil })
	if _, err := a.PrepareTransition(64); err != nil {
		t.Fatal(err)
	}
	ack := syn
	ack.Flags = FlagACK
	ack.Seq = 1001
	ack.Ack = 5001
	res, err := a.HandleSegment(ack, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Disposition != RouteAckOnly || res.Record != nil {
		t.Fatalf("pure ACK escaped FakeTCP path: %#v", res)
	}
}

func TestServerAssociationTableFlowIsolationAndCap(t *testing.T) {
	table, err := NewServerAssociationTable(2, func(Segment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	a, err := table.AddSYN(p2SYN(22001, 100), 1000, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.AddSYN(p2SYN(22001, 100), 2000, time.Second); !errors.Is(err, ErrAssociationExists) {
		t.Fatalf("duplicate err=%v", err)
	}
	b, err := table.AddSYN(p2SYN(22002, 200), 3000, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Flow() == b.Flow() {
		t.Fatal("distinct clients share flow")
	}
	if _, err := table.AddSYN(p2SYN(22003, 300), 4000, time.Second); !errors.Is(err, ErrMuxFull) {
		t.Fatalf("full err=%v", err)
	}
	if !table.Remove(a.Flow()) || table.Len() != 1 {
		t.Fatal("remove failed")
	}
	if _, err := table.AddSYN(p2SYN(22003, 300), 4000, time.Second); err != nil {
		t.Fatalf("slot not reusable: %v", err)
	}
}
