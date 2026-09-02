package gamelane

import (
	"errors"
	"reflect"
	"testing"
)

func TestLaneSetControlAcceptsDormantFourLogicalLanesAndOneReplacementTarget(t *testing.T) {
	dormant, err := ParseLaneSetCommand([]byte(`{"op":"set","lanes":[]}`))
	if err != nil { t.Fatal(err) }
	if len(dormant.Lanes) != 0 { t.Fatalf("dormant lanes=%v", dormant.Lanes) }

	cmd, err := ParseLaneSetCommand([]byte(`{"op":"set","lanes":[{"id":4,"address":"127.0.0.1:47105"},{"id":1,"address":"127.0.0.1:47101"},{"id":3,"address":"127.0.0.1:47103"},{"id":4,"address":"127.0.0.1:47104"},{"id":2,"address":"127.0.0.1:47102"}]}`))
	if err != nil { t.Fatal(err) }
	got := CanonicalLaneTargets(cmd.Lanes)
	want := []LaneTarget{{ID:1,Address:"127.0.0.1:47101"},{ID:2,Address:"127.0.0.1:47102"},{ID:3,Address:"127.0.0.1:47103"},{ID:4,Address:"127.0.0.1:47104"},{ID:4,Address:"127.0.0.1:47105"}}
	if !reflect.DeepEqual(got, want) { t.Fatalf("canonical=%v want=%v", got, want) }
}

func TestLaneSetControlRejectsInvalidMembership(t *testing.T) {
	cases := []string{
		`{"op":"add","lanes":[]}`,
		`{"op":"set","lanes":[{"id":1,"address":"127.0.0.1:47101"},{"id":1,"address":"127.0.0.1:47102"},{"id":1,"address":"127.0.0.1:47103"}]}`,
		`{"op":"set","lanes":[{"id":1,"address":"127.0.0.1:47101"},{"id":1,"address":"127.0.0.1:47102"},{"id":2,"address":"127.0.0.1:47103"},{"id":2,"address":"127.0.0.1:47104"}]}`,
		`{"op":"set","lanes":[{"id":5,"address":"127.0.0.1:47105"}]}`,
		`{"op":"set","lanes":[{"id":1,"address":"10.0.0.1:47101"}]}`,
		`{"op":"set","lanes":[{"id":1,"address":"127.0.0.1:0"}]}`,
		`{"op":"set","lanes":[{"id":1,"address":"127.0.0.1:47101"},{"id":2,"address":"127.0.0.1:47101"}]}`,
	}
	for _, raw := range cases {
		if _, err := ParseLaneSetCommand([]byte(raw)); err == nil {
			t.Fatalf("invalid command accepted: %s", raw)
		}
	}

	sixTargets := LaneSetCommand{Op:LaneControlSet,Lanes:[]LaneTarget{{ID:1,Address:"127.0.0.1:1"},{ID:1,Address:"127.0.0.1:2"},{ID:2,Address:"127.0.0.1:3"},{ID:3,Address:"127.0.0.1:4"},{ID:4,Address:"127.0.0.1:5"},{ID:4,Address:"127.0.0.1:6"}}}
	if err := sixTargets.Validate(); !errors.Is(err, ErrLanes) { t.Fatalf("six targets err=%v", err) }
}
