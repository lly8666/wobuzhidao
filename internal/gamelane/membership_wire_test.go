package gamelane

import (
	"errors"
	"testing"
)

func TestLaneLeaveMembershipControlRoundTrip(t *testing.T) {
	var sid SessionID
	for i := range sid { sid[i] = byte(i + 1) }
	wire, err := MarshalLaneLeave(sid, 4)
	if err != nil { t.Fatal(err) }
	got, err := ParseMembershipControl(wire)
	if err != nil { t.Fatal(err) }
	if got.SessionID != sid || got.LaneID != 4 || got.Op != MembershipLeave {
		t.Fatalf("control=%+v", got)
	}
}

func TestLaneLeaveMembershipControlRejectsMalformed(t *testing.T) {
	var sid SessionID
	sid[0] = 1
	if _, err := MarshalLaneLeave(sid, 0); !errors.Is(err, ErrMalformed) { t.Fatalf("lane zero err=%v", err) }
	wire, _ := MarshalLaneLeave(sid, 1)
	wire[21] = 9
	if _, err := ParseMembershipControl(wire); !errors.Is(err, ErrMalformed) { t.Fatalf("op err=%v", err) }
	if _, err := ParseMembershipControl([]byte("ordinary-game-payload")); err == nil { t.Fatal("ordinary payload accepted as membership control") }
}
