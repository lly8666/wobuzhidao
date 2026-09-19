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
	if err != nil {
		t.Fatal(err)
	}
	want := testPackets(DataShards)
	var wire [][]byte
	for i, p := range want {
		got, err := enc.Add(p, time.Unix(0, int64(i)))
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			wire = got
		}
	}
	if len(wire) != TotalShards {
		t.Fatalf("wire=%d want=%d", len(wire), TotalShards)
	}

	// Keep exactly 20 shards in a mixed source/parity pattern, then feed them in
	// reverse order to prove reconstruction is independent of arrival ordering.
	kept := make([][]byte, 0, DataShards)
	for i, d := range wire {
		if i%2 == 0 {
			kept = append(kept, d)
		}
	}
	if len(kept) != DataShards {
		t.Fatalf("kept=%d", len(kept))
	}
	dec, err := NewBlockDecoder(codec, 1400, 8)
	if err != nil {
		t.Fatal(err)
	}
	var got [][]byte
	for i := len(kept) - 1; i >= 0; i-- {
		packets, done, err := dec.Add(kept[i])
		if err != nil {
			t.Fatal(err)
		}
		if done {
			got = packets
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
}

func TestPartialBlockFlushAndRecovery(t *testing.T) {
	codec := NewReedSolomon20x20()
	enc, err := NewBlockEncoder(codec, 1400, 8*time.Millisecond, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := testPackets(3)
	t0 := time.Unix(100, 0)
	for _, p := range want {
		if got, err := enc.Add(p, t0); err != nil || got != nil {
			t.Fatalf("add got=%v err=%v", got != nil, err)
		}
	}
	if got, err := enc.FlushDue(t0.Add(7 * time.Millisecond)); err != nil || got != nil {
		t.Fatalf("early flush got=%v err=%v", got != nil, err)
	}
	wire, err := enc.FlushDue(t0.Add(8 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 3+ParityShards {
		t.Fatalf("wire=%d want=%d", len(wire), 3+ParityShards)
	}

	// Drop source shard 1; the known-zero unused source shards plus parity must
	// still recover all three original packets.
	dec, _ := NewBlockDecoder(codec, 1400, 4)
	var got [][]byte
	for i, d := range wire {
		h, err := ParseBlockHeader(d[:HeaderSize])
		if err != nil {
			t.Fatal(err)
		}
		if h.ShardIndex == 1 {
			continue
		}
		packets, done, err := dec.Add(d)
		if err != nil {
			t.Fatalf("wire %d: %v", i, err)
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
}

func TestBlockDoesNotCompleteBeyondParityBudget(t *testing.T) {
	codec := NewReedSolomon20x20()
	enc, _ := NewBlockEncoder(codec, 1400, time.Millisecond, 1)
	var wire [][]byte
	for _, p := range testPackets(DataShards) {
		wire, _ = enc.Add(p, time.Now())
	}
	dec, _ := NewBlockDecoder(codec, 1400, 4)
	for i := 0; i < DataShards-1; i++ {
		_, done, err := dec.Add(wire[i])
		if err != nil {
			t.Fatal(err)
		}
		if done {
			t.Fatal("decoded with only 19 available shards")
		}
	}
}

func TestBlockDecoderPressurePreservesRecoverableOlderBlock(t *testing.T) {
	codec := NewReedSolomon20x20()
	makeBlock := func(blockID uint32) ([][]byte, [][]byte, [][]byte) {
		t.Helper()
		enc, err := NewFastBlockEncoder(codec, 1400, time.Millisecond, blockID)
		if err != nil {
			t.Fatal(err)
		}
		want := testPackets(DataShards)
		sources := make([][]byte, 0, DataShards)
		var parity [][]byte
		for i, packet := range want {
			out, err := enc.Add(packet, time.Unix(0, int64(i)))
			if err != nil {
				t.Fatal(err)
			}
			if len(out) == 0 {
				t.Fatalf("block %d source %d emitted no systematic shard", blockID, i)
			}
			sources = append(sources, append([]byte(nil), out[0]...))
			if len(out) > 1 {
				for _, wire := range out[1:] {
					parity = append(parity, append([]byte(nil), wire...))
				}
			}
		}
		if len(sources) != DataShards || len(parity) != ParityShards {
			t.Fatalf("block %d sources=%d parity=%d", blockID, len(sources), len(parity))
		}
		return want, sources, parity
	}

	want1, source1, parity1 := makeBlock(1)
	_, source2, _ := makeBlock(2)
	dec, err := NewBlockDecoder(codec, 1400, 1)
	if err != nil {
		t.Fatal(err)
	}
	const missing = 7
	for i, wire := range source1 {
		if i == missing {
			continue
		}
		packets, done, err := dec.Add(wire)
		if err != nil {
			t.Fatalf("block1 source %d: %v", i, err)
		}
		if done {
			t.Fatalf("block1 completed before missing source %d was recoverable", missing)
		}
		if len(packets) != 1 || !bytes.Equal(packets[0], want1[i]) {
			t.Fatalf("block1 source %d first delivery mismatch", i)
		}
	}

	// The only heavy slot is occupied by an older generation that still needs
	// parity to recover source 7. The newer streaming source must first-deliver
	// through compact state instead of evicting the recoverable older block.
	packets, done, err := dec.Add(source2[0])
	if err != nil {
		t.Fatalf("new streaming block under pressure: %v", err)
	}
	if done || len(packets) != 1 {
		t.Fatalf("new streaming block done=%v delivered=%d", done, len(packets))
	}
	if dec.blocks[1] == nil {
		t.Fatal("recoverable older block was retired under decoder pressure")
	}
	if _, ok := dec.retired[2]; !ok {
		t.Fatal("new streaming block did not fall back to compact retired state")
	}

	packets, done, err = dec.Add(parity1[0])
	if err != nil {
		t.Fatalf("block1 parity: %v", err)
	}
	if !done {
		t.Fatal("older block did not complete after parity arrived")
	}
	if len(packets) != 1 || !bytes.Equal(packets[0], want1[missing]) {
		t.Fatalf("recovered=%d want missing source %d", len(packets), missing)
	}
}

func TestBlockDecoderWindowBounded(t *testing.T) {
	codec := NewReedSolomon20x20()
	dec, _ := NewBlockDecoder(codec, 1400, 2)
	for block := uint32(1); block <= 3; block++ {
		enc, _ := NewBlockEncoder(codec, 1400, time.Millisecond, block)
		var wire [][]byte
		for _, packet := range testPackets(DataShards) {
			wire, _ = enc.Add(packet, time.Now())
		}
		if len(wire) != TotalShards {
			t.Fatalf("block %d wire=%d", block, len(wire))
		}
		// Feed only one shard from each full block. The first two blocks remain
		// incomplete/in-flight; opening the third must hit the configured bound.
		_, _, err := dec.Add(wire[0])
		if block < 3 && err != nil {
			t.Fatal(err)
		}
		if block == 3 && !errors.Is(err, ErrDecoderFull) {
			t.Fatalf("got %v want ErrDecoderFull", err)
		}
	}
}
