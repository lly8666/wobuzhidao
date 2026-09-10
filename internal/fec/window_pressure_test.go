package fec

import (
	"bytes"
	"testing"
	"time"
)

func fastTestBlockWire(t *testing.T, codec Codec, blockID uint32) [][]byte {
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

func TestBlockDecoderStreamingWindowPressureDegradesOldest(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil {
		t.Fatal(err)
	}
	one := fastTestBlockWire(t, codec, 1)
	two := fastTestBlockWire(t, codec, 2)
	three := fastTestBlockWire(t, codec, 3)

	for block, wire := range map[uint32][][]byte{1: one, 2: two} {
		packets, done, err := dec.Add(wire[0])
		if err != nil {
			t.Fatalf("block %d first source: %v", block, err)
		}
		if done || len(packets) != 1 {
			t.Fatalf("block %d packets=%d done=%t", block, len(packets), done)
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
		t.Fatal("oldest block 1 remained in heavy window")
	}
	if _, ok := dec.degraded[1]; !ok {
		t.Fatal("oldest block 1 missing degraded ARQ state")
	}
	if dec.blocks[2] == nil || dec.blocks[3] == nil {
		t.Fatalf("heavy blocks after pressure: %v", dec.blocks)
	}

	// Final parity may arrive after shedding. It supplies validation metadata but
	// must not recreate heavy reconstruction state.
	packets, done, err = dec.Add(one[DataShards])
	if err != nil {
		t.Fatalf("late parity: %v", err)
	}
	if done || len(packets) != 0 || dec.InFlight() != 2 {
		t.Fatalf("late parity packets=%d done=%t in_flight=%d", len(packets), done, dec.InFlight())
	}

	// Lower-layer ARQ can now finish the shed block from systematic retransmits.
	// Each source is still delivered exactly once.
	for i := 1; i < DataShards; i++ {
		packets, done, err = dec.Add(one[i])
		if err != nil {
			t.Fatalf("late source %d: %v", i, err)
		}
		if len(packets) != 1 || !bytes.Equal(packets[0], testPackets(DataShards)[i]) {
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
	if _, ok := dec.degraded[1]; ok {
		t.Fatal("completed degraded block 1 retained")
	}
	if !dec.completed.contains(1) {
		t.Fatal("completed degraded block 1 missing completion history")
	}
	late, lateDone, err := dec.Add(one[DataShards+1])
	if err != nil || lateDone || len(late) != 0 {
		t.Fatalf("late completed parity packets=%d done=%t err=%v", len(late), lateDone, err)
	}
}

func TestBlockDecoderStreamingPressureDoesNotEvictUnsafeReferenceState(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Reference final-header source shards are not delivered until reconstruction,
	// so these two heavy blocks are intentionally unsafe to shed.
	for id := uint32(10); id <= 11; id++ {
		wire := testBlockWire(t, codec, id)
		if _, _, err := dec.Add(wire[0]); err != nil {
			t.Fatal(err)
		}
	}
	if canDegradeBlock(dec.blocks[10]) || canDegradeBlock(dec.blocks[11]) {
		t.Fatal("reference blocks unexpectedly degradable")
	}

	streaming := fastTestBlockWire(t, codec, 12)
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
	if dec.degraded[12] == nil {
		t.Fatal("new streaming block did not enter degraded fallback")
	}
}
