package linkdata

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

// This is a deterministic counterexample, NOT a simulated netem probability
// or a product performance claim. Compare the same five LINK datagrams and
// exactly 20% targeted erasures under 8ms and 100ms partial-parity closure.
//
// At 12ms inter-arrival, 8ms closes each size class before the next datagram:
// erasing the first datagram's sources plus as many parity shards for its
// respective blocks makes the first datagram unrecoverable. A 100ms closure
// aggregates later sources in each group and may retain enough equations to
// recover that identical first datagram. This proves a FEC granularity tradeoff,
// not that the original 5205 Action used this exact erasure pattern.
func TestE1PartialParityDeadlineDeterministic20PercentErasureTradeoff(t *testing.T) {
	const sourceMTU = 1200
	const packetCount = 5
	for _, size := range []int{96, 1372, 4068} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			type verdict struct {
				deliveredFirst bool
				totalWire      int
				lostWire       int
				sourceFragments int
				blockIDs       int
			}
			run := func(flushAfter time.Duration) verdict {
				t.Helper()
				tx, err := NewFECPath(FECPathConfig{
					SourceMTU: sourceMTU, ParityShards: 20,
					FlushAfter: flushAfter, MaxBlocks: 32,
				})
				if err != nil { t.Fatal(err) }
				rx, err := NewFECPath(FECPathConfig{
					SourceMTU: sourceMTU, ParityShards: 20,
					FlushAfter: flushAfter, MaxBlocks: 32,
				})
				if err != nil { t.Fatal(err) }
				t0 := time.Unix(20261009, 0)
				var wire [][]byte
				payloads := make([][]byte, packetCount)
				firstSources := make(map[uint32]map[uint8]bool)
				for i := 0; i < packetCount; i++ {
					now := t0.Add(time.Duration(i) * 12 * time.Millisecond)
					// Model the previous pending FEC timer actually firing
					// before the next source arrives, only when it is due.
					due, err := tx.FlushDue(now.Add(-time.Nanosecond))
					if err != nil { t.Fatal(err) }
					wire = append(wire, due...)
					payloads[i] = bytes.Repeat([]byte{byte(i + 1)}, size)
					enc, err := tx.Encode(payloads[i], now)
					if err != nil { t.Fatal(err) }
					if i == 0 {
						for _, w := range enc {
							h, err := fec.ParseBlockHeader(w)
							if err != nil { t.Fatal(err) }
							if int(h.ShardIndex) >= fec.DataShards {
								t.Fatal("unexpected parity on first source")
							}
							if firstSources[h.BlockID] == nil {
								firstSources[h.BlockID] = make(map[uint8]bool)
							}
							firstSources[h.BlockID][h.ShardIndex] = true
						}
					}
					wire = append(wire, enc...)
				}
				tail, err := tx.FlushDue(t0.Add(200 * time.Millisecond))
				if err != nil { t.Fatal(err) }
				wire = append(wire, tail...)

				// For each FEC block containing fragments of datagram zero,
				// drop those first systematic fragments AND the same number
				// of that block's parity fragments. Thus exactly 20% of all
				// 5-datagram, 100%-redundancy wire is erased in both schedules.
				dropParity := make(map[uint32]int, len(firstSources))
				for id, shards := range firstSources { dropParity[id] = len(shards) }
				var lost int
				delivered := make([]int, packetCount)
				now := t0.Add(210 * time.Millisecond)
				for _, w := range wire {
					h, err := fec.ParseBlockHeader(w)
					if err != nil { t.Fatal(err) }
					drop := false
					if shards := firstSources[h.BlockID]; shards != nil {
						if int(h.ShardIndex) < fec.DataShards {
							drop = shards[h.ShardIndex]
						} else if dropParity[h.BlockID] != 0 {
							dropParity[h.BlockID]--
							drop = true
						}
					}
					if drop { lost++; continue }
					out, err := rx.Decode(w, now)
					if err != nil { t.Fatalf("decode source_bytes=%d flush=%s header=%+v: %v",size,flushAfter,h,err) }
					for _, got := range out {
						if len(got) != size { t.Fatalf("unexpected LINK frame size=%d want=%d",len(got),size) }
						idx := int(got[0]) - 1
						if idx < 0 || idx >= packetCount || !bytes.Equal(got,payloads[idx]) {
							t.Fatalf("corrupt/reassembled unexpected payload (idx=%d)",idx)
						}
						delivered[idx]++
						if delivered[idx] != 1 { t.Fatalf("duplicate initial delivery idx=%d",idx) }
					}
				}
				for id, remain := range dropParity {
					if remain != 0 { t.Fatalf("did not find expected first-block parity for block %d: %d",id,remain) }
				}
				for i:=1;i<packetCount;i++ {
					if delivered[i]!=1 { t.Fatalf("independent later datagram idx=%d delivered=%d",i,delivered[i]) }
				}
				sourceFragments := 0
				for _, shards := range firstSources { sourceFragments += len(shards) }
				if len(wire)!=10*sourceFragments || lost!=2*sourceFragments {
					t.Fatalf("must be exactly 20%% erasure: total=%d lost=%d first_fragments=%d",len(wire),lost,sourceFragments)
				}
				t.Logf("E1_FEC_TIMING_COUNTEREXAMPLE LINK_bytes=%d flush_ms=%d total_wire=%d erased=%d first_fragments=%d first_delivered=%v later_delivered=4",
					size,flushAfter/time.Millisecond,len(wire),lost,sourceFragments,delivered[0]==1)
				return verdict{deliveredFirst:delivered[0]==1,totalWire:len(wire),
					lostWire:lost,sourceFragments:sourceFragments,blockIDs:len(firstSources)}
			}
			short := run(8*time.Millisecond)
			long := run(100*time.Millisecond)
			if short.totalWire != long.totalWire || short.lostWire != long.lostWire {
				t.Fatalf("different impairment budget: short=%+v long=%+v",short,long)
			}
			if short.deliveredFirst || !long.deliveredFirst {
				t.Fatalf("expected short partial block unrecoverable and grouped long block recoverable: short=%+v long=%+v",short,long)
			}
		})
	}
}
