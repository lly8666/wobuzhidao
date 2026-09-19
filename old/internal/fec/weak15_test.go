package fec

import (
	"bytes"
	"testing"
	"time"
)

func TestWeak15FastCodecRecoversTenLostSystematicShards(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, err := NewFastBlockEncoderWithParity(codec, 1400, 8*time.Millisecond, 7, WeakParityShards)
	if err != nil {
		t.Fatal(err)
	}
	var wire [][]byte
	want := make([][]byte, DataShards)
	now := time.Unix(1, 0)
	for i := 0; i < DataShards; i++ {
		want[i] = []byte{byte(i + 1), byte(200 - i), byte(i ^ 0x5a)}
		out, err := enc.Add(want[i], now)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range out {
			wire = append(wire, append([]byte(nil), b...))
		}
	}
	if len(wire) != DataShards+WeakParityShards {
		t.Fatalf("wire shards=%d want=%d", len(wire), DataShards+WeakParityShards)
	}
	for _, b := range wire {
		h, err := ParseBlockHeader(b[:HeaderSize])
		if err != nil {
			t.Fatal(err)
		}
		if h.EffectiveParityCount() != WeakParityShards {
			t.Fatalf("parity=%d", h.EffectiveParityCount())
		}
	}
	dec, err := NewBlockDecoderWithParity(NewReedSolomon20x20(), 1400, 8, WeakParityShards)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[byte][]byte)
	// Drop systematic 0..9. Surviving 10 sources plus 10 repair shards are
	// exactly the 20 equations needed to recover all missing originals.
	for _, b := range wire[10:] {
		packets, _, err := dec.Add(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range packets {
			got[p[0]] = append([]byte(nil), p...)
		}
	}
	if len(got) != DataShards {
		t.Fatalf("recovered=%d want=%d", len(got), DataShards)
	}
	for _, p := range want {
		if !bytes.Equal(got[p[0]], p) {
			t.Fatalf("packet %d mismatch", p[0])
		}
	}
}

func TestDecoderRejectsNegotiatedParityMismatch(t *testing.T) {
	enc, _ := NewFastBlockEncoderWithParity(NewReedSolomon20x20(), 1400, time.Millisecond, 1, WeakParityShards)
	out, err := enc.Add([]byte("x"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := NewBlockDecoder(NewReedSolomon20x20(), 1400, 2)
	if _, _, err := dec.Add(out[0]); err != ErrHeaderMismatch {
		t.Fatalf("err=%v want ErrHeaderMismatch", err)
	}
}
