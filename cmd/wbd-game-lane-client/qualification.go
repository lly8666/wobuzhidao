package main

import (
	"fmt"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

const (
	laneQualificationTimeout = 3 * time.Second
	laneQualificationRetry   = 200 * time.Millisecond
)

func (c *client) qualifyLane(lane *laneConn, timeout time.Duration) error {
	if lane == nil || lane.conn == nil || lane.ready == nil || c == nil || c.enc == nil {
		return fmt.Errorf("candidate lane qualification requires an active Game association")
	}
	if timeout <= 0 { timeout = laneQualificationTimeout }
	probe, err := gamelane.MarshalLaneProbe(c.enc.SessionID(), lane.id)
	if err != nil { return err }
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	retry := time.NewTicker(laneQualificationRetry)
	defer retry.Stop()

	send := func() error {
		if _, err := lane.conn.Write(probe); err != nil {
			return fmt.Errorf("write candidate lane %d qualification probe: %w", lane.id, err)
		}
		return nil
	}
	if err := send(); err != nil { return err }
	for {
		select {
		case <-lane.ready:
			fmt.Printf("WBD_GAME_LANE_CLIENT_QUALIFIED lane=%d proxy=%s\n", lane.id, lane.addr)
			return nil
		case <-retry.C:
			if err := send(); err != nil { return err }
		case <-deadline.C:
			return fmt.Errorf("candidate lane %d Game qualification timed out after %s", lane.id, timeout)
		}
	}
}

func (c *client) handleLaneMembershipControl(lane *laneConn, wire []byte) (bool, error) {
	control, err := gamelane.ParseMembershipControl(wire)
	if err != nil { return false, nil }
	if c == nil || c.enc == nil || lane == nil {
		return true, fmt.Errorf("received Game membership control without client lane state")
	}
	if control.SessionID != c.enc.SessionID() {
		return true, fmt.Errorf("received Game membership control for another logical session")
	}
	if control.LaneID != lane.id {
		return true, fmt.Errorf("received Game membership control for lane %d on lane %d", control.LaneID, lane.id)
	}
	if control.Op != gamelane.MembershipReady {
		return true, fmt.Errorf("unexpected server Game membership op=%d", control.Op)
	}
	lane.readyOnce.Do(func() { close(lane.ready) })
	return true, nil
}
