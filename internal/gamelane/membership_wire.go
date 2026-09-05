package gamelane

import (
	"errors"
)

const membershipControlSize = 22

var membershipMagic = [4]byte{'W', 'G', 'C', '1'}

type MembershipOp uint8

const (
	MembershipLeave MembershipOp = 1
	MembershipProbe MembershipOp = 2
	MembershipReady MembershipOp = 3
)

type MembershipControl struct {
	SessionID SessionID
	LaneID    uint8
	Op        MembershipOp
}

func marshalMembershipControl(sessionID SessionID, laneID uint8, op MembershipOp) ([]byte, error) {
	if sessionID == (SessionID{}) || laneID == 0 || laneID > MaxLanes {
		return nil, ErrMalformed
	}
	switch op {
	case MembershipLeave, MembershipProbe, MembershipReady:
	default:
		return nil, ErrMalformed
	}
	wire := make([]byte, membershipControlSize)
	copy(wire[:4], membershipMagic[:])
	copy(wire[4:20], sessionID[:])
	wire[20] = laneID
	wire[21] = byte(op)
	return wire, nil
}

// MarshalLaneLeave creates an idempotent Logical Tunnel membership hint carried
// inside an already-authenticated lane. It is not a public transport handshake
// and changes no FakeTCP/DTLS wire semantics. The server may still recover from
// a lost leave by authenticated same-session lane rebinding.
func MarshalLaneLeave(sessionID SessionID, laneID uint8) ([]byte, error) {
	return marshalMembershipControl(sessionID, laneID, MembershipLeave)
}

// MarshalLaneProbe asks the authenticated Game backend to bind this exact
// transport incarnation before it may replace the current incarnation. The
// probe stays inside the existing private WGC1 membership family and therefore
// does not alter the public FakeTCP/Reality/DTLS/LINK handshake.
func MarshalLaneProbe(sessionID SessionID, laneID uint8) ([]byte, error) {
	return marshalMembershipControl(sessionID, laneID, MembershipProbe)
}

// MarshalLaneReady acknowledges that the probed association has authenticated
// Logical Tunnel metadata and is bound in the server Game race set.
func MarshalLaneReady(sessionID SessionID, laneID uint8) ([]byte, error) {
	return marshalMembershipControl(sessionID, laneID, MembershipReady)
}

func ParseMembershipControl(wire []byte) (MembershipControl, error) {
	var out MembershipControl
	if len(wire) != membershipControlSize || string(wire[:4]) != string(membershipMagic[:]) {
		return out, errors.New("gamelane: not a membership control frame")
	}
	copy(out.SessionID[:], wire[4:20])
	out.LaneID = wire[20]
	out.Op = MembershipOp(wire[21])
	if out.SessionID == (SessionID{}) || out.LaneID == 0 || out.LaneID > MaxLanes {
		return MembershipControl{}, ErrMalformed
	}
	switch out.Op {
	case MembershipLeave, MembershipProbe, MembershipReady:
		return out, nil
	default:
		return MembershipControl{}, ErrMalformed
	}
}
