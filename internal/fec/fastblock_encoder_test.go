package fec

import (
	"bytes"
	"testing"
	"time"
)

func TestFastBlockEncoderRoundTripAndPartialFlush(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, err := NewFastBlockEncoder(codec, 1400, 8*time.Millisecond, 77)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := NewBlockDecoder(codec, 1400, 8)
	if err != nil {
		t.Fatal(err)
	}

	want := testPackets(DataShards)
	var wire [][]byte
	for i, p := range want {
		wire, err = enc.Add(p, time.Unix(0, int64(i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(wire) != TotalShards {
		t.Fatalf("wire=%d want=%d", len(wire), TotalShards)
	}
	var got [][]byte
	for i := len(wire) - 1; i >= 0; i -= 2 {
		packets, done, err := dec.Add(wire[i])
		if err != nil {
			t.Fatal(err)
		}
		if done {
			got = packets
			break
		}
	}
	if len(got) != len(want) {
		t.Fatalf("decoded=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d mismatch", i)
		}
	}

	partial := testPackets(3)
	t0 := time.Unix(100, 0)
	for _, p := range partial {
		if out, err := enc.Add(p, t0); err != nil || out != nil {
			t.Fatalf("partial add out=%v err=%v", out != nil, err)
		}
	}
	wire, err = enc.FlushDue(t0.Add(8 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 3+ParityShards {
		t.Fatalf("partial wire=%d want=%d", len(wire), 3+ParityShards)
	}
	dec2, _ := NewBlockDecoder(codec, 1400, 8)
	got = nil
	for i, d := range wire {
		h, err := ParseBlockHeader(d[:HeaderSize])
		if err != nil {
			t.Fatal(err)
		}
		if h.ShardIndex == 1 {
			continue
		}
		packets, done, err := dec2.Add(d)
		if err != nil {
			t.Fatalf("wire %d: %v", i, err)
		}
		if done {
			got = packets
			break
		}
	}
	if len(got) != len(partial) {
		t.Fatalf("partial decoded=%d want=%d", len(got), len(partial))
	}
	for i := range partial {
		if !bytes.Equal(got[i], partial[i]) {
			t.Fatalf("partial packet %d mismatch", i)
		}
	}
}
