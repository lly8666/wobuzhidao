package fec

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestFastCodecMatchesReferenceParityAndRecovery(t *testing.T) {
	ref := NewReedSolomon20x20()
	fast := NewFastReedSolomon20x20()
	rng := rand.New(rand.NewSource(260825))
	base := make([][]byte, TotalShards)
	for i := range base {
		base[i] = make([]byte, 1200)
	}
	for i := 0; i < DataShards; i++ {
		if _, err := rng.Read(base[i]); err != nil {
			t.Fatal(err)
		}
	}
	want := cloneShards(base)
	got := cloneShards(base)
	if err := ref.Encode(want); err != nil {
		t.Fatal(err)
	}
	if err := fast.Encode(got); err != nil {
		t.Fatal(err)
	}
	for i := DataShards; i < TotalShards; i++ {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("parity shard %d mismatch", i)
		}
	}

	for missing := 0; missing <= ParityShards; missing++ {
		shards := cloneShards(want)
		present := make([]bool, TotalShards)
		for i := range present {
			present[i] = true
		}
		for n := 0; n < missing; n++ {
			idx := (n * 7) % TotalShards
			for !present[idx] {
				idx = (idx + 1) % TotalShards
			}
			present[idx] = false
			clear(shards[idx])
		}
		if err := fast.Reconstruct(shards, present); err != nil {
			t.Fatalf("missing=%d: %v", missing, err)
		}
		for i := 0; i < DataShards; i++ {
			if !bytes.Equal(shards[i], want[i]) {
				t.Fatalf("missing=%d data shard %d mismatch", missing, i)
			}
		}
	}
}

func BenchmarkFastEncode20x20_1200B(b *testing.B) {
	codec, shards, _ := makeEncodedShards(b)
	_ = codec
	fast := NewFastReedSolomon20x20()
	b.SetBytes(DataShards * testShardSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := fast.Encode(shards); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFastReconstruct20Missing_1200B(b *testing.B) {
	_, encoded, _ := makeEncodedShards(b)
	fast := NewFastReedSolomon20x20()
	b.SetBytes(DataShards * testShardSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shards := cloneShards(encoded)
		present := make([]bool, TotalShards)
		for j := range present {
			present[j] = true
		}
		for j := 0; j < DataShards; j++ {
			present[j] = false
			clear(shards[j])
		}
		if err := fast.Reconstruct(shards, present); err != nil {
			b.Fatal(err)
		}
	}
}
