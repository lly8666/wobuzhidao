package windowsruntime

import (
	"errors"
	"testing"
	"time"
)

type gracefulStopRecorder struct {
	gracefulCalls  int
	peerResetCalls int
	stopCalls      int
	gracefulErr    error
	peerResetErr   error
}

func (p *gracefulStopRecorder) Stop() error {
	p.stopCalls++
	return nil
}

func (p *gracefulStopRecorder) GracefulStop(timeout time.Duration) error {
	p.gracefulCalls++
	if timeout != dynamicLaneProcessStopWait {
		return errors.New("unexpected graceful timeout")
	}
	return p.gracefulErr
}

func (p *gracefulStopRecorder) PeerResetStop(timeout time.Duration) error {
	p.peerResetCalls++
	if timeout != dynamicLaneProcessStopWait {
		return errors.New("unexpected peer-reset timeout")
	}
	return p.peerResetErr
}

func TestStopDynamicLaneProcessUsesLayerSpecificRetirement(t *testing.T) {
	link := &gracefulStopRecorder{}
	if err := stopDynamicLaneProcess("link-2", link); err != nil {
		t.Fatal(err)
	}
	if link.gracefulCalls != 1 || link.peerResetCalls != 0 || link.stopCalls != 0 {
		t.Fatalf("link calls graceful=%d peer_reset=%d stop=%d", link.gracefulCalls, link.peerResetCalls, link.stopCalls)
	}

	dtls := &gracefulStopRecorder{}
	if err := stopDynamicLaneProcess("dtls-2", dtls); err != nil {
		t.Fatal(err)
	}
	if dtls.gracefulCalls != 0 || dtls.peerResetCalls != 0 || dtls.stopCalls != 1 {
		t.Fatalf("dtls calls graceful=%d peer_reset=%d stop=%d", dtls.gracefulCalls, dtls.peerResetCalls, dtls.stopCalls)
	}

	fake := &gracefulStopRecorder{}
	if err := stopDynamicLaneProcess("faketcp-2", fake); err != nil {
		t.Fatal(err)
	}
	if fake.gracefulCalls != 0 || fake.peerResetCalls != 1 || fake.stopCalls != 0 {
		t.Fatalf("faketcp calls graceful=%d peer_reset=%d stop=%d", fake.gracefulCalls, fake.peerResetCalls, fake.stopCalls)
	}
}

func TestStopDynamicLanePlanReturnsGracefulRetirementFailure(t *testing.T) {
	lane := LanePlan{
		ID:      3,
		Slot:    5,
		FakeTCP: Command{Name: "faketcp-3-candidate-s5"},
		DTLS:    Command{Name: "dtls-3-candidate-s5"},
		Link:    Command{Name: "link-3-candidate-s5"},
	}
	link := &gracefulStopRecorder{gracefulErr: errors.New("missing remote close ACK")}
	dtls := &gracefulStopRecorder{}
	fake := &gracefulStopRecorder{}
	e := &Executor{
		running: true,
		processes: []namedProcess{
			{name: lane.FakeTCP.Name, proc: fake},
			{name: lane.DTLS.Name, proc: dtls},
			{name: lane.Link.Name, proc: link},
		},
	}

	if err := e.StopDynamicLanePlan(lane); err == nil {
		t.Fatal("expected graceful retirement failure")
	}
	if link.gracefulCalls != 1 || link.peerResetCalls != 0 || link.stopCalls != 0 {
		t.Fatalf("link calls graceful=%d peer_reset=%d stop=%d", link.gracefulCalls, link.peerResetCalls, link.stopCalls)
	}
	if dtls.stopCalls != 1 || fake.peerResetCalls != 1 || fake.stopCalls != 0 {
		t.Fatalf("fallback cleanup dtls_stop=%d fake_peer_reset=%d fake_stop=%d", dtls.stopCalls, fake.peerResetCalls, fake.stopCalls)
	}
	if len(e.processes) != 0 {
		t.Fatalf("failed retirement left local children registered: %v", e.processes)
	}
}

func TestStopDynamicLanePlanReturnsFakeTCPResetFailure(t *testing.T) {
	lane := LanePlan{
		ID:      1,
		Slot:    4,
		FakeTCP: Command{Name: "faketcp-1-candidate-s4"},
		DTLS:    Command{Name: "dtls-1-candidate-s4"},
		Link:    Command{Name: "link-1-candidate-s4"},
	}
	link := &gracefulStopRecorder{}
	dtls := &gracefulStopRecorder{}
	fake := &gracefulStopRecorder{peerResetErr: errors.New("missing peer reset marker")}
	e := &Executor{
		running: true,
		processes: []namedProcess{
			{name: lane.FakeTCP.Name, proc: fake},
			{name: lane.DTLS.Name, proc: dtls},
			{name: lane.Link.Name, proc: link},
		},
	}

	if err := e.StopDynamicLanePlan(lane); err == nil {
		t.Fatal("expected FakeTCP retirement reset failure")
	}
	if link.gracefulCalls != 1 || dtls.stopCalls != 1 || fake.peerResetCalls != 1 {
		t.Fatalf("retirement calls link=%d dtls=%d fake_reset=%d", link.gracefulCalls, dtls.stopCalls, fake.peerResetCalls)
	}
}
