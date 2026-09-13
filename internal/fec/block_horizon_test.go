package fec

import (
	"bytes"
	"testing"
	"time"
)

func makeHorizonFastBlock(t *testing.T, blockID uint32) ([][]byte, [][]byte, [][]byte) {
	t.Helper()
	codec := NewReedSolomon20x20()
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
		for _, wire := range out[1:] {
			parity = append(parity, append([]byte(nil), wire...))
		}
	}
	if len(sources) != DataShards || len(parity) != ParityShards {
		t.Fatalf("block %d sources=%d parity=%d", blockID, len(sources), len(parity))
	}
	return want, sources, parity
}

func oneHorizonStreamingShard(t *testing.T, blockID uint32) []byte {
	t.Helper()
	enc, err := NewFastBlockEncoder(NewReedSolomon20x20(), 1400, time.Millisecond, blockID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := enc.Add([]byte{byte(blockID), 1, 2, 3}, time.Unix(0, int64(blockID)))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("block %d first add emitted %d frames want 1", blockID, len(out))
	}
	return append([]byte(nil), out[0]...)
}

func TestBlockDecoderRecoveryHorizonRetiresIncompleteHeavyAndKeepsLateSystematic(t *testing.T) {
	codec := NewReedSolomon20x20()
	want1, source1, parity1 := makeHorizonFastBlock(t, 1)
	dec, err := NewBlockDecoder(codec, 1400, 640)
	if err != nil {
		t.Fatal(err)
	}

	// Make block 1 authoritative/final but still unrecoverable: two sources are
	// missing and only one parity shard is present, leaving 19 reconstruction
	// inputs. This is the zombie shape that used to keep a heavy slot forever.
	for i := 0; i < DataShards-2; i++ {
		packets, done, err := dec.Add(source1[i])
		if err != nil {
			t.Fatalf("block1 source %d: %v", i, err)
		}
		if done || len(packets) != 1 || !bytes.Equal(packets[0], want1[i]) {
			t.Fatalf("block1 source %d done=%v delivered=%d", i, done, len(packets))
		}
	}
	if packets, done, err := dec.Add(parity1[0]); err != nil || done || len(packets) != 0 {
		t.Fatalf("block1 parity done=%v delivered=%d err=%v", done, len(packets), err)
	}
	if b := dec.blocks[1]; b == nil || !b.final {
		t.Fatal("block1 is not retained as an incomplete final heavy block")
	}

	// Advance through block 64. Block 1 is only 63 generations old and must
	// remain reconstructable inside the configured horizon.
	for blockID := uint32(2); blockID <= 64; blockID++ {
		if _, _, err := dec.Add(oneHorizonStreamingShard(t, blockID)); err != nil {
			t.Fatalf("advance block %d: %v", blockID, err)
		}
	}
	if dec.blocks[1] == nil {
		t.Fatal("block1 retired before the 64-block recovery horizon")
	}
	if st := dec.PressureStats(); st.HorizonRetireEvents != 0 || st.OldestActiveAgeBlocks != 63 {
		t.Fatalf("pre-horizon stats=%#v", st)
	}

	// Seeing block 65 makes block 1 exactly 64 generations old. It must shed
	// heavy shard payloads now, independent of maxBlocks pressure.
	if _, _, err := dec.Add(oneHorizonStreamingShard(t, 65)); err != nil {
		t.Fatalf("advance block 65: %v", err)
	}
	if dec.blocks[1] != nil {
		t.Fatal("expired block1 still occupies a heavy decoder slot")
	}
	if _, ok := dec.retired[1]; !ok {
		t.Fatal("expired block1 did not move to compact retired state")
	}
	st := dec.PressureStats()
	if st.RecoveryHorizonBlocks != 64 || st.HorizonRetireEvents != 1 || st.HorizonRetiredIncomplete != 1 {
		t.Fatalf("post-horizon stats=%#v", st)
	}
	if st.OldestActiveAgeBlocks > 63 {
		t.Fatalf("oldest heavy age=%d exceeds bounded horizon", st.OldestActiveAgeBlocks)
	}

	// Heavy parity recovery is intentionally over, but late systematic sources
	// must still make their first application delivery through compact state.
	for _, missing := range []int{DataShards - 2, DataShards - 1} {
		packets, done, err := dec.Add(source1[missing])
		if err != nil {
			t.Fatalf("late source %d: %v", missing, err)
		}
		if len(packets) != 1 || !bytes.Equal(packets[0], want1[missing]) {
			t.Fatalf("late source %d delivered=%d", missing, len(packets))
		}
		if missing == DataShards-2 && done {
			t.Fatal("block completed with one late source still missing")
		}
		if missing == DataShards-1 && !done {
			t.Fatal("block did not complete after both late systematic sources arrived")
		}
	}

	// Once compact delivery is complete, ancient parity/source duplicates must
	// be ignored rather than recreating the block or duplicating payload.
	if packets, done, err := dec.Add(parity1[1]); err != nil || done || len(packets) != 0 {
		t.Fatalf("late duplicate parity done=%v delivered=%d err=%v", done, len(packets), err)
	}
	if packets, done, err := dec.Add(source1[DataShards-1]); err != nil || done || len(packets) != 0 {
		t.Fatalf("late duplicate source done=%v delivered=%d err=%v", done, len(packets), err)
	}
}

func TestBlockDecoderLateFirstSeenBlockStartsCompactPastHorizon(t *testing.T) {
	dec, err := NewBlockDecoder(NewReedSolomon20x20(), 1400, 640)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := dec.Add(oneHorizonStreamingShard(t, 100)); err != nil {
		t.Fatal(err)
	}
	late := oneHorizonStreamingShard(t, 1)
	packets, done, err := dec.Add(late)
	if err != nil {
		t.Fatal(err)
	}
	if done || len(packets) != 1 {
		t.Fatalf("late first-seen block done=%v delivered=%d", done, len(packets))
	}
	if dec.blocks[1] != nil {
		t.Fatal("late first-seen block reopened heavy reconstruction state")
	}
	if _, ok := dec.retired[1]; !ok {
		t.Fatal("late first-seen block did not use compact state")
	}
}
