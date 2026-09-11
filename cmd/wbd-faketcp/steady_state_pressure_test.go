package main

import (
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type pressureTestRaw struct {
	mu     sync.Mutex
	writes int
	resets int
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
	r.mu.Unlock()
	return nil
}
func (r *pressureTestRaw) counts() (writes, resets int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writes, r.resets
}

func TestUDPLoopWaitsForOutstandingCapacityInsteadOfResetting(t *testing.T) {
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
		sender:       faketcp.NewSenderWithRecovery(100, time.Second, faketcp.RecoveryLegacy),
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

	errCh := make(chan error, 1)
	go func() { errCh <- e.udpLoop() }()

	peer, err := net.DialUDP("udp4", nil, udp.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if _, err := peer.Write([]byte("pressure-datagram")); err != nil {
		t.Fatal(err)
	}

	select {
	case loopErr := <-errCh:
		t.Fatalf("udpLoop returned under transient full-window pressure: %v; want bounded wait", loopErr)
	case <-time.After(25 * time.Millisecond):
	}
	if _, resets := raw.counts(); resets != 0 {
		t.Fatalf("transient full-window pressure emitted %d RST packets; want 0", resets)
	}

	e.senderMu.Lock()
	e.sender.Ack(first.End, time.Now())
	e.senderMu.Unlock()

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		writes, resets := raw.counts()
		if resets != 0 {
			t.Fatalf("capacity recovery emitted %d RST packets; want 0", resets)
		}
		if writes > 0 {
			e.close()
			select {
			case loopErr := <-errCh:
				if loopErr != nil {
					t.Fatalf("udpLoop close: %v", loopErr)
				}
			case <-time.After(time.Second):
				t.Fatal("udpLoop did not stop after close")
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	e.close()
	t.Fatal("datagram was not sent after cumulative ACK reopened one steady-state slot")
}
