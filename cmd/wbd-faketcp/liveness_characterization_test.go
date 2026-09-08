package main

import (
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// These tests intentionally characterize the current transport-liveness
// contract. They are not an endorsement of the behavior: the Actions job that
// runs them labels the RST/blackhole result as a known-gap characterization so
// a later terminal-policy change has an explicit test to update.

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

func TestCurrentPeerRSTIsNonTerminal(t *testing.T) {
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
		t.Fatalf("current raw loop unexpectedly treated peer RST as terminal: %v", err)
	case <-time.After(75 * time.Millisecond):
		// Current behavior: exact-flow RST has no payload and is ignored after
		// ordinary ACK processing. Therefore the child stays alive.
	}
	e.close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("raw loop close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("raw loop did not stop after endpoint close")
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
