//go:build windows

package main

import (
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// windowsHandshakeCapture models the physical Windows/Npcap boundary that
// mattered in the 2026-08-29 failure: a busy adapter can expose unrelated
// frames before the real WBD SYNACK. Only the exact inbound WBD four-tuple may
// reach the shared FakeTCP handshake state machine.
type windowsHandshakeCapture struct {
	mu       sync.Mutex
	filter   npcapRawPacketIO
	queue    [][]byte
	writes   [][]byte
	answered bool
}

func (r *windowsHandshakeCapture) ReadPacket(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.queue) != 0 {
		pkt := r.queue[0]
		r.queue = r.queue[1:]
		if !r.filter.matchesInboundFlow(pkt) {
			continue
		}
		if len(pkt) > len(buf) {
			return 0, io.ErrShortBuffer
		}
		copy(buf, pkt)
		return len(pkt), nil
	}
	return 0, errRawTimeout
}

func (r *windowsHandshakeCapture) WritePacket(packet []byte, _ [4]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := append([]byte(nil), packet...)
	r.writes = append(r.writes, cp)
	seg, err := faketcp.ParseIPv4TCP(packet)
	if err != nil || r.answered || seg.Flags&faketcp.FlagSYN == 0 || seg.Flags&faketcp.FlagACK != 0 {
		return nil
	}
	r.answered = true
	serverSeq := uint32(0x4a17c003)

	// 1) exact IPs but UDP: a busy physical adapter may deliver this first.
	udpNoise := make([]byte, 40)
	udpNoise[0] = 0x45
	binary.BigEndian.PutUint16(udpNoise[2:4], uint16(len(udpNoise)))
	udpNoise[9] = 17
	copy(udpNoise[12:16], r.filter.remoteIP[:])
	copy(udpNoise[16:20], r.filter.sourceIP[:])

	// 2) valid TCP from the server but a different connection.
	wrongTCP := faketcp.MarshalIPv4TCP(
		r.filter.remoteIP, r.filter.sourceIP,
		r.filter.remotePort+1, r.filter.sourcePort,
		123, seg.Seq+1, faketcp.FlagACK, 65535, nil, 2,
	)

	// 3) Npcap may expose a locally injected outbound frame on some capture
	// configurations. The direction check must suppress it.
	outboundEcho := append([]byte(nil), packet...)

	// 4) the real WBD SYNACK. This must be the first packet the protocol parser
	// sees after adapter-level filtering.
	synAck := faketcp.MarshalIPv4TCP(
		r.filter.remoteIP, r.filter.sourceIP,
		r.filter.remotePort, r.filter.sourcePort,
		serverSeq, seg.Seq+1, faketcp.FlagSYN|faketcp.FlagACK, 65535, nil, 3,
	)
	r.queue = append(r.queue, udpNoise, wrongTCP, outboundEcho, synAck)
	return nil
}

func (*windowsHandshakeCapture) SetReadTimeout(time.Duration) error { return nil }
func (*windowsHandshakeCapture) ClearReadTimeout() error            { return nil }
func (*windowsHandshakeCapture) Close() error                       { return nil }

func TestWindowsAdapterNoiseStillCompletesFakeTCPHandshake(t *testing.T) {
	sourceIP := [4]byte{192, 0, 2, 20}
	remoteIP := [4]byte{198, 51, 100, 10}
	const sourcePort uint16 = 41001
	const remotePort uint16 = 443

	raw := &windowsHandshakeCapture{filter: npcapRawPacketIO{
		sourceIP: sourceIP, remoteIP: remoteIP,
		sourcePort: sourcePort, remotePort: remotePort,
	}}
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

	if err := e.handshakeClient(); err != nil {
		t.Fatalf("handshake through noisy Windows adapter: %v", err)
	}
	if e.sender == nil || e.receiver == nil {
		t.Fatal("successful handshake did not initialize steady-state sender/receiver")
	}

	raw.mu.Lock()
	writes := append([][]byte(nil), raw.writes...)
	raw.mu.Unlock()
	if len(writes) != 2 {
		t.Fatalf("raw writes = %d, want SYN + final ACK", len(writes))
	}
	syn, err := faketcp.ParseIPv4TCP(writes[0])
	if err != nil {
		t.Fatal(err)
	}
	ack, err := faketcp.ParseIPv4TCP(writes[1])
	if err != nil {
		t.Fatal(err)
	}
	if syn.Flags != faketcp.FlagSYN || !faketcp.IsWBDHandshakeSegment(syn) {
		t.Fatalf("first write is not the WBD SYN: flags=%02x", syn.Flags)
	}
	if ack.Flags != faketcp.FlagACK || ack.Seq != syn.Seq+1 || ack.Ack != serverSeqForTest()+1 {
		t.Fatalf("final ACK mismatch: flags=%02x seq=%d want=%d ack=%d want=%d",
			ack.Flags, ack.Seq, syn.Seq+1, ack.Ack, serverSeqForTest()+1)
	}
	if ack.SrcIP != sourceIP || ack.DstIP != remoteIP || ack.SrcPort != sourcePort || ack.DstPort != remotePort {
		t.Fatalf("final ACK left the single flow: %+v", ack)
	}
}

func serverSeqForTest() uint32 { return 0x4a17c003 }
