package rawipbackend

import "testing"

func TestSessionMetaRoundTrip(t *testing.T) {
	wire, err := MarshalSessionMeta("7c31a9")
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != MetaLen {
		t.Fatalf("len=%d want=%d", len(wire), MetaLen)
	}
	got, ok := UnmarshalSessionMeta(wire)
	if !ok || got.SID != "7c31a9" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestSessionMetaRejectsInvalidSID(t *testing.T) {
	for _, sid := range []string{"", "123", "zzzzzz", "12345678"} {
		if _, err := MarshalSessionMeta(sid); err == nil {
			t.Fatalf("MarshalSessionMeta(%q) unexpectedly succeeded", sid)
		}
	}
}
