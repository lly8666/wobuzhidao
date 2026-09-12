package fec

import (
	"bytes"
	"testing"
	"time"
)

// The fast encoder reuses final wire payload storage as its RS shard backing.
// Exercise two consecutive blocks with opposite packet-size profiles so stale
// bytes from the first block cannot contaminate parity in the second.
func TestFastBlockEncoderReusedWirePaddingDoesNotLeakAcrossBlocks(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	enc, err := NewFastBlockEncoder(codec, 1400, time.Second, 41)
	if err != nil {
		t.Fatal(err)
	}

	// Prime every systematic wire slot with a long, non-zero payload.
	for i := 0; i < DataShards; i++ {
		p := bytes.Repeat([]byte{byte(0x80 + i)}, 1300-i)
		out, err := enc.Add(p, time.Unix(1, int64(i)))
		if err != nil {
			t.Fatal(err)
		}
		if i == DataShards-1 && len(out) != 1+ParityShards {
			t.Fatalf("first block final output=%d want=%d", len(out), 1+ParityShards)
		}
	}

	// Reuse the same slots with short, varied payloads. Drop all systematic
	// datagrams and reconstruct only from parity; any uncleared old tail would
	// change the RS equations and corrupt recovered source bytes.
	want := make([][]byte, DataShards)
	var parity [][]byte
	for i := 0; i < DataShards; i++ {
		want[i] = bytes.Repeat([]byte{byte(i + 1)}, 3+i*7)
		out, err := enc.Add(want[i], time.Unix(2, int64(i)))
		if err != nil {
			t.Fatal(err)
		}
		if i == DataShards-1 {
			parity = append(parity, out[1:]...)
		}
	}
	if len(parity) != ParityShards {
		t.Fatalf("parity=%d want=%d", len(parity), ParityShards)
	}

	dec, err := NewBlockDecoder(codec, 1400, 8)
	if err != nil {
		t.Fatal(err)
	}
	var got [][]byte
	for i, d := range parity {
		packets, _, err := dec.Add(d)
		if err != nil {
			t.Fatalf("parity %d: %v", i, err)
		}
		got = append(got, packets...)
	}
	if len(got) != len(want) {
		t.Fatalf("decoded=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d mismatch got=%x want=%x", i, got[i], want[i])
		}
	}
}
