package fec

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func testPackets(n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		ln := 80 + (i*67)%1000
		out[i] = bytes.Repeat([]byte{byte(i + 1)}, ln)
	}
	return out
}

func TestBlockRoundTripTwentyMissingReordered(t *testing.T) {
	codec := NewReedSolomon20x20()
	enc, err := NewBlockEncoder(codec, 1400, 8*time.Millisecond, 100)
	if err != nil { t.Fatal(err) }
	want := testPackets(DataShards)
	var wire [][]byte
	for i, p := range want {
		got, err := enc.Add(p, time.Unix(0, int64(i)))
		if err != nil { t.Fatal(err) }
		if got != nil { wire = got }
	}
	if len(wire) != TotalShards { t.Fatalf("wire=%d want=%d", len(wire), TotalShards) }

	// Keep exactly 20 shards in a mixed source/parity pattern, then feed them in
	// reverse order to prove reconstruction is independent of arrival ordering.
	kept := make([][]byte, 0, DataShards)
	for i, d := range wire {
		if i%2 == 0 { kept = append(kept, d) }
	}
	if len(kept) != DataShards { t.Fatalf("kept=%d", len(kept)) }
	dec, err := NewBlockDecoder(codec, 1400, 8)
	if err != nil { t.Fatal(err) }
	var got [][]byte
	for i := len(kept)-1; i >= 0; i-- {
		packets, done, err := dec.Add(kept[i])
		if err != nil { t.Fatal(err) }
		if done { got = packets }
	}
	if len(got) != len(want) { t.Fatalf("decoded=%d want=%d", len(got), len(want)) }
	for i := range want {
		if !bytes.Equal(got[i], want[i]) { t.Fatalf("packet %d mismatch", i) }
	}
}

func TestPartialBlockFlushAndRecovery(t *testing.T) {
	codec := NewReedSolomon20x20()
	enc, err := NewBlockEncoder(codec, 1400, 8*time.Millisecond, 7)
	if err != nil { t.Fatal(err) }
	want := testPackets(3)
	t0 := time.Unix(100, 0)
	for _, p := range want {
		if got, err := enc.Add(p, t0); err != nil || got != nil { t.Fatalf("add got=%v err=%v", got != nil, err) }
	}
	if got, err := enc.FlushDue(t0.Add(7*time.Millisecond)); err != nil || got != nil { t.Fatalf("early flush got=%v err=%v", got != nil, err) }
	wire, err := enc.FlushDue(t0.Add(8*time.Millisecond))
	if err != nil { t.Fatal(err) }
	if len(wire) != 3+ParityShards { t.Fatalf("wire=%d want=%d", len(wire), 3+ParityShards) }

	// Drop source shard 1; the known-zero unused source shards plus parity must
	// still recover all three original packets.
	dec, _ := NewBlockDecoder(codec, 1400, 4)
	var got [][]byte
	for i, d := range wire {
		h, err := ParseBlockHeader(d[:HeaderSize])
		if err != nil { t.Fatal(err) }
		if h.ShardIndex == 1 { continue }
		packets, done, err := dec.Add(d)
		if err != nil { t.Fatalf("wire %d: %v", i, err) }
		if done { got = packets; break }
	}
	if len(got) != len(want) { t.Fatalf("decoded=%d want=%d", len(got), len(want)) }
	for i := range want {
		if !bytes.Equal(got[i], want[i]) { t.Fatalf("packet %d mismatch", i) }
	}
}

func TestBlockDoesNotCompleteBeyondParityBudget(t *testing.T) {
	codec := NewReedSolomon20x20()
	enc, _ := NewBlockEncoder(codec, 1400, time.Millisecond, 1)
	var wire [][]byte
	for _, p := range testPackets(DataShards) { wire, _ = enc.Add(p, time.Now()) }
	dec, _ := NewBlockDecoder(codec, 1400, 4)
	for i := 0; i < DataShards-1; i++ {
		_, done, err := dec.Add(wire[i])
		if err != nil { t.Fatal(err) }
		if done { t.Fatal("decoded with only 19 available shards") }
	}
}

func TestBlockDecoderWindowBounded(t *testing.T) {
	codec := NewReedSolomon20x20()
	dec, _ := NewBlockDecoder(codec, 1400, 2)
	for block := uint32(1); block <= 3; block++ {
		enc, _ := NewBlockEncoder(codec, 1400, time.Millisecond, block)
		wire, err := enc.Add([]byte{1,2,3}, time.Now())
		if err != nil { t.Fatal(err) }
		if wire != nil { t.Fatal("unexpected full flush") }
		wire, err = enc.Flush()
		if err != nil { t.Fatal(err) }
		_, _, err = dec.Add(wire[0])
		if block < 3 && err != nil { t.Fatal(err) }
		if block == 3 && !errors.Is(err, ErrDecoderFull) { t.Fatalf("got %v want ErrDecoderFull", err) }
	}
}
