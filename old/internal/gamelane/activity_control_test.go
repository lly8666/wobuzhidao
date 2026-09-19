package gamelane

import "testing"

func TestLaneControlOpDispatchesSetAndActivity(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{`{"op":"set","lanes":[]}`, LaneControlSet},
		{`{"op":"activity"}`, LaneControlActivity},
	} {
		got, err := ParseLaneControlOp([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("op=%q want=%q", got, tc.want)
		}
	}
}

func TestLaneActivityControlIsStrict(t *testing.T) {
	cmd, err := ParseLaneActivityCommand([]byte(`{"op":"activity"}`))
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Op != LaneControlActivity {
		t.Fatalf("op=%q", cmd.Op)
	}
	for _, raw := range []string{
		`{"op":"set"}`,
		`{"op":"activity","lanes":[]}`,
		`{"op":"activity"} {"op":"activity"}`,
	} {
		if _, err := ParseLaneActivityCommand([]byte(raw)); err == nil {
			t.Fatalf("invalid activity command accepted: %s", raw)
		}
	}
}
