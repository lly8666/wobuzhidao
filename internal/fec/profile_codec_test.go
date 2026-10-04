package fec

import (
	"bytes"
	"errors"
	"testing"
)

func TestReedSolomon20xRRecoversThroughParityBudget(t *testing.T) {
	for _, parity := range []int{4, 8, 12, 16, 20} {
		t.Run(profileName(parity), func(t *testing.T) {
			codec, err := NewReedSolomon20xR(parity)
			if err != nil {
				t.Fatal(err)
			}
			total := DataShards + parity
			shards := make([][]byte, total)
			want := make([][]byte, DataShards)
			for i := 0; i < total; i++ {
				shards[i] = make([]byte, 257)
			}
			for d := 0; d < DataShards; d++ {
				for j := range shards[d] {
					shards[d][j] = byte((d*37 + j*13 + 11) & 0xff)
				}
				want[d] = append([]byte(nil), shards[d]...)
			}
			if err := codec.Encode(shards); err != nil {
				t.Fatal(err)
			}
			present := make([]bool, total)
			for i := range present {
				present[i] = true
			}
			// Drop exactly the parity budget while always losing systematic data.
			// The deterministic pattern also removes parity when R > 4.
			for m := 0; m < parity; m++ {
				idx := m
				if m >= parity/2 {
					idx = DataShards + (m - parity/2)
				}
				if idx >= total || !present[idx] {
					// Fill any remaining removals from the highest systematic slots.
					idx = DataShards - 1 - m
				}
				present[idx] = false
				clear(shards[idx])
			}
			if err := codec.Reconstruct(shards, present); err != nil {
				t.Fatalf("R=%d reconstruct: %v", parity, err)
			}
			for d := 0; d < DataShards; d++ {
				if !bytes.Equal(shards[d], want[d]) {
					t.Fatalf("R=%d data shard %d mismatch", parity, d)
				}
			}
		})
	}
}

func TestReedSolomon20xRRejectsBeyondBudget(t *testing.T) {
	for _, parity := range []int{4, 8, 12, 16, 20} {
		codec, err := NewReedSolomon20xR(parity)
		if err != nil {
			t.Fatal(err)
		}
		total := DataShards + parity
		shards := make([][]byte, total)
		present := make([]bool, total)
		for i := range shards {
			shards[i] = make([]byte, 64)
			present[i] = true
		}
		for i := 0; i < parity+1; i++ {
			present[i] = false
		}
		if err := codec.Reconstruct(shards, present); !errors.Is(err, ErrTooManyMissing) {
			t.Fatalf("R=%d err=%v", parity, err)
		}
	}
}

func TestReedSolomon20x20ProfileMatchesReferenceParity(t *testing.T) {
	profile, err := NewReedSolomon20xR(20)
	if err != nil {
		t.Fatal(err)
	}
	ref := NewReedSolomon20x20()
	a := make([][]byte, TotalShards)
	b := make([][]byte, TotalShards)
	for i := 0; i < TotalShards; i++ {
		a[i] = make([]byte, 113)
		b[i] = make([]byte, 113)
	}
	for d := 0; d < DataShards; d++ {
		for j := range a[d] {
			a[d][j] = byte(d + j*3)
			b[d][j] = a[d][j]
		}
	}
	if err := profile.Encode(a); err != nil {
		t.Fatal(err)
	}
	if err := ref.Encode(b); err != nil {
		t.Fatal(err)
	}
	for i := DataShards; i < TotalShards; i++ {
		if !bytes.Equal(a[i], b[i]) {
			t.Fatalf("parity shard %d differs from qualified 20:20 generator", i)
		}
	}
}

func profileName(r int) string {
	return "R" + string(rune('A'+r))
}
