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

func TestBlockDecoderStreamingWindowPressurePreservesRecoverableOldBlocks(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil {
		t.Fatal(err)
	}
	one := pressureFastWire(t, codec, 1)
	two := pressureFastWire(t, codec, 2)
	three := pressureFastWire(t, codec, 3)

	// A provisional streaming block with only one source observed is not safe to
	// compact: until final metadata arrives it may still be a full generation
	// whose missing originals can be recovered by later parity.
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

	// With every heavy slot still recoverable, a newer streaming generation must
	// fall back to compact first-delivery state instead of evicting either old
	// reconstruction generation.
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
	if dec.blocks[1] == nil || dec.blocks[2] == nil {
		t.Fatalf("recoverable old heavy state was evicted: %v", dec.blocks)
	}
	if _, ok := dec.retired[3]; !ok {
		t.Fatal("new streaming block did not enter bounded compact state")
	}

	// One retained systematic source plus nineteen parity shards provide the
	// twenty equations needed to reconstruct all nineteen missing originals.
	// The first eighteen parity shards must not complete the block; the nineteenth
	// must recover it. This is exactly the recovery value pressure must not destroy.
	want := testPackets(DataShards)
	for p := DataShards; p < DataShards+DataShards-2; p++ {
		packets, done, err = dec.Add(one[p])
		if err != nil {
			t.Fatalf("block 1 parity %d: %v", p-DataShards, err)
		}
		if done || len(packets) != 0 {
			t.Fatalf("block 1 parity %d packets=%d done=%t before recoverable", p-DataShards, len(packets), done)
		}
	}
	packets, done, err = dec.Add(one[DataShards+DataShards-2])
	if err != nil {
		t.Fatalf("block 1 recovery parity: %v", err)
	}
	if !done || len(packets) != DataShards-1 {
		t.Fatalf("block 1 recovered packets=%d done=%t want=%d,true", len(packets), done, DataShards-1)
	}
	for i, packet := range packets {
		if !bytes.Equal(packet, want[i+1]) {
			t.Fatalf("recovered source %d mismatch", i+1)
		}
	}
	if dec.blocks[1] != nil || !dec.completed.contains(1) {
		t.Fatal("recovered oldest block did not complete")
	}
	packets, done, err = dec.Add(one[DataShards+DataShards-1])
	if err != nil {
		t.Fatalf("block 1 late extra parity: %v", err)
	}
	if done || len(packets) != 0 {
		t.Fatalf("block 1 late extra parity packets=%d done=%t want=0,false", len(packets), done)
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
