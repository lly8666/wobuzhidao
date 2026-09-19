package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

type payloadActivityTracker struct {
	sequence uint64
	lastUnixNano int64
}

func (t *payloadActivityTracker) mark(now time.Time) {
	atomic.StoreInt64(&t.lastUnixNano, now.UnixNano())
	atomic.AddUint64(&t.sequence, 1)
}

func (t *payloadActivityTracker) snapshot() gamelane.PayloadActivity {
	seq := atomic.LoadUint64(&t.sequence)
	last := atomic.LoadInt64(&t.lastUnixNano)
	if seq == 0 {
		last = 0
	}
	return gamelane.PayloadActivity{Sequence: seq, LastPayloadActivityUnixNano: last}
}

func (c *client) handleControlRequest(raw []byte) any {
	op, err := gamelane.ParseLaneControlOp(raw)
	if err != nil {
		return gamelane.LaneControlReply{Error: err.Error()}
	}
	switch op {
	case gamelane.LaneControlSet:
		cmd, parseErr := gamelane.ParseLaneSetCommand(raw)
		reply := gamelane.LaneControlReply{}
		if parseErr != nil {
			reply.Error = parseErr.Error()
			return reply
		}
		active, applyErr := c.setLaneTargets(cmd.Lanes)
		if applyErr != nil {
			reply.Error = applyErr.Error()
			return reply
		}
		reply.OK = true
		reply.Active = active
		return reply
	case gamelane.LaneControlActivity:
		if _, parseErr := gamelane.ParseLaneActivityCommand(raw); parseErr != nil {
			return gamelane.LaneActivityReply{Error: parseErr.Error()}
		}
		return gamelane.LaneActivityReply{OK: true, Activity: c.activity.snapshot()}
	default:
		return gamelane.LaneControlReply{Error: fmt.Sprintf("unsupported game lane control op %q", op)}
	}
}
