package fec

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestFixedParityProfileMatrixRecoversEquivalentSystematicLoss(t *testing.T) {
	for _, parity := range []int{4, 8, 10, 12, 16, 20} {
		t.Run(fmt.Sprintf("20_%d", parity), func(t *testing.T) {
			codec := NewFastReedSolomon20x20()
			enc, err := NewFastBlockEncoderWithParity(codec, 1400, 8*time.Millisecond, 77, parity)
			if err != nil {
				t.Fatal(err)
			}
			dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 4, parity)
			if err != nil {
				t.Fatal(err)
			}

			want := make([][]byte, DataShards)
			var wire [][]byte
			now := time.Unix(1, 0)
			for i := 0; i < DataShards; i++ {
				want[i] = []byte{byte(i), byte(255 - i), byte(i ^ 0x5a), byte(i + 17)}
				out, err := enc.Add(want[i], now)
				if err != nil {
					t.Fatal(err)
				}
				for _, datagram := range out {
					wire = append(wire, append([]byte(nil), datagram...))
				}
			}
			if got, wantN := len(wire), DataShards+parity; got != wantN {
				t.Fatalf("wire shards=%d want=%d", got, wantN)
			}
			for _, datagram := range wire {
				h, err := ParseBlockHeader(datagram[:HeaderSize])
				if err != nil {
					t.Fatal(err)
				}
				if h.EffectiveParityCount() != parity {
					t.Fatalf("wire parity=%d want=%d", h.EffectiveParityCount(), parity)
				}
			}

			got := make(map[byte][]byte, DataShards)
			// Drop exactly R systematic shards. The remaining 20-R sources plus
			// R repair rows must reconstruct all R missing originals.
			for _, datagram := range wire {
				h, err := ParseBlockHeader(datagram[:HeaderSize])
				if err != nil {
					t.Fatal(err)
				}
				if int(h.ShardIndex) < parity {
					continue
				}
				packets, _, err := dec.Add(datagram)
				if err != nil {
					t.Fatal(err)
				}
				for _, packet := range packets {
					got[packet[0]] = packet
				}
			}
			if len(got) != DataShards {
				t.Fatalf("recovered packets=%d want=%d", len(got), DataShards)
			}
			for i, packet := range want {
				if !bytes.Equal(got[byte(i)], packet) {
					t.Fatalf("packet %d mismatch: got=%v want=%v", i, got[byte(i)], packet)
				}
			}
		})
	}
}

func TestRejectsNonProductParityGeometry(t *testing.T) {
	for _, parity := range []int{1, 5, 6, 9, 11, 13, 19} {
		if _, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, time.Millisecond, 1, parity); err == nil {
			t.Fatalf("parity=%d unexpectedly accepted", parity)
		}
	}
}
