package windowsruntime

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type retirementRecordingProcess struct {
	name    string
	events  *[]string
	waitErr error
}

func (p *retirementRecordingProcess) Stop() error {
	*p.events = append(*p.events, "stop:"+p.name)
	return nil
}

func (p *retirementRecordingProcess) WaitStopped(timeout time.Duration) error {
	*p.events = append(*p.events, "wait:"+p.name)
	if timeout != dynamicLaneProcessStopWait {
		return errors.New("unexpected retirement timeout")
	}
	return p.waitErr
}

func retirementTestLane(events *[]string) (LanePlan, []namedProcess) {
	lane := LanePlan{
		ID:   2,
		Slot: 1,
		FakeTCP: Command{Name: "faketcp-2-candidate-s1"},
		DTLS:    Command{Name: "dtls-2-candidate-s1"},
		Link:    Command{Name: "link-2-candidate-s1"},
	}
	processes := []namedProcess{
		{name: lane.FakeTCP.Name, proc: &retirementRecordingProcess{name: lane.FakeTCP.Name, events: events}},
		{name: lane.DTLS.Name, proc: &retirementRecordingProcess{name: lane.DTLS.Name, events: events}},
		{name: lane.Link.Name, proc: &retirementRecordingProcess{name: lane.Link.Name, events: events}},
	}
	return lane, processes
}

func TestStopDynamicLanePlanWaitsForRetiredChildren(t *testing.T) {
	var events []string
	lane, processes := retirementTestLane(&events)
	e := &Executor{running: true, processes: processes}

	if err := e.StopDynamicLanePlan(lane); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"stop:" + lane.Link.Name, "wait:" + lane.Link.Name,
		"stop:" + lane.DTLS.Name, "wait:" + lane.DTLS.Name,
		"stop:" + lane.FakeTCP.Name, "wait:" + lane.FakeTCP.Name,
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("retirement events=%v want=%v", events, want)
	}
	if len(e.processes) != 0 {
		t.Fatalf("retired process group remains registered: %v", e.processes)
	}
}

func TestStopDynamicLanePlanReportsRetirementWaitFailure(t *testing.T) {
	var events []string
	lane, processes := retirementTestLane(&events)
	for i := range processes {
		if processes[i].name == lane.DTLS.Name {
			processes[i].proc.(*retirementRecordingProcess).waitErr = errors.New("still exiting")
		}
	}
	e := &Executor{running: true, processes: processes}

	err := e.StopDynamicLanePlan(lane)
	if err == nil || !strings.Contains(err.Error(), "wait "+lane.DTLS.Name+" exit") || !strings.Contains(err.Error(), "still exiting") {
		t.Fatalf("retirement wait error=%v", err)
	}
}
