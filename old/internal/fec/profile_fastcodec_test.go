package fec

import (
	"bytes"
	"testing"
)

func TestFastReedSolomon20xRMatchesReferenceAndRecovers(t *testing.T) {
	for _, parity := range []int{4, 8, 12, 16, 20} {
		fast, err := NewFastReedSolomon20xR(parity)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := NewReedSolomon20xR(parity)
		if err != nil {
			t.Fatal(err)
		}
		total := DataShards + parity
		a := make([][]byte, total)
		b := make([][]byte, total)
		want := make([][]byte, DataShards)
		for i := 0; i < total; i++ {
			a[i] = make([]byte, 1024)
			b[i] = make([]byte, 1024)
		}
		for d := 0; d < DataShards; d++ {
			for j := range a[d] {
				a[d][j] = byte((d*29 + j*7 + parity) & 0xff)
				b[d][j] = a[d][j]
			}
			want[d] = append([]byte(nil), a[d]...)
		}
		if err := fast.Encode(a); err != nil {
			t.Fatal(err)
		}
		if err := ref.Encode(b); err != nil {
			t.Fatal(err)
		}
		for i := DataShards; i < total; i++ {
			if !bytes.Equal(a[i], b[i]) {
				t.Fatalf("R=%d parity %d differs", parity, i)
			}
		}

		present := make([]bool, total)
		for i := range present {
			present[i] = true
		}
		for i := 0; i < parity; i++ {
			present[i] = false
			clear(a[i])
		}
		if err := fast.Reconstruct(a, present); err != nil {
			t.Fatalf("R=%d reconstruct: %v", parity, err)
		}
		for d := 0; d < DataShards; d++ {
			if !bytes.Equal(a[d], want[d]) {
				t.Fatalf("R=%d data %d mismatch", parity, d)
			}
		}
	}
}

func BenchmarkFastReedSolomon20xR(b *testing.B) {
	for _, parity := range []int{4, 8, 12, 16, 20} {
		b.Run(profileBenchName(parity), func(b *testing.B) {
			codec, err := NewFastReedSolomon20xR(parity)
			if err != nil {
				b.Fatal(err)
			}
			total := DataShards + parity
			shards := make([][]byte, total)
			for i := range shards {
				shards[i] = make([]byte, 1200)
			}
			for i := 0; i < DataShards; i++ {
				for j := range shards[i] {
					shards[i][j] = byte(i + j)
				}
			}
			b.SetBytes(int64(DataShards * 1200))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := codec.Encode(shards); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func profileBenchName(r int) string {
	return map[int]string{4:"20x4", 8:"20x8", 12:"20x12", 16:"20x16", 20:"20x20"}[r]
}
