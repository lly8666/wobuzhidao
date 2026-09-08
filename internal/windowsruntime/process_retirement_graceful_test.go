package windowsruntime

import (
	"errors"
	"testing"
	"time"
)

type gracefulStopRecorder struct {
	gracefulCalls int
	stopCalls     int
	gracefulErr   error
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

func TestStopDynamicLaneProcessUsesGracefulRetirementOnlyForLink(t *testing.T) {
	link := &gracefulStopRecorder{}
	if err := stopDynamicLaneProcess("link-2", link); err != nil {
		t.Fatal(err)
	}
	if link.gracefulCalls != 1 || link.stopCalls != 0 {
		t.Fatalf("link calls graceful=%d stop=%d", link.gracefulCalls, link.stopCalls)
	}

	dtls := &gracefulStopRecorder{}
	if err := stopDynamicLaneProcess("dtls-2", dtls); err != nil {
		t.Fatal(err)
	}
	if dtls.gracefulCalls != 0 || dtls.stopCalls != 1 {
		t.Fatalf("dtls calls graceful=%d stop=%d", dtls.gracefulCalls, dtls.stopCalls)
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
	if link.gracefulCalls != 1 || link.stopCalls != 0 {
		t.Fatalf("link calls graceful=%d stop=%d", link.gracefulCalls, link.stopCalls)
	}
	if dtls.stopCalls != 1 || fake.stopCalls != 1 {
		t.Fatalf("fallback cleanup dtls=%d fake=%d", dtls.stopCalls, fake.stopCalls)
	}
	if len(e.processes) != 0 {
		t.Fatalf("failed retirement left local children registered: %v", e.processes)
	}
}
