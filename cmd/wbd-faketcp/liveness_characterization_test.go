package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// Blackhole behavior remains a characterization until an explicit dead-peer
// policy is qualified. RST, however, is an exact-flow terminal signal and must
// retire the current transport incarnation so the lane watchdog can replace it.

type livenessRaw struct {
	mu     sync.Mutex
	packet []byte
	sent   bool
	closed bool
}

func (r *livenessRaw) ReadPacket(buf []byte) (int, error) {
	r.mu.Lock()
	if !r.sent {
		r.sent = true
		p := append([]byte(nil), r.packet...)
		r.mu.Unlock()
		copy(buf, p)
		return len(p), nil
	}
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return 0, errRawTimeout
	}
	time.Sleep(2 * time.Millisecond)
	return 0, errRawTimeout
}

func (r *livenessRaw) WritePacket([]byte, [4]byte) error { return nil }
func (r *livenessRaw) SetReadTimeout(time.Duration) error { return nil }
func (r *livenessRaw) ClearReadTimeout() error { return nil }
func (r *livenessRaw) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}

func TestPeerRSTTerminatesCurrentAssociation(t *testing.T) {
	clientIP := [4]byte{10, 91, 0, 2}
	serverIP := [4]byte{10, 91, 0, 1}
	raw := &livenessRaw{packet: faketcp.MarshalIPv4TCP(
		serverIP, clientIP, 40000, 41001, 9000, 7000,
		faketcp.FlagRST, 65535, nil, 1,
	)}
	e := &endpoint{
		cfg:          config{role: "client"},
		raw:          raw,
		srcIP:        clientIP,
		dstIP:        serverIP,
		srcPort:      41001,
		dstPort:      40000,
		sender:       faketcp.NewSender(7000, 1200*time.Millisecond),
		receiver:     faketcp.NewReceiver(9000),
		sendBuf:      make([]byte, 65535),
		stop:         make(chan struct{}),
		bootstrapAck: make(chan struct{}, 1),
	}

	done := make(chan error, 1)
	go func() { done <- e.rawLoop() }()
	select {
	case err := <-done:
		if !errors.Is(err, errPeerReset) {
			t.Fatalf("raw loop error=%v want peer reset", err)
		}
	case <-time.After(time.Second):
		t.Fatal("peer RST did not retire current FakeTCP association")
	}
	select {
	case <-e.stop:
		// The bootstrap/data child must be closed too; merely returning rawLoop
		// is insufficient while a TLS bootstrap is blocked on the same stream.
	default:
		t.Fatal("peer RST did not close endpoint incarnation")
	}
}

func TestCurrentBlackholeRetransmissionHasNoRetryCeiling(t *testing.T) {
	now := time.Unix(1000, 0)
	s := faketcp.NewSender(1000, 1200*time.Millisecond)
	p := s.Enqueue([]byte("blackhole-probe"), now)

	for i := 0; i < 12; i++ {
		now = now.Add(s.RTO())
		got := s.RetransmitDue(now)
		if got != p {
			t.Fatalf("retry %d got=%p want=%p", i+1, got, p)
		}
	}
	if p.Retries != 12 {
		t.Fatalf("retries=%d want=12", p.Retries)
	}
	if got := s.RTO(); got != 60*time.Second {
		t.Fatalf("backed-off RTO=%s want 60s clamp", got)
	}
	// There is intentionally no retry-limit assertion here: receiving no ACK
	// does not currently convert the client association into a terminal state.
}
