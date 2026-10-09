package fec

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// This is a wire-compatibility guard for the E2 hot-path optimization:
// the concrete fast codec only multiplies source slots below dataCount.
// Unused slots may contain dirty bytes without changing parity; generic
// codecs still need canonical zero-filled known-absent source slots.
func TestE2FastPartialParityNeverReadsInactiveShards(t *testing.T) {
	for _, parity := range SupportedParityShards() {
		for _, count := range []int{1, 2, 5, 19} {
			for _, size := range []int{1, 256, 1200} {
				t.Run(fmt.Sprintf("20:%d/n%d/s%d", parity, count, size), func(t *testing.T) {
					fast, err := NewFastBlockEncoderWithParity(
						NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, 1, parity)
					if err != nil { t.Fatal(err) }
					generic, err := NewFastBlockEncoderWithParity(
						NewReedSolomon20x20(), 1200, 8*time.Millisecond, 1, parity)
					if err != nil { t.Fatal(err) }
					now := time.Unix(20261009, 0)
					for i := 0; i < count; i++ {
						payload := bytes.Repeat([]byte{byte(11+i*7)}, size)
						x, err := fast.Add(payload, now)
						if err != nil { t.Fatal(err) }
						y, err := generic.Add(payload, now)
						if err != nil { t.Fatal(err) }
						compareActiveWire(t, x, y)
					}
					// Poison every inactive source shard to catch *either*
					// accidental inactive reads or a regression to clearing
					// all the padding on the fast path.
					for d := count; d < DataShards; d++ {
						for i := 0; i < size; i++ {
							fast.shardBuf[d][i] = 0xa5
							generic.shardBuf[d][i] = 0xa5
						}
					}
					fastParity, err := fast.Flush()
					if err != nil { t.Fatal(err) }
					refParity, err := generic.Flush()
					if err != nil { t.Fatal(err) }
					compareActiveWire(t, fastParity, refParity)
					for d := count; d < DataShards; d++ {
						if !bytes.Equal(fast.shardBuf[d][:size], bytes.Repeat([]byte{0xa5}, size)) {
							t.Fatalf("fast path needlessly cleared inactive source d=%d", d)
						}
						if !bytes.Equal(generic.shardBuf[d][:size], make([]byte, size)) {
							t.Fatalf("generic path failed to zero known-inactive source d=%d", d)
						}
					}
					if fast.Pending() != 0 || generic.Pending() != 0 {
						t.Fatal("partial group not retired")
					}
				})
			}
		}
	}
}
