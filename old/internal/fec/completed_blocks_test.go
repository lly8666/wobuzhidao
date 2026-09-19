package fec

import (
	"testing"
	"time"
)

func testBlockWire(t *testing.T, codec Codec, blockID uint32) [][]byte {
	t.Helper()
	enc, err := NewBlockEncoder(codec, 1400, time.Millisecond, blockID)
	if err != nil {
		t.Fatal(err)
	}
	var wire [][]byte
	for _, packet := range testPackets(DataShards) {
		wire, err = enc.Add(packet, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(wire) != TotalShards {
		t.Fatalf("block %d wire=%d want=%d", blockID, len(wire), TotalShards)
	}
	return wire
}

func completeTestBlock(t *testing.T, dec *BlockDecoder, wire [][]byte) {
	t.Helper()
	done := false
	for i := 0; i < DataShards; i++ {
		_, blockDone, err := dec.Add(wire[i])
		if err != nil {
			t.Fatal(err)
		}
		done = done || blockDone
	}
	if !done {
		t.Fatal("block did not complete")
	}
	if dec.InFlight() != 0 {
		t.Fatalf("in_flight=%d want=0 after completion", dec.InFlight())
	}
}

func TestBlockDecoderLateCompletedShardDoesNotReopen(t *testing.T) {
	codec := NewReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 1)
	if err != nil {
		t.Fatal(err)
	}

	// maxBlocks=1 used to retain only four completed BlockIDs. Completing eight
	// blocks therefore evicted block 1 from the old FIFO tombstone set.
	var first [][]byte
	for id := uint32(1); id <= 8; id++ {
		wire := testBlockWire(t, codec, id)
		if id == 1 {
			first = wire
		}
		completeTestBlock(t, dec, wire)
	}

	// A very late retransmission from block 1 must remain a duplicate forever
	// within this immutable LINK association. Reopening it would consume the only
	// decoder slot and make the next real block fail with ErrDecoderFull.
	packets, done, err := dec.Add(first[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 0 || done {
		t.Fatalf("late completed shard packets=%d done=%t", len(packets), done)
	}
	if dec.InFlight() != 0 {
		t.Fatalf("late completed shard reopened block; in_flight=%d", dec.InFlight())
	}

	next := testBlockWire(t, codec, 9)
	if _, _, err := dec.Add(next[0]); err != nil {
		t.Fatalf("new block rejected after late duplicate: %v", err)
	}
	if dec.InFlight() != 1 {
		t.Fatalf("in_flight=%d want=1 for new block", dec.InFlight())
	}
}

func TestCompletedBlockSetCompactsContiguousHistory(t *testing.T) {
	var s completedBlockSet

	// Exercise out-of-order completion and range merging first.
	s.add(2)
	s.add(4)
	s.add(3)
	if s.through != 0 || len(s.ranges) != 1 || s.ranges[0] != (completedBlockRange{first: 2, last: 4}) {
		t.Fatalf("before gap close through=%d ranges=%v", s.through, s.ranges)
	}
	s.add(1)
	if s.through != 4 || len(s.ranges) != 0 {
		t.Fatalf("after gap close through=%d ranges=%v", s.through, s.ranges)
	}

	// Long normal runs stay constant-memory instead of keeping one tombstone per
	// FEC block. This matters when lane rotation is disabled or delayed.
	for id := uint32(5); id <= 100000; id++ {
		s.add(id)
	}
	if s.through != 100000 || len(s.ranges) != 0 {
		t.Fatalf("sequential history through=%d ranges=%d", s.through, len(s.ranges))
	}
	for _, id := range []uint32{1, 2, 99999, 100000} {
		if !s.contains(id) {
			t.Fatalf("completed id %d forgotten", id)
		}
	}
	if s.contains(100001) {
		t.Fatal("future block reported completed")
	}
}
