package fec

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func appendDecoded(got *[][]byte, packets [][]byte) {
	for _, p := range packets {
		*got = append(*got, p)
	}
}

func TestFastBlockEncoderStreamsSystematicImmediately(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, err := NewFastBlockEncoder(codec, 1400, 8*time.Millisecond, 77)
	if err != nil { t.Fatal(err) }
	p := []byte("first-complete-inner-packet")
	out, err := enc.Add(p, time.Unix(1, 0))
	if err != nil { t.Fatal(err) }
	if len(out) != 1 { t.Fatalf("first Add emitted %d datagrams, want 1", len(out)) }
	if len(out[0]) != HeaderSize+len(p) { t.Fatalf("streaming wire size=%d", len(out[0])) }
	h, err := ParseBlockHeader(out[0][:HeaderSize])
	if err != nil { t.Fatal(err) }
	if h.BlockID != 77 || h.ShardIndex != 0 || h.ShardSize != uint16(len(p)) {
		t.Fatalf("bad streaming header: %+v", h)
	}
	if binary.BigEndian.Uint16(out[0][14:16]) != headerFlagStreamingSystematic {
		t.Fatalf("streaming flag missing: %x", out[0][14:16])
	}
	if !bytes.Equal(out[0][HeaderSize:], p) { t.Fatal("streaming payload mismatch") }

	dec, _ := NewBlockDecoder(codec, 1400, 8)
	packets, done, err := dec.Add(out[0])
	if err != nil { t.Fatal(err) }
	if done { t.Fatal("one source unexpectedly completed block") }
	if len(packets) != 1 || !bytes.Equal(packets[0], p) {
		t.Fatalf("first source not immediately decoded: %#v", packets)
	}
}

func TestFastBlockEncoderRoundTripTwentyMissingReordered(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, err := NewFastBlockEncoder(codec, 1400, 8*time.Millisecond, 100)
	if err != nil { t.Fatal(err) }
	want := testPackets(DataShards)
	wire := make([][]byte, 0, TotalShards)
	for i, p := range want {
		out, err := enc.Add(p, time.Unix(0, int64(i)))
		if err != nil { t.Fatal(err) }
		if i < DataShards-1 && len(out) != 1 {
			t.Fatalf("Add %d emitted %d, want 1 streaming source", i, len(out))
		}
		if i == DataShards-1 && len(out) != 1+ParityShards {
			t.Fatalf("final Add emitted %d, want %d", len(out), 1+ParityShards)
		}
		wire = append(wire, out...)
	}
	if len(wire) != TotalShards { t.Fatalf("wire=%d want=%d", len(wire), TotalShards) }

	// Keep exactly 20 mixed source/parity datagrams and reverse arrival order.
	kept := make([][]byte, 0, DataShards)
	for i, d := range wire {
		if i%2 == 0 { kept = append(kept, d) }
	}
	dec, _ := NewBlockDecoder(codec, 1400, 8)
	var got [][]byte
	completed := false
	for i := len(kept)-1; i >= 0; i-- {
		packets, done, err := dec.Add(kept[i])
		if err != nil { t.Fatalf("wire %d: %v", i, err) }
		appendDecoded(&got, packets)
		if done { completed = true }
	}
	if !completed { t.Fatal("mixed 20-shard set did not complete") }
	if len(got) != len(want) { t.Fatalf("decoded=%d want=%d", len(got), len(want)) }
	seen := make(map[byte][]byte)
	for _, p := range got { if len(p) != 0 { seen[p[0]] = p } }
	for i := range want {
		if !bytes.Equal(seen[byte(i+1)], want[i]) { t.Fatalf("packet %d mismatch", i) }
	}
}

func TestFastBlockEncoderPartialParityMatchesDataCountAndRecoversAllLostSources(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, _ := NewFastBlockEncoder(codec, 1400, 8*time.Millisecond, 7)
	want := testPackets(3)
	t0 := time.Unix(100, 0)
	for i, p := range want {
		out, err := enc.Add(p, t0.Add(time.Duration(i)*time.Millisecond))
		if err != nil { t.Fatal(err) }
		if len(out) != 1 { t.Fatalf("partial Add %d emitted %d", i, len(out)) }
	}
	if out, err := enc.FlushDue(t0.Add(7*time.Millisecond)); err != nil || out != nil {
		t.Fatalf("early flush out=%d err=%v", len(out), err)
	}
	parity, err := enc.FlushDue(t0.Add(8*time.Millisecond))
	if err != nil { t.Fatal(err) }
	if len(parity) != len(want) { t.Fatalf("partial parity=%d want dataCount=%d", len(parity), len(want)) }

	// Drop every streaming source. The three parity shards plus the 17 known-zero
	// unused systematic slots must still provide the 20 equations needed to
	// reconstruct all three real source packets.
	dec, _ := NewBlockDecoder(codec, 1400, 8)
	var got [][]byte
	completed := false
	for i, d := range parity {
		packets, done, err := dec.Add(d)
		if err != nil { t.Fatalf("parity %d: %v", i, err) }
		appendDecoded(&got, packets)
		if done { completed = true }
	}
	if !completed { t.Fatal("partial block did not complete from parity-only input") }
	if len(got) != len(want) { t.Fatalf("decoded=%d want=%d", len(got), len(want)) }
	for i := range want {
		if !bytes.Equal(got[i], want[i]) { t.Fatalf("partial packet %d mismatch", i) }
	}
}

func TestFastBlockEncoderSinglePacketFlushIsTwoDatagramsNotTwentyOne(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, _ := NewFastBlockEncoder(codec, 1400, 8*time.Millisecond, 11)
	t0 := time.Unix(200, 0)
	if out, err := enc.Add([]byte("one"), t0); err != nil || len(out) != 1 {
		t.Fatalf("Add out=%d err=%v", len(out), err)
	}
	parity, err := enc.FlushDue(t0.Add(8 * time.Millisecond))
	if err != nil { t.Fatal(err) }
	if len(parity) != 1 {
		t.Fatalf("single-packet partial block parity=%d want=1", len(parity))
	}
}

func TestFastBlockAllSourcesCompleteWithoutParity(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, _ := NewFastBlockEncoder(codec, 1400, time.Second, 9)
	dec, _ := NewBlockDecoder(codec, 1400, 2)
	want := testPackets(DataShards)
	count := 0
	completed := false
	for i, p := range want {
		out, err := enc.Add(p, time.Unix(1, int64(i)))
		if err != nil { t.Fatal(err) }
		// Ignore parity from the final Add; feed only the streaming source.
		packets, done, err := dec.Add(out[0])
		if err != nil { t.Fatal(err) }
		count += len(packets)
		if done { completed = true }
	}
	if !completed || count != DataShards || dec.InFlight() != 0 {
		t.Fatalf("completed=%v delivered=%d inflight=%d", completed, count, dec.InFlight())
	}
}
