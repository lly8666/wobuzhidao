package datapath

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// Health is independent of LINK/FEC, padding and application delivery. The
// record PN fences stale/reordered idle hints within a lane incarnation.
type HealthMessage struct {
	PN      uint64
	IdleFor time.Duration
}

const MaxHealthIdle = 7 * 24 * time.Hour

func parseHealth(pn uint64, payload []byte) (HealthMessage, error) {
	if len(payload) != 9 || payload[0] != 1 {
		return HealthMessage{}, errors.New("datapath: malformed health record")
	}
	ms := binary.BigEndian.Uint64(payload[1:])
	if ms > uint64(MaxHealthIdle/time.Millisecond) {
		return HealthMessage{}, errors.New("datapath: health idle overflow")
	}
	return HealthMessage{PN: pn, IdleFor: time.Duration(ms) * time.Millisecond}, nil
}

func (o *TunnelOwner) HealthRecord(ref logicaltunnel.LaneRef, idle time.Duration) (WireRecord, error) {
	o.mu.Lock()
	binding, ok := o.active[ref.ID]
	if o.closed || !ok || binding.ref != ref {
		o.mu.Unlock()
		return WireRecord{}, ErrLaneUnavailable
	}
	o.mu.Unlock()
	if idle < 0 {
		idle = 0
	}
	if idle > MaxHealthIdle {
		idle = MaxHealthIdle
	}
	var payload [9]byte
	payload[0] = 1
	binary.BigEndian.PutUint64(payload[1:], uint64(idle/time.Millisecond))
	lane := binding.lane
	lane.mu.Lock()
	defer lane.mu.Unlock()
	if lane.closed {
		return WireRecord{}, ErrLaneClosed
	}
	wire, pn, err := lane.sealer.SealHealth(payload[:])
	if err != nil {
		return WireRecord{}, err
	}
	// No FEC, no startup padding, no flow allocation, no new repair credit.
	return WireRecord{PN: pn, Wire: wire, Control: true}, nil
}
