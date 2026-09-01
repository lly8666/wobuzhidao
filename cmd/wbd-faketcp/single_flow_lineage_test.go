package main

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// lineageRawPacketIO is a deterministic raw peer for the common endpoint state
// machine. It answers exactly one WBD SYN with a WBD SYNACK and cumulatively
// ACKs every later payload. The endpoint under test still creates every client
// packet itself, so this catches a second SYN, tuple change, or sequence reset
// introduced by either the bootstrap adapter or the post-bootstrap mode switch.
type lineageRawPacketIO struct {
	mu sync.Mutex

	sourceIP, remoteIP     [4]byte
	sourcePort, remotePort uint16
	serverISN              uint32
	queue                  [][]byte
	writes                 [][]byte
}

func (r *lineageRawPacketIO) ReadPacket(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return 0, errRawTimeout
	}
	pkt := r.queue[0]
	r.queue = r.queue[1:]
	if len(pkt) > len(buf) {
		return 0, io.ErrShortBuffer
	}
	copy(buf, pkt)
	return len(pkt), nil
}

func (r *lineageRawPacketIO) WritePacket(packet []byte, _ [4]byte) error {
	seg, err := faketcp.ParseIPv4TCP(packet)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, append([]byte(nil), packet...))

	if seg.Flags&faketcp.FlagSYN != 0 && seg.Flags&faketcp.FlagACK == 0 {
		synAck := faketcp.MarshalIPv4TCP(
			r.remoteIP, r.sourceIP,
			r.remotePort, r.sourcePort,
			r.serverISN, seg.Seq+1,
			faketcp.FlagSYN|faketcp.FlagACK,
			65535, nil, 3,
		)
		r.queue = append(r.queue, synAck)
		return nil
	}

	if len(seg.Payload) != 0 {
		ack := faketcp.MarshalIPv4TCP(
			r.remoteIP, r.sourceIP,
			r.remotePort, r.sourcePort,
			r.serverISN+1, seg.Seq+uint32(len(seg.Payload)),
			faketcp.FlagACK,
			65535, nil, 4,
		)
		r.queue = append(r.queue, ack)
	}
	return nil
}

func (*lineageRawPacketIO) SetReadTimeout(time.Duration) error { return nil }
func (*lineageRawPacketIO) ClearReadTimeout() error            { return nil }
func (*lineageRawPacketIO) Close() error                       { return nil }

func (r *lineageRawPacketIO) snapshotWrites(t *testing.T) []faketcp.Segment {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]faketcp.Segment, 0, len(r.writes))
	for _, pkt := range r.writes {
		seg, err := faketcp.ParseIPv4TCP(pkt)
		if err != nil {
			t.Fatalf("parse captured client packet: %v", err)
		}
		out = append(out, seg)
	}
	return out
}

// TestSingleFlowLineageContract is deliberately OS-neutral and is run on both
// Windows and Linux CI. It locks the product invariant that the temporary
// reliable Reality-like bootstrap and the no-HOL steady-state payload share one
// TCP-shaped incarnation: one SYN, one four-tuple, one monotonically continuing
// sequence space, and no reconnect at the mode barrier.
func TestSingleFlowLineageContract(t *testing.T) {
	sourceIP := [4]byte{192, 0, 2, 20}
	remoteIP := [4]byte{198, 51, 100, 10}
	const sourcePort uint16 = 41001
	const remotePort uint16 = 443
	const serverISN uint32 = 0x49f0c003

	raw := &lineageRawPacketIO{
		sourceIP: sourceIP, remoteIP: remoteIP,
		sourcePort: sourcePort, remotePort: remotePort,
		serverISN: serverISN,
	}
	e := &endpoint{
		cfg:          config{role: "client", recovery: "legacy"},
		raw:          raw,
		srcIP:        sourceIP,
		dstIP:        remoteIP,
		srcPort:      sourcePort,
		dstPort:      remotePort,
		sendBuf:      make([]byte, 65535),
		stop:         make(chan struct{}),
		bootstrapAck: make(chan struct{}, 1),
	}
	defer e.close()

	if err := e.handshakeClient(); err != nil {
		t.Fatalf("FakeTCP handshake: %v", err)
	}

	rawErr := make(chan error, 1)
	go func() { rawErr <- e.rawLoop() }()

	stream, err := e.newBootstrapStream()
	if err != nil {
		t.Fatal(err)
	}
	e.setBootstrap(stream)
	if err := stream.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}

	// A TLS-looking setup prefix plus enough bytes to exercise multiple reliable
	// bootstrap chunks. The actual Firefox-120 TLS transcript is separately
	// exercised by internal/realityfront; this test owns the FakeTCP lineage.
	bootstrap := []byte{0x16, 0x03, 0x01, 0x02, 0x00, 0x01, 0x00, 0x01, 0xfc}
	bootstrap = append(bootstrap, bytes.Repeat([]byte{0x5a}, faketcp.DefaultBootstrapChunk+137)...)
	if n, err := stream.Write(bootstrap); err != nil || n != len(bootstrap) {
		t.Fatalf("bootstrap write n=%d err=%v", n, err)
	}

	// The mode barrier discards only the ordered adapter. The Sender/Receiver and
	// therefore the public TCP-shaped sequence lineage must remain untouched.
	e.clearBootstrap(stream)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	postBootstrap := []byte{0x16, 0xfe, 0xfd, 0x00, 0x00, 'D', 'T', 'L', 'S', '-', 'D', 'A', 'T', 'A'}
	e.senderMu.Lock()
	pending := e.sender.Enqueue(postBootstrap, time.Now())
	postEnd := pending.End
	err = e.sendDataPending(pending)
	e.senderMu.Unlock()
	if err != nil {
		t.Fatalf("post-bootstrap send: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		e.senderMu.Lock()
		acked := e.sender.LastAck() == postEnd
		e.senderMu.Unlock()
		if acked {
			break
		}
		time.Sleep(time.Millisecond)
	}
	e.senderMu.Lock()
	acked := e.sender.LastAck()
	e.senderMu.Unlock()
	if acked != postEnd {
		t.Fatalf("post-bootstrap ACK=%d want=%d", acked, postEnd)
	}

	segments := raw.snapshotWrites(t)
	if len(segments) < 5 {
		t.Fatalf("captured only %d client packets", len(segments))
	}

	var syns []faketcp.Segment
	var finalACK *faketcp.Segment
	var data []faketcp.Segment
	for i := range segments {
		seg := segments[i]
		if seg.SrcIP != sourceIP || seg.DstIP != remoteIP || seg.SrcPort != sourcePort || seg.DstPort != remotePort {
			t.Fatalf("packet %d left the single public four-tuple: %+v", i, seg)
		}
		if seg.Flags&faketcp.FlagSYN != 0 {
			syns = append(syns, seg)
		}
		if seg.Flags == faketcp.FlagACK && len(seg.Payload) == 0 && finalACK == nil {
			cp := seg
			finalACK = &cp
		}
		if len(seg.Payload) != 0 {
			data = append(data, seg)
		}
	}
	if len(syns) != 1 {
		t.Fatalf("single-flow emitted %d SYN packets, want exactly one incarnation", len(syns))
	}
	if !faketcp.IsWBDHandshakeSegment(syns[0]) || syns[0].Flags != faketcp.FlagSYN {
		t.Fatalf("first incarnation is not a WBD SYN: flags=%02x", syns[0].Flags)
	}
	if finalACK == nil || finalACK.Seq != syns[0].Seq+1 || finalACK.Ack != serverISN+1 {
		t.Fatalf("invalid final ACK: %+v", finalACK)
	}
	if len(data) < 3 {
		t.Fatalf("payload segments=%d want bootstrap chunks plus post-bootstrap data", len(data))
	}

	expectedSeq := syns[0].Seq + 1
	joined := make([]byte, 0, len(bootstrap)+len(postBootstrap))
	for i, seg := range data {
		if seg.Seq != expectedSeq {
			t.Fatalf("payload %d reset/created a sequence hole: seq=%d want=%d", i, seg.Seq, expectedSeq)
		}
		expectedSeq += uint32(len(seg.Payload))
		joined = append(joined, seg.Payload...)
	}
	want := append(append([]byte(nil), bootstrap...), postBootstrap...)
	if !bytes.Equal(joined, want) {
		t.Fatalf("single-flow payload transcript changed across mode barrier: got=%d bytes want=%d", len(joined), len(want))
	}
	if data[len(data)-1].Seq != syns[0].Seq+1+uint32(len(bootstrap)) {
		t.Fatalf("post-bootstrap data did not continue bootstrap sequence space: seq=%d", data[len(data)-1].Seq)
	}

	e.close()
	select {
	case err := <-rawErr:
		if err != nil {
			t.Fatalf("raw loop stopped with error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("raw loop did not stop")
	}
}
