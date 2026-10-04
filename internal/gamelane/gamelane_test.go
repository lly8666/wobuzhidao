package gamelane

import (
	"bytes"
	"errors"
	"encoding/binary"
	"math"
	"math/rand"
	"testing"
)

func testSession(v byte) SessionID {
	var id SessionID
	for i := range id {
		id[i] = v + byte(i)
	}
	return id
}

// Compare delivery, duplicate/stale classification and retained membership with
// the original exhaustive-window semantics across gaps, reordering and wrap
// boundaries. This is a semantic oracle, not a timing-sensitive benchmark.
func TestReplayWindowIncrementalEvictionMatchesReference(t *testing.T) {
 for _,window:=range []int{64,65,4096} {
  enc,_:=NewEncoder(testSession(11),1);_,copies,_:=enc.WrapCopies([]byte("payload"),[]uint8{1});wire:=copies[0].Wire
  d,_:=NewDecoder(testSession(11),window);seen:=map[uint64]bool{};var high uint64
  rng:=rand.New(rand.NewSource(5205))
  ids:=[]uint64{1,2,1000000000000,1000000000001,999999999999,math.MaxUint64-5000}
  for i:=0;i<6000;i++ { ids=append(ids,math.MaxUint64-4000+uint64(rng.Intn(3999))) }
  for _,id:=range ids {
   binary.BigEndian.PutUint64(wire[20:28],id)
   stale:=high>id&&high-id>=uint64(window);duplicate:=!stale&&seen[id]
   if !stale&&!duplicate {if id>high{high=id};for old:=range seen{if high>old&&high-old>=uint64(window){delete(seen,old)}};seen[id]=true}
   got,e:=d.Add(wire)
   if got.Stale!=stale||got.Duplicate!=duplicate||got.Deliver!=(!stale&&!duplicate)||(e!=nil)!=stale{t.Fatalf("window=%d id=%d result=%+v err=%v",window,id,got,e)}
   if d.Recent()!=len(seen){t.Fatalf("window=%d retained=%d want=%d",window,d.Recent(),len(seen))}
   for old:=range seen{if _,ok:=d.seen[old];!ok{t.Fatalf("lost retained ID %d",old)}}
  }
 }
}

func TestWrapCopiesUseOnePacketIDAndDistinctLaneEnvelope(t *testing.T) {
	enc, err := NewEncoder(testSession(1), 1)
	if err != nil {
		t.Fatal(err)
	}
	id, copies, err := enc.WrapCopies([]byte("same-inner-game-datagram"), []uint8{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 || len(copies) != 4 {
		t.Fatalf("id=%d copies=%d", id, len(copies))
	}
	for i, c := range copies {
		h, payload, err := Parse(c.Wire)
		if err != nil {
			t.Fatal(err)
		}
		if h.PacketID != 1 || h.LaneID != uint8(i+1) || string(payload) != "same-inner-game-datagram" {
			t.Fatalf("copy[%d] header=%+v payload=%q", i, h, payload)
		}
		for j := 0; j < i; j++ {
			if bytes.Equal(c.Wire, copies[j].Wire) {
				t.Fatalf("lane %d and %d envelopes identical", i+1, j+1)
			}
		}
	}
}

func TestFirstArrivalWinsAndOtherLaneCopiesDeduplicate(t *testing.T) {
	enc, _ := NewEncoder(testSession(2), 10)
	dec, _ := NewDecoder(testSession(2), 64)
	_, copies, err := enc.WrapCopies([]byte("game-input"), []uint8{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	first, err := dec.Add(copies[2].Wire)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Deliver || first.Duplicate || first.LaneID != 3 || first.PacketID != 10 || string(first.Payload) != "game-input" {
		t.Fatalf("first=%+v", first)
	}
	for _, i := range []int{0, 1, 3} {
		r, err := dec.Add(copies[i].Wire)
		if err != nil {
			t.Fatal(err)
		}
		if r.Deliver || !r.Duplicate || r.PacketID != 10 || r.LaneID != copies[i].LaneID {
			t.Fatalf("copy=%d result=%+v", i, r)
		}
	}
}

func TestOutOfOrderUniquePacketIDsDoNotHOL(t *testing.T) {
	enc, _ := NewEncoder(testSession(3), 100)
	dec, _ := NewDecoder(testSession(3), 64)
	_, a, _ := enc.WrapCopies([]byte("a"), []uint8{1, 2})
	_, b, _ := enc.WrapCopies([]byte("b"), []uint8{1, 2})
	_, c, _ := enc.WrapCopies([]byte("c"), []uint8{1, 2})
	for i, x := range []struct {
		wire []byte
		want string
	}{{c[1].Wire, "c"}, {a[0].Wire, "a"}, {b[1].Wire, "b"}} {
		r, err := dec.Add(x.wire)
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if !r.Deliver || string(r.Payload) != x.want {
			t.Fatalf("%d: result=%+v", i, r)
		}
	}
}

func TestReplayWindowIsBoundedAndVeryLatePacketIsStale(t *testing.T) {
	enc, _ := NewEncoder(testSession(4), 1)
	dec, _ := NewDecoder(testSession(4), 64)
	var oldest []byte
	for i := 0; i < 200; i++ {
		_, copies, err := enc.WrapCopies([]byte{byte(i)}, []uint8{1, 2, 3, 4})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldest = append([]byte(nil), copies[0].Wire...)
		}
		r, err := dec.Add(copies[i%len(copies)].Wire)
		if err != nil || !r.Deliver {
			t.Fatalf("i=%d result=%+v err=%v", i, r, err)
		}
	}
	if dec.Recent() > 64 {
		t.Fatalf("recent=%d", dec.Recent())
	}
	r, err := dec.Add(oldest)
	if !errors.Is(err, ErrReplayTooOld) || !r.Stale || r.Deliver {
		t.Fatalf("stale result=%+v err=%v", r, err)
	}
}

func TestWrongSessionAndInvalidLaneSetFailClosed(t *testing.T) {
	enc, _ := NewEncoder(testSession(5), 1)
	dec, _ := NewDecoder(testSession(6), 64)
	_, copies, _ := enc.WrapCopies([]byte("x"), []uint8{1})
	if _, err := dec.Add(copies[0].Wire); !errors.Is(err, ErrWrongSession) {
		t.Fatalf("wrong session err=%v", err)
	}
	for _, lanes := range [][]uint8{{}, {0}, {1, 1}, {1, 2, 3, 4, 4}, {5}} {
		if _, _, err := enc.WrapCopies([]byte("abc"), lanes); !errors.Is(err, ErrMalformed) {
			t.Fatalf("lanes=%v err=%v", lanes, err)
		}
	}
	reserved := append([]byte(nil), copies[0].Wire...)
	reserved[31] = 1
	if _, _, err := Parse(reserved); !errors.Is(err, ErrMalformed) {
		t.Fatalf("reserved err=%v", err)
	}
}
