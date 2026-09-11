package main

import (
	"bytes"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type pressureTestRaw struct {
	mu       sync.Mutex
	writes   int
	resets   int
	payloads [][]byte
}

func (r *pressureTestRaw) ReadPacket([]byte) (int, error)     { return 0, os.ErrClosed }
func (r *pressureTestRaw) SetReadTimeout(time.Duration) error { return nil }
func (r *pressureTestRaw) ClearReadTimeout() error            { return nil }
func (r *pressureTestRaw) Close() error                       { return nil }
func (r *pressureTestRaw) WritePacket(pkt []byte, _ [4]byte) error {
	seg, err := faketcp.ParseIPv4TCP(pkt)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.writes++
	if seg.Flags&faketcp.FlagRST != 0 {
		r.resets++
	}
	if len(seg.Payload) != 0 {
		r.payloads = append(r.payloads, append([]byte(nil), seg.Payload...))
	}
	r.mu.Unlock()
	return nil
}
func (r *pressureTestRaw) counts() (writes, resets int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writes, r.resets
}
func (r *pressureTestRaw) payloadSnapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.payloads))
	for i := range r.payloads {
		out[i] = append([]byte(nil), r.payloads[i]...)
	}
	return out
}

func newPressureEndpoint(t *testing.T) (*endpoint, *pressureTestRaw, *faketcp.Pending) {
	t.Helper()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	fragmenter, err := faketcp.NewCarrierFragmenter(1500)
	if err != nil {
		t.Fatal(err)
	}
	raw := &pressureTestRaw{}
	e := &endpoint{
		cfg:          config{role: "client"},
		raw:          raw,
		srcIP:        [4]byte{10, 0, 0, 2},
		dstIP:        [4]byte{10, 0, 0, 1},
		srcPort:      41000,
		dstPort:      40000,
		udp:          udp,
		carrierMTU:   1500,
		fragmenter:   fragmenter,
		reassembler:  faketcp.NewCarrierReassembler(),
		sender:       faketcp.NewSenderWithRecovery(100, time.Second, faketcp.RecoverySACKRACK),
		receiver:     faketcp.NewReceiver(500),
		sendBuf:      make([]byte, 65535),
		stop:         make(chan struct{}),
		bootstrapAck: make(chan struct{}, 1),
	}

	now := time.Unix(100, 0)
	var first *faketcp.Pending
	for i := 0; i < faketcp.MaxSteadyStateOutstandingDatagrams; i++ {
		p, enqueueErr := e.sender.EnqueueSteadyState([]byte{byte(i)}, now)
		if enqueueErr != nil {
			t.Fatalf("prefill %d: %v", i, enqueueErr)
		}
		if i == 0 {
			first = p
		}
	}
	return e, raw, first
}

func TestUDPLoopAdmitsFreshWithoutResetAtRepairCeiling(t *testing.T) {
	e, raw, first := newPressureEndpoint(t)
	defer e.close()

	errCh := make(chan error, 1)
	go func() { errCh <- e.udpLoop() }()

	peer, err := net.DialUDP("udp4", nil, e.udp.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	payload := []byte("pressure-datagram")
	if _, err := peer.Write(payload); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		ps := raw.payloadSnapshot()
		if len(ps) == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		reassembler := faketcp.NewCarrierReassembler()
		datagram, complete, frameErr := reassembler.Push(ps[0])
		if frameErr != nil || !complete {
			t.Fatalf("reassemble fresh datagram complete=%t err=%v", complete, frameErr)
		}
		if !bytes.Equal(datagram, payload) {
			t.Fatalf("fresh datagram=%q want=%q", datagram, payload)
		}
		if _, resets := raw.counts(); resets != 0 {
			t.Fatalf("repair ceiling emitted %d RST packets; want 0", resets)
		}
		e.senderMu.Lock()
		retired := e.sender.Outstanding(first.Seq) == nil && first.Payload == nil && first.Retired
		pending := e.sender.Pending()
		e.senderMu.Unlock()
		if !retired {
			t.Fatal("oldest optional repair was not retired to admit fresh data")
		}
		if pending != faketcp.MaxSteadyStateOutstandingDatagrams {
			t.Fatalf("pending=%d want=%d", pending, faketcp.MaxSteadyStateOutstandingDatagrams)
		}
		return
	}
	select {
	case loopErr := <-errCh:
		t.Fatalf("udpLoop returned before fresh admission: %v", loopErr)
	default:
	}
	t.Fatal("fresh datagram was HOL-blocked by full shadow repair debt")
}

func TestUDPLoopFullRepairDebtPreservesFreshFIFOWithoutSizeHeuristic(t *testing.T) {
	e, raw, first := newPressureEndpoint(t)
	defer e.close()

	errCh := make(chan error, 1)
	go func() { errCh <- e.udpLoop() }()

	peer, err := net.DialUDP("udp4", nil, e.udp.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	bulk := bytes.Repeat([]byte{0x42}, 1000)
	controlSized := bytes.Repeat([]byte{0x43}, 64)
	if _, err := peer.Write(bulk); err != nil {
		t.Fatal(err)
	}
	if _, err := peer.Write(controlSized); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		ps := raw.payloadSnapshot()
		if len(ps) < 2 {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		reassembler := faketcp.NewCarrierReassembler()
		firstDatagram, complete, frameErr := reassembler.Push(ps[0])
		if frameErr != nil || !complete {
			t.Fatalf("first carrier complete=%t err=%v", complete, frameErr)
		}
		secondDatagram, complete, frameErr := reassembler.Push(ps[1])
		if frameErr != nil || !complete {
			t.Fatalf("second carrier complete=%t err=%v", complete, frameErr)
		}
		if !bytes.Equal(firstDatagram, bulk) || !bytes.Equal(secondDatagram, controlSized) {
			t.Fatalf("fresh FIFO changed: first=%d second=%d want=%d,%d", len(firstDatagram), len(secondDatagram), len(bulk), len(controlSized))
		}
		if _, resets := raw.counts(); resets != 0 {
			t.Fatalf("repair debt admission emitted %d RST packets; want 0", resets)
		}
		e.senderMu.Lock()
		oldestRetired := e.sender.Outstanding(first.Seq) == nil && first.Payload == nil && first.Retired
		pending := e.sender.Pending()
		e.senderMu.Unlock()
		if !oldestRetired {
			t.Fatal("old shadow repair remained eligible after fresh FIFO admission")
		}
		if pending != faketcp.MaxSteadyStateOutstandingDatagrams {
			t.Fatalf("pending=%d want=%d", pending, faketcp.MaxSteadyStateOutstandingDatagrams)
		}
		return
	}
	select {
	case loopErr := <-errCh:
		t.Fatalf("udpLoop returned under repair debt pressure: %v", loopErr)
	default:
	}
	t.Fatal("fresh bulk/control pair did not pass repair ceiling without HOL")
}
