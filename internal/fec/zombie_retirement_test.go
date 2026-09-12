package fec

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func makeSinglePacketFastBlock(t *testing.T, blockID uint32, payload []byte) (source, parity []byte) {
	t.Helper()
	enc, err := NewFastBlockEncoder(NewFastReedSolomon20x20(), 1400, 8*time.Millisecond, blockID)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(100, 0)
	out, err := enc.Add(payload, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("block %d source datagrams=%d want=1", blockID, len(out))
	}
	source = append([]byte(nil), out[0]...)
	out, err = enc.FlushDue(t0.Add(8 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("block %d parity datagrams=%d want=1", blockID, len(out))
	}
	parity = append([]byte(nil), out[0]...)
	return source, parity
}

func TestBlockDecoderKeepsProvisionalOldBlockUntilFinalMetadata(t *testing.T) {
	dec, err := NewBlockDecoder(NewFastReedSolomon20x20(), 1400, 2)
	if err != nil {
		t.Fatal(err)
	}

	s1, p1 := makeSinglePacketFastBlock(t, 1, []byte("one"))
	s2, _ := makeSinglePacketFastBlock(t, 2, []byte("two"))
	s3, p3 := makeSinglePacketFastBlock(t, 3, []byte("three"))

	for i, tc := range []struct {
		wire []byte
		want []byte
	}{{s1, []byte("one")}, {s2, []byte("two")}, {s3, []byte("three")}} {
		packets, _, err := dec.Add(tc.wire)
		if err != nil {
			t.Fatalf("source %d: %v", i+1, err)
		}
		if len(packets) != 1 || !bytes.Equal(packets[0], tc.want) {
			t.Fatalf("source %d delivered=%q want=%q", i+1, packets, tc.want)
		}
	}

	if dec.InFlight() != 2 {
		t.Fatalf("inflight=%d want=2", dec.InFlight())
	}
	// Before final metadata, block 1 could still be a larger generation with
	// missing source payloads that later parity can reconstruct. It therefore
	// must remain heavy; the newer block falls back to compact first-delivery
	// state instead of evicting the older recoverable generation.
	if dec.blocks[1] == nil || dec.blocks[2] == nil {
		t.Fatal("provisional recoverable block was retired under pressure")
	}
	if _, ok := dec.retired[3]; !ok {
		t.Fatal("new provisional block did not enter compact fallback state")
	}

	// Final metadata proves block 1 really contained only the already-delivered
	// source. It can now complete and release its heavy slot without duplicate
	// delivery.
	packets, done, err := dec.Add(p1)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 0 || !done {
		t.Fatalf("late parity packets=%d done=%v want=0,true", len(packets), done)
	}
	if dec.blocks[1] != nil || !dec.completed.contains(1) {
		t.Fatal("finalized one-source block did not release heavy state")
	}

	packets, done, err = dec.Add(s1)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 0 || done {
		t.Fatalf("late duplicate source packets=%d done=%v want=0,false", len(packets), done)
	}

	// The compact fallback for block 3 also closes cleanly once its final parity
	// reveals that its sole source was already delivered.
	packets, done, err = dec.Add(p3)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 0 || !done {
		t.Fatalf("compact late parity packets=%d done=%v want=0,true", len(packets), done)
	}
	if _, ok := dec.retired[3]; ok {
		t.Fatal("completed compact fallback state was not released")
	}
	if !dec.completed.contains(3) {
		t.Fatal("completed compact fallback missing completion history")
	}
}

func TestBlockDecoderDoesNotRetireFinalReconstructionState(t *testing.T) {
	codec := NewReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil {
		t.Fatal(err)
	}

	for block := uint32(1); block <= 3; block++ {
		enc, err := NewBlockEncoder(codec, 1400, time.Millisecond, block)
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
		_, _, err = dec.Add(wire[0])
		if block < 3 && err != nil {
			t.Fatalf("block %d: %v", block, err)
		}
		if block == 3 && !errors.Is(err, ErrDecoderFull) {
			t.Fatalf("block %d got %v want ErrDecoderFull", block, err)
		}
	}
	if len(dec.retired) != 0 {
		t.Fatalf("final reconstruction state retired=%d want=0", len(dec.retired))
	}
}
