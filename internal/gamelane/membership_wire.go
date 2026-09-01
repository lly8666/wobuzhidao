package gamelane

import (
	"errors"
)

const membershipControlSize = 22

var membershipMagic = [4]byte{'W', 'G', 'C', '1'}

type MembershipOp uint8

const MembershipLeave MembershipOp = 1

type MembershipControl struct {
	SessionID SessionID
	LaneID    uint8
	Op        MembershipOp
}

// MarshalLaneLeave creates an idempotent Logical Tunnel membership hint carried
// inside an already-authenticated lane. It is not a public transport handshake
// and changes no FakeTCP/DTLS wire semantics. The server may still recover from
// a lost leave by authenticated same-session lane rebinding.
func MarshalLaneLeave(sessionID SessionID, laneID uint8) ([]byte, error) {
	if sessionID == (SessionID{}) || laneID == 0 || laneID > MaxLanes {
		return nil, ErrMalformed
	}
	wire := make([]byte, membershipControlSize)
	copy(wire[:4], membershipMagic[:])
	copy(wire[4:20], sessionID[:])
	wire[20] = laneID
	wire[21] = byte(MembershipLeave)
	return wire, nil
}

func ParseMembershipControl(wire []byte) (MembershipControl, error) {
	var out MembershipControl
	if len(wire) != membershipControlSize || string(wire[:4]) != string(membershipMagic[:]) {
		return out, errors.New("gamelane: not a membership control frame")
	}
	copy(out.SessionID[:], wire[4:20])
	out.LaneID = wire[20]
	out.Op = MembershipOp(wire[21])
	if out.SessionID == (SessionID{}) || out.LaneID == 0 || out.LaneID > MaxLanes || out.Op != MembershipLeave {
		return MembershipControl{}, ErrMalformed
	}
	return out, nil
}
