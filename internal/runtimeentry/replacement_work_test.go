package runtimeentry

import (
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func TestFreshQualificationDoesNotPollRetiringStatePerRecord(t *testing.T) {
	now := time.Now()
	old := &serverLifecycleLane{ref: logicaltunnel.LaneRef{ID: 1, Generation: 1}, closeStarted: now}
	group := &serverLifecycleTunnel{lanes: make(map[uint8]*serverLifecycleLane)}
	fresh := &serverLifecycleLane{ref: logicaltunnel.LaneRef{ID: 1, Generation: 2}, replaces: old, group: group}
	group.lanes[1] = fresh
	s := &LifecycleServer{cfg: LifecycleServerConfig{ReplacementGrace: time.Hour}}
	s.cfg.ObserveTiming = true
	// A retired transport need not exist for this operation-count regression.
	// Stats returns missing; the not-yet-expired close retains the old lane.
	for i := 0; i < 10000; i++ {
		s.markLaneQualified(fresh, now)
	}
	if !fresh.qualified || fresh.replaces != old || s.pipeline.replacementChecks.Load() != 0 {
		t.Fatal("fresh records performed old retirement checks or changed lifetime")
	}
	s.retireServerReplacement(fresh)
	if s.pipeline.replacementChecks.Load() != 1 || fresh.replaces != old {
		t.Fatal("maintenance lost its bounded retirement check")
	}
}
