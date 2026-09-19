package main

import (
	"fmt"
)

// rollForwardSerializedOverlapLocked clears a stale replacement overlap left by
// a lost CLIENT_LEAVE when the next authenticated replacement is for a different
// logical LaneID. The product client owns only one replacement physical slot and
// serializes ReplaceLane calls, so reaching a different LaneID candidate proves
// that the previous overlap was already promoted client-side. This preserves the
// wire-compatible A -> A+B -> B contract without requiring a new ACK/version.
//
// Caller must hold both s.mu and gs.mu.
func (s *server) rollForwardSerializedOverlapLocked(gs *gameSession, incomingLaneID uint8) bool {
	if gs == nil || len(gs.overlap) != 1 {
		return false
	}
	for staleLaneID, candidate := range gs.overlap {
		if staleLaneID == incomingLaneID || candidate == nil {
			return false
		}
		primary := gs.lanes[staleLaneID]
		if primary == nil {
			return false
		}

		staleKey := primary.String()
		gs.lanes[staleLaneID] = candidate
		delete(gs.overlap, staleLaneID)
		delete(gs.peerLane, staleKey)
		delete(s.peerSession, staleKey)
		delete(s.peerMeta, staleKey)
		fmt.Printf("WBD_GAME_LANE_UNBIND tunnel_id_prefix=%s lane=%d association_peer=%s lanes=%d targets=%d reason=lost_leave_serialized_recovery next_lane=%d\n",
			tunnelIDPrefix(gs.meta), staleLaneID, staleKey, len(gs.lanes), len(gs.lanes)+len(gs.overlap), incomingLaneID)
		return true
	}
	return false
}
