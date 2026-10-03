package fec

import (
	"bytes"
	"testing"
	"time"
)

func TestActiveParityMatchesReferenceForEveryShortenedProfile(t *testing.T) {
	fast := NewFastReedSolomon20x20()
	ref := NewReedSolomon20x20()
	for _, parity := range SupportedParityShards() {
		for n := 1; n <= DataShards; n++ {
			for _, size := range []int{1, 63, 1400} {
				var a, b [TotalShards][]byte
				for i := range a {
					a[i] = make([]byte, size)
					b[i] = make([]byte, size)
					if i < n {
						for j := range a[i] {
							a[i][j] = byte(i*43 + j*17 + n)
						}
						copy(b[i], a[i])
					}
					if i >= DataShards {
						for j := range a[i] {
							a[i][j] = 0xa5
						}
					}
				}
				count := min(n, parity)
				if err := fast.EncodeActive(a[:], n, count); err != nil {
					t.Fatal(err)
				}
				if err := ref.Encode(b[:]); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < DataShards+count; i++ {
					if !bytes.Equal(a[i], b[i]) {
						t.Fatalf("20:%d n=%d size=%d shard=%d differs", parity, n, size, i)
					}
				}
				for i := DataShards + count; i < TotalShards; i++ {
					if !bytes.Equal(a[i], bytes.Repeat([]byte{0xa5}, size)) {
						t.Fatal("unused parity row modified")
					}
				}
			}
		}
	}
}

func TestActiveBlockEncoderWireMatchesGenericCodecAcrossReuse(t *testing.T) {
	for _, parity := range SupportedParityShards() {
		fast, _ := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, time.Millisecond, 1, parity)
		ref, _ := NewFastBlockEncoderWithParity(NewReedSolomon20x20(), 1400, time.Millisecond, 1, parity)
		for n := DataShards; n >= 1; n-- {
			for i := 0; i < n; i++ {
				p := bytes.Repeat([]byte{byte(n*7 + i)}, 1+(i*79+n*11)%1400)
				a, err := fast.Add(p, time.Unix(1, 0))
				if err != nil {
					t.Fatal(err)
				}
				b, err := ref.Add(p, time.Unix(1, 0))
				if err != nil {
					t.Fatal(err)
				}
				compareActiveWire(t, a, b)
			}
			a, err := fast.Flush()
			if err != nil {
				t.Fatal(err)
			}
			b, err := ref.Flush()
			if err != nil {
				t.Fatal(err)
			}
			compareActiveWire(t, a, b)
		}
	}
}

func compareActiveWire(t *testing.T, a, b [][]byte) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("wire count differs %d/%d", len(a), len(b))
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			t.Fatalf("wire shard %d differs", i)
		}
	}
}

func TestActiveParityRejectsInvalidDimensions(t *testing.T) {
	c := NewFastReedSolomon20x20()
	for _, dims := range [][2]int{{0, 1}, {21, 1}, {1, 0}, {1, 21}} {
		if err := c.EncodeActive(nil, dims[0], dims[1]); err != ErrInvalidShardSet {
			t.Fatalf("dimensions=%v err=%v", dims, err)
		}
	}
}
