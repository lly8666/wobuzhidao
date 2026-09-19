package fec

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
)

const testShardSize = 1200

func makeEncodedShards(t testing.TB) (Codec, [][]byte, [][]byte) {
	t.Helper()
	codec := NewReedSolomon20x20()
	shards := make([][]byte, TotalShards)
	original := make([][]byte, DataShards)
	rng := rand.New(rand.NewSource(8666))
	for i := range shards {
		shards[i] = make([]byte, testShardSize)
	}
	for i := 0; i < DataShards; i++ {
		if _, err := rng.Read(shards[i]); err != nil {
			t.Fatal(err)
		}
		original[i] = append([]byte(nil), shards[i]...)
	}
	if err := codec.Encode(shards); err != nil {
		t.Fatal(err)
	}
	return codec, shards, original
}

func cloneShards(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i := range in {
		out[i] = append([]byte(nil), in[i]...)
	}
	return out
}

func TestReconstructZeroThroughTwentyMissing(t *testing.T) {
	codec, encoded, original := makeEncodedShards(t)
	for missing := 0; missing <= ParityShards; missing++ {
		t.Run("data-prefix-"+itoa(missing), func(t *testing.T) {
			shards := cloneShards(encoded)
			present := make([]bool, TotalShards)
			for i := range present {
				present[i] = true
			}
			for i := 0; i < missing; i++ {
				present[i] = false
				clear(shards[i])
			}
			if err := codec.Reconstruct(shards, present); err != nil {
				t.Fatalf("missing=%d: %v", missing, err)
			}
			for i := 0; i < DataShards; i++ {
				if !bytes.Equal(shards[i], original[i]) {
					t.Fatalf("missing=%d data shard %d mismatch", missing, i)
				}
			}
		})

		t.Run("mixed-"+itoa(missing), func(t *testing.T) {
			shards := cloneShards(encoded)
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
			if err := codec.Reconstruct(shards, present); err != nil {
				t.Fatalf("missing=%d: %v", missing, err)
			}
			for i := 0; i < DataShards; i++ {
				if !bytes.Equal(shards[i], original[i]) {
					t.Fatalf("missing=%d data shard %d mismatch", missing, i)
				}
			}
		})
	}
}

func TestReconstructRejectsBeyondParityBudget(t *testing.T) {
	codec, shards, _ := makeEncodedShards(t)
	present := make([]bool, TotalShards)
	for i := range present {
		present[i] = true
	}
	for i := 0; i < ParityShards+1; i++ {
		present[i] = false
		clear(shards[i])
	}
	if err := codec.Reconstruct(shards, present); !errors.Is(err, ErrTooManyMissing) {
		t.Fatalf("got %v, want ErrTooManyMissing", err)
	}
}

func TestBlockHeaderRoundTripVariableLengths(t *testing.T) {
	var h BlockHeader
	h.BlockID = 0x10203040
	h.ShardIndex = 37
	h.DataCount = DataShards
	h.ShardSize = 1400
	for i := range h.OriginalLengths {
		h.OriginalLengths[i] = uint16(64 + i*53)
	}
	b, err := h.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != HeaderSize {
		t.Fatalf("header size=%d want=%d", len(b), HeaderSize)
	}
	got, err := ParseBlockHeader(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != h {
		t.Fatalf("round trip mismatch: got=%+v want=%+v", got, h)
	}
}

func TestBufferPoolIsBounded(t *testing.T) {
	pool, err := NewBufferPool(1400, 2)
	if err != nil {
		t.Fatal(err)
	}
	a, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	b, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Get(); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
	if err := pool.Put(a); err != nil {
		t.Fatal(err)
	}
	if err := pool.Put(b); err != nil {
		t.Fatal(err)
	}
	if pool.Available() != 2 {
		t.Fatalf("available=%d want=2", pool.Available())
	}
}

func BenchmarkEncode20x20_1200B(b *testing.B) {
	codec, shards, _ := makeEncodedShards(b)
	b.SetBytes(DataShards * testShardSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := codec.Encode(shards); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReconstruct20Missing_1200B(b *testing.B) {
	codec, encoded, _ := makeEncodedShards(b)
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
		if err := codec.Reconstruct(shards, present); err != nil {
			b.Fatal(err)
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
