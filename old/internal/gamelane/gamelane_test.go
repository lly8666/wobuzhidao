package gamelane

import (
	"bytes"
	"errors"
	"testing"
)

func sid(v byte) SessionID {
	var id SessionID
	for i := range id {
		id[i] = v + byte(i)
	}
	return id
}

func TestFourLaneCopiesAreDistinctBeforeDTLS(t *testing.T) {
	enc, err := NewEncoder(sid(1), 1)
	if err != nil { t.Fatal(err) }
	id, copies, err := enc.WrapCopies([]byte("same-inner-game-datagram"), []uint8{1, 2, 3, 4})
	if err != nil { t.Fatal(err) }
	if id != 1 || len(copies) != 4 { t.Fatalf("id=%d copies=%d", id, len(copies)) }
	for i, c := range copies {
		h, payload, err := Parse(c.Wire)
		if err != nil { t.Fatal(err) }
		if h.PacketID != 1 || h.LaneID != uint8(i+1) || string(payload) != "same-inner-game-datagram" {
			t.Fatalf("copy[%d] header=%+v payload=%q", i, h, payload)
		}
		for j := 0; j < i; j++ {
			if bytes.Equal(c.Wire, copies[j].Wire) {
				t.Fatalf("lane %d and %d plaintext envelopes identical", i+1, j+1)
			}
		}
	}
}

func TestRaceFirstArrivalWinsAndOtherLaneCopiesSuppress(t *testing.T) {
	enc, _ := NewEncoder(sid(2), 10)
	dec, _ := NewDecoder(sid(2), 64)
	_, copies, err := enc.WrapCopies([]byte("game-input"), []uint8{1, 2, 3, 4})
	if err != nil { t.Fatal(err) }

	first, err := dec.Add(copies[2].Wire)
	if err != nil { t.Fatal(err) }
	if !first.Deliver || first.Duplicate || first.LaneID != 3 || first.PacketID != 10 || string(first.Payload) != "game-input" {
		t.Fatalf("first=%+v", first)
	}
	for _, i := range []int{0, 1, 3} {
		r, err := dec.Add(copies[i].Wire)
		if err != nil { t.Fatal(err) }
		if r.Deliver || !r.Duplicate || r.PacketID != 10 || r.LaneID != copies[i].LaneID {
			t.Fatalf("lane=%d result=%+v", i+1, r)
		}
	}
}

func TestOutOfOrderUniquePacketsDoNotHOL(t *testing.T) {
	enc, _ := NewEncoder(sid(3), 100)
	dec, _ := NewDecoder(sid(3), 64)
	_, a, _ := enc.WrapCopies([]byte("a"), []uint8{1, 2})
	_, b, _ := enc.WrapCopies([]byte("b"), []uint8{1, 2})
	_, c, _ := enc.WrapCopies([]byte("c"), []uint8{1, 2})

	for i, x := range []struct{ wire []byte; want string }{{c[1].Wire,"c"},{a[0].Wire,"a"},{b[1].Wire,"b"}} {
		r, err := dec.Add(x.wire)
		if err != nil { t.Fatalf("%d: %v", i, err) }
		if !r.Deliver || string(r.Payload) != x.want {
			t.Fatalf("%d: %+v", i, r)
		}
	}
}

func TestReplayWindowBoundsMemoryAndRejectsVeryLateDuplicate(t *testing.T) {
	enc, _ := NewEncoder(sid(4), 1)
	dec, _ := NewDecoder(sid(4), 64)
	var oldest []byte
	for i := 0; i < 200; i++ {
		_, copies, err := enc.WrapCopies([]byte{byte(i)}, []uint8{1, 2, 3, 4})
		if err != nil { t.Fatal(err) }
		if i == 0 { oldest = append([]byte(nil), copies[0].Wire...) }
		r, err := dec.Add(copies[i%len(copies)].Wire)
		if err != nil || !r.Deliver { t.Fatalf("i=%d r=%+v err=%v", i, r, err) }
	}
	if dec.Recent() > 64 { t.Fatalf("recent=%d", dec.Recent()) }
	r, err := dec.Add(oldest)
	if !errors.Is(err, ErrReplayTooOld) || !r.Stale || r.Deliver {
		t.Fatalf("stale r=%+v err=%v", r, err)
	}
}

func TestWrongLogicalSessionRejected(t *testing.T) {
	enc, _ := NewEncoder(sid(5), 1)
	dec, _ := NewDecoder(sid(6), 64)
	_, wire, _ := enc.Wrap([]byte("x"))
	if _, err := dec.Add(wire); !errors.Is(err, ErrWrongSession) {
		t.Fatalf("err=%v", err)
	}
}

func TestRejectsInvalidLaneSetAndReservedByte(t *testing.T) {
	enc, _ := NewEncoder(sid(7), 1)
	for _, lanes := range [][]uint8{{}, {0}, {1,1}, {1,2,3,4,4}, {5}} {
		if _, _, err := enc.WrapCopies([]byte("abc"), lanes); !errors.Is(err, ErrMalformed) {
			t.Fatalf("lanes=%v err=%v", lanes, err)
		}
	}
	_, wire, _ := enc.Wrap([]byte("abc"))
	reserved := append([]byte(nil), wire...)
	reserved[31] = 1
	if _, _, err := Parse(reserved); !errors.Is(err, ErrMalformed) {
		t.Fatalf("reserved err=%v", err)
	}
}
