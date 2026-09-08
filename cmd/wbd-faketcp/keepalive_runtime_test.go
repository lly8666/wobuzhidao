package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type keepaliveRuntimeRaw struct {
	mu        sync.Mutex
	closed    bool
	reads     chan []byte
	probes    int
	rsts      int
	replyFrom int
	replyAll  bool
	replyWait time.Duration
	clientIP  [4]byte
	serverIP  [4]byte
	clientPort uint16
	serverPort uint16
	clientNext uint32
	serverNext uint32
}

func newKeepaliveRuntimeRaw() *keepaliveRuntimeRaw {
	return &keepaliveRuntimeRaw{
		reads: make(chan []byte, 128),
		clientIP: [4]byte{10, 93, 0, 2}, serverIP: [4]byte{10, 93, 0, 1},
		clientPort: 41001, serverPort: 40000, clientNext: 7000, serverNext: 9000,
	}
}

func (r *keepaliveRuntimeRaw) ReadPacket(buf []byte) (int, error) {
	select {
	case p := <-r.reads:
		copy(buf, p)
		return len(p), nil
	case <-time.After(time.Millisecond):
		r.mu.Lock()
		closed := r.closed
		r.mu.Unlock()
		if closed {
			return 0, errRawTimeout
		}
		return 0, errRawTimeout
	}
}

func (r *keepaliveRuntimeRaw) WritePacket(p []byte, _ [4]byte) error {
	seg, err := faketcp.ParseIPv4TCP(p)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	if seg.Flags&faketcp.FlagRST != 0 {
		r.rsts++
		r.mu.Unlock()
		return nil
	}
	isProbe := faketcp.IsKeepaliveProbe(seg, r.clientNext)
	if !isProbe {
		r.mu.Unlock()
		return nil
	}
	r.probes++
	probeNo := r.probes
	reply := r.replyAll || (r.replyFrom > 0 && probeNo >= r.replyFrom)
	wait := r.replyWait
	r.mu.Unlock()
	if reply {
		resp := faketcp.MarshalIPv4TCP(r.serverIP, r.clientIP, r.serverPort, r.clientPort,
			r.serverNext, r.clientNext, faketcp.FlagACK, 65535, nil, uint16(probeNo))
		if wait == 0 {
			r.reads <- resp
		} else {
			time.AfterFunc(wait, func() {
				r.mu.Lock()
				closed := r.closed
				r.mu.Unlock()
				if !closed {
					select { case r.reads <- resp: default: }
				}
			})
		}
	}
	return nil
}

func (r *keepaliveRuntimeRaw) SetReadTimeout(time.Duration) error { return nil }
func (r *keepaliveRuntimeRaw) ClearReadTimeout() error { return nil }
func (r *keepaliveRuntimeRaw) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}
func (r *keepaliveRuntimeRaw) counts() (probes, rsts int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.probes, r.rsts
}

func runtimeKeepaliveEndpoint(r *keepaliveRuntimeRaw) *endpoint {
	return &endpoint{
		cfg: config{role: "client"}, raw: r,
		srcIP: r.clientIP, dstIP: r.serverIP, srcPort: r.clientPort, dstPort: r.serverPort,
		sender: faketcp.NewSender(r.clientNext, time.Second), receiver: faketcp.NewReceiver(r.serverNext),
		sendBuf: make([]byte, 65535), stop: make(chan struct{}), bootstrapAck: make(chan struct{}, 1),
	}
}

func withRuntimeKeepalivePolicy(t *testing.T, p keepalivePolicy) {
	t.Helper()
	old := defaultKeepalivePolicy
	defaultKeepalivePolicy = p
	t.Cleanup(func() { defaultKeepalivePolicy = old })
}

func tinyRuntimePolicy() keepalivePolicy {
	return keepalivePolicy{Idle: 4 * time.Millisecond, ProbeWaitMin: 4 * time.Millisecond, ProbeWaitMax: 4 * time.Millisecond, MissWindows: 4, AttemptsPerMiss: 3}
}

func startRuntimeLoops(e *endpoint) (<-chan error, <-chan error) {
	rawDone := make(chan error, 1)
	retxDone := make(chan error, 1)
	go func() { rawDone <- e.rawLoop() }()
	go func() { retxDone <- e.retransmitLoop() }()
	return rawDone, retxDone
}

func TestRuntimeKeepaliveHealthyPeerDoesNotRetire(t *testing.T) {
	withRuntimeKeepalivePolicy(t, tinyRuntimePolicy())
	r := newKeepaliveRuntimeRaw()
	r.replyAll = true
	e := runtimeKeepaliveEndpoint(r)
	_, retxDone := startRuntimeLoops(e)
	time.Sleep(45 * time.Millisecond)
	select {
	case err := <-retxDone:
		t.Fatalf("healthy peer retired: %v", err)
	default:
	}
	probes, rsts := r.counts()
	if probes == 0 || rsts != 0 {
		t.Fatalf("healthy probes=%d rsts=%d", probes, rsts)
	}
	e.close()
}

func TestRuntimeKeepaliveBlackholeRetiresAfterTwelveProbesAndSendsRST(t *testing.T) {
	withRuntimeKeepalivePolicy(t, tinyRuntimePolicy())
	r := newKeepaliveRuntimeRaw()
	e := runtimeKeepaliveEndpoint(r)
	_, retxDone := startRuntimeLoops(e)
	select {
	case err := <-retxDone:
		if !errors.Is(err, errDeadPeer) {
			t.Fatalf("retire error=%v want dead peer", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blackholed peer was not retired")
	}
	probes, rsts := r.counts()
	if probes != 12 {
		t.Fatalf("blackhole probes=%d want 12", probes)
	}
	if rsts != 1 {
		t.Fatalf("best-effort terminal RST count=%d want 1", rsts)
	}
	select {
	case <-e.stop:
	default:
		t.Fatal("dead-peer retirement did not close endpoint")
	}
}

func TestRuntimeKeepaliveTwelfthProbeCanRecover(t *testing.T) {
	withRuntimeKeepalivePolicy(t, tinyRuntimePolicy())
	r := newKeepaliveRuntimeRaw()
	r.replyFrom = 12
	e := runtimeKeepaliveEndpoint(r)
	_, retxDone := startRuntimeLoops(e)
	deadline := time.Now().Add(time.Second)
	for {
		probes, _ := r.counts()
		if probes >= 12 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("did not reach twelfth probe")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(15 * time.Millisecond)
	select {
	case err := <-retxDone:
		t.Fatalf("peer replying on final allowed probe retired: %v", err)
	default:
	}
	_, rsts := r.counts()
	if rsts != 0 {
		t.Fatalf("recovered peer got terminal RST count=%d", rsts)
	}
	e.close()
}

func TestRuntimeKeepalive300msRTTAfterTwoLostProbes(t *testing.T) {
	p := keepalivePolicy{Idle: 5 * time.Millisecond, ProbeWaitMin: 700 * time.Millisecond, ProbeWaitMax: 700 * time.Millisecond, MissWindows: 4, AttemptsPerMiss: 3}
	withRuntimeKeepalivePolicy(t, p)
	r := newKeepaliveRuntimeRaw()
	r.replyFrom = 3
	r.replyWait = 300 * time.Millisecond
	e := runtimeKeepaliveEndpoint(r)
	_, retxDone := startRuntimeLoops(e)
	deadline := time.Now().Add(3 * time.Second)
	for {
		probes, _ := r.counts()
		if probes >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("300ms RTT test did not reach third probe")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)
	select {
	case err := <-retxDone:
		t.Fatalf("300ms RTT peer retired after valid delayed ACK: %v", err)
	default:
	}
	e.close()
}

func TestRuntimeKeepaliveFourLanesBlackholeIndependently(t *testing.T) {
	withRuntimeKeepalivePolicy(t, tinyRuntimePolicy())
	const lanes = 4
	done := make([]<-chan error, 0, lanes)
	raws := make([]*keepaliveRuntimeRaw, 0, lanes)
	for i := 0; i < lanes; i++ {
		r := newKeepaliveRuntimeRaw()
		r.clientPort += uint16(i)
		e := runtimeKeepaliveEndpoint(r)
		_, d := startRuntimeLoops(e)
		done = append(done, d)
		raws = append(raws, r)
	}
	for i, d := range done {
		select {
		case err := <-d:
			if !errors.Is(err, errDeadPeer) {
				t.Fatalf("lane %d error=%v", i+1, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("lane %d did not independently retire", i+1)
		}
		probes, rsts := raws[i].counts()
		if probes != 12 || rsts != 1 {
			t.Fatalf("lane %d probes=%d rsts=%d", i+1, probes, rsts)
		}
	}
}
