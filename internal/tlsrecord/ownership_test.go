package tlsrecord

import (
	"bytes"
	"testing"
)

func TestSealedAndOpenedRecordsOwnTheirStorage(t *testing.T) {
	keys := testKeys(t)
	s, err := NewSealer(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}
	o, err := NewOpener(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}
	for _, padding := range []int{0, 17, 511} {
		input := []byte{1, 2, 0, 3}
		wire, _, err := s.SealWithPadding(input, padding)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := append([]byte(nil), wire...)
		input[0] = 99
		r, err := o.OpenRecord(wire)
		if err != nil {
			t.Fatal(err)
		}
		want := []byte{1, 2, 0, 3}
		if !bytes.Equal(r.Payload, want) || cap(r.Payload) != len(r.Payload) {
			t.Fatalf("padding=%d payload=%x cap=%d", padding, r.Payload, cap(r.Payload))
		}
		other, _, err := s.SealHealth([]byte("other"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wire, snapshot) {
			t.Fatal("later seal overwrote retained wire")
		}
		r2, err := o.OpenRecord(other)
		if err != nil {
			t.Fatal(err)
		}
		r2.Payload[0] = 0
		for i := range wire {
			wire[i] = 0
		}
		if !bytes.Equal(r.Payload, want) {
			t.Fatal("opened payload aliases wire or later open")
		}
		if got := append(r.Payload, 9); !bytes.Equal(got, []byte{1, 2, 0, 3, 9}) {
			t.Fatalf("append=%x", got)
		}
	}
}
