package fec

import (
	"bytes"
	"testing"
	"time"
)

func pressureFastWire(t *testing.T, codec Codec, blockID uint32) [][]byte {
	t.Helper()
	enc, err := NewFastBlockEncoder(codec, 1400, time.Second, blockID)
	if err != nil {
		t.Fatal(err)
	}
	var wire [][]byte
	for i, packet := range testPackets(DataShards) {
		out, err := enc.Add(packet, time.Unix(1, int64(i)))
		if err != nil {
			t.Fatal(err)
		}
		for _, datagram := range out {
			wire = append(wire, append([]byte(nil), datagram...))
		}
	}
	if len(wire) != TotalShards {
		t.Fatalf("block %d wire=%d want=%d", blockID, len(wire), TotalShards)
	}
	return wire
}

func TestBlockDecoderStreamingWindowPressureRetiresOldestSafeBlock(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil {
		t.Fatal(err)
	}
	one := pressureFastWire(t, codec, 1)
	two := pressureFastWire(t, codec, 2)
	three := pressureFastWire(t, codec, 3)

	// Preserve insertion order explicitly: blockOrder defines "oldest", while Go
	// map iteration is intentionally randomized and made this regression flaky.
	for _, item := range []struct {
		block uint32
		wire  [][]byte
	}{{1, one}, {2, two}} {
		packets, done, err := dec.Add(item.wire[0])
		if err != nil {
			t.Fatalf("block %d first source: %v", item.block, err)
		}
		if done || len(packets) != 1 {
			t.Fatalf("block %d packets=%d done=%t", item.block, len(packets), done)
		}
	}
	if dec.InFlight() != 2 {
		t.Fatalf("in_flight=%d want=2", dec.InFlight())
	}

	packets, done, err := dec.Add(three[0])
	if err != nil {
		t.Fatalf("new streaming block hit pressure error: %v", err)
	}
	if done || len(packets) != 1 {
		t.Fatalf("block 3 packets=%d done=%t", len(packets), done)
	}
	if dec.InFlight() != 2 {
		t.Fatalf("in_flight=%d want=2 after pressure", dec.InFlight())
	}
	if _, ok := dec.blocks[1]; ok {
		t.Fatal("oldest safe block 1 remained in heavy window")
	}
	if _, ok := dec.retired[1]; !ok {
		t.Fatal("oldest safe block 1 missing bounded retired state")
	}
	if dec.blocks[2] == nil || dec.blocks[3] == nil {
		t.Fatalf("heavy blocks after pressure: %v", dec.blocks)
	}

	// Final parity supplies authoritative metadata without reopening heavy state.
	packets, done, err = dec.Add(one[DataShards])
	if err != nil {
		t.Fatalf("late parity: %v", err)
	}
	if done || len(packets) != 0 || dec.InFlight() != 2 {
		t.Fatalf("late parity packets=%d done=%t in_flight=%d", len(packets), done, dec.InFlight())
	}

	// Lower-layer ARQ can finish the compacted block from systematic repairs,
	// each delivered exactly once.
	want := testPackets(DataShards)
	for i := 1; i < DataShards; i++ {
		packets, done, err = dec.Add(one[i])
		if err != nil {
			t.Fatalf("late source %d: %v", i, err)
		}
		if len(packets) != 1 || !bytes.Equal(packets[0], want[i]) {
			t.Fatalf("late source %d packets=%d", i, len(packets))
		}
		if i == 1 {
			dup, dupDone, dupErr := dec.Add(one[i])
			if dupErr != nil || dupDone || len(dup) != 0 {
				t.Fatalf("duplicate late source: packets=%d done=%t err=%v", len(dup), dupDone, dupErr)
			}
		}
		if done != (i == DataShards-1) {
			t.Fatalf("source %d done=%t", i, done)
		}
	}
	if _, ok := dec.retired[1]; ok {
		t.Fatal("completed retired block 1 retained")
	}
	if !dec.completed.contains(1) {
		t.Fatal("completed retired block 1 missing completion history")
	}
}

func TestBlockDecoderStreamingPressureDoesNotEvictUnsafeReferenceState(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Final-header source shards from the reference encoder are not delivered
	// until reconstruction, so these heavy blocks are unsafe to compact.
	for id := uint32(10); id <= 11; id++ {
		wire := testBlockWire(t, codec, id)
		if _, _, err := dec.Add(wire[0]); err != nil {
			t.Fatal(err)
		}
	}
	if canRetireBlock(dec.blocks[10]) || canRetireBlock(dec.blocks[11]) {
		t.Fatal("reference blocks unexpectedly retirable")
	}

	streaming := pressureFastWire(t, codec, 12)
	packets, done, err := dec.Add(streaming[0])
	if err != nil {
		t.Fatalf("streaming source blocked by unsafe heavy state: %v", err)
	}
	if done || len(packets) != 1 {
		t.Fatalf("streaming packets=%d done=%t", len(packets), done)
	}
	if dec.InFlight() != 2 || dec.blocks[10] == nil || dec.blocks[11] == nil {
		t.Fatalf("unsafe heavy state was evicted: in_flight=%d blocks=%v", dec.InFlight(), dec.blocks)
	}
	if _, ok := dec.retired[12]; !ok {
		t.Fatal("new streaming block did not enter bounded ARQ-only state")
	}

	// The incoming compact state must still make forward progress from later
	// final metadata plus systematic retransmissions without taking a heavy slot.
	if packets, done, err = dec.Add(streaming[DataShards]); err != nil || done || len(packets) != 0 {
		t.Fatalf("incoming retired parity packets=%d done=%t err=%v", len(packets), done, err)
	}
	want := testPackets(DataShards)
	for i := 1; i < DataShards; i++ {
		packets, done, err = dec.Add(streaming[i])
		if err != nil || len(packets) != 1 || !bytes.Equal(packets[0], want[i]) {
			t.Fatalf("incoming retired source %d packets=%d done=%t err=%v", i, len(packets), done, err)
		}
	}
	if !done || dec.InFlight() != 2 || !dec.completed.contains(12) {
		t.Fatalf("incoming retired completion done=%t in_flight=%d completed=%t", done, dec.InFlight(), dec.completed.contains(12))
	}
}

func TestBlockDecoderRetiredStateHardBound(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 1)
	if err != nil {
		t.Fatal(err)
	}

	const extra = 17
	for id := uint32(1); id <= uint32(maxRetiredBlocks+extra); id++ {
		dec.addRetiredState(id, retiredBlock{})
		if len(dec.retired) > maxRetiredBlocks {
			t.Fatalf("retired=%d exceeds hard cap=%d", len(dec.retired), maxRetiredBlocks)
		}
	}
	if got := len(dec.retired); got != maxRetiredBlocks {
		t.Fatalf("retired=%d want=%d", got, maxRetiredBlocks)
	}
	for id := uint32(1); id <= extra; id++ {
		if !dec.completed.contains(id) {
			t.Fatalf("aged-out retired id %d not folded into completed frontier", id)
		}
		if _, ok := dec.retired[id]; ok {
			t.Fatalf("aged-out retired id %d still retained", id)
		}
	}
}
