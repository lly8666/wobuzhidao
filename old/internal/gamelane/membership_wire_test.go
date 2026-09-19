package gamelane

import (
	"errors"
	"testing"
)

func TestMembershipControlRoundTrip(t *testing.T) {
	var sid SessionID
	for i := range sid { sid[i] = byte(i + 1) }
	cases := []struct {
		name string
		op   MembershipOp
		make func(SessionID, uint8) ([]byte, error)
	}{
		{"leave", MembershipLeave, MarshalLaneLeave},
		{"probe", MembershipProbe, MarshalLaneProbe},
		{"ready", MembershipReady, MarshalLaneReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := tc.make(sid, 4)
			if err != nil { t.Fatal(err) }
			got, err := ParseMembershipControl(wire)
			if err != nil { t.Fatal(err) }
			if got.SessionID != sid || got.LaneID != 4 || got.Op != tc.op {
				t.Fatalf("control=%+v", got)
			}
		})
	}
}

func TestMembershipControlRejectsMalformed(t *testing.T) {
	var sid SessionID
	sid[0] = 1
	if _, err := MarshalLaneLeave(sid, 0); !errors.Is(err, ErrMalformed) { t.Fatalf("lane zero err=%v", err) }
	wire, _ := MarshalLaneProbe(sid, 1)
	wire[21] = 9
	if _, err := ParseMembershipControl(wire); !errors.Is(err, ErrMalformed) { t.Fatalf("op err=%v", err) }
	if _, err := ParseMembershipControl([]byte("ordinary-game-payload")); err == nil { t.Fatal("ordinary payload accepted as membership control") }
}
