package runtimeentry

import (
	"context"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

const gameIngressControlDepth = 256

// gameIngressShards divides the original 4096-record ingress budget.
// It never creates four more 4096 queues. Each whole four-tuple is assigned
// to one FIFO shard before the first SYN, throughout adoption and retirement;
// selecting a worker is NOT admission, decryption or packet deduplication.
type gameIngressShards struct {
	in []chan segmentRead
}

func newGameIngressShards(desired int) (*gameIngressShards, int) {
	if desired < 2 || desired > 4 {
		return nil, serverReadQueueDepth
	}
	perLane := (serverReadQueueDepth - gameIngressControlDepth) / desired
	lanes := make([]chan segmentRead, desired)
	for i := range lanes {
		lanes[i] = make(chan segmentRead, perLane)
	}
	return &gameIngressShards{in: lanes}, gameIngressControlDepth
}

// Index uses the stable client source port for one FakeTCP association.
// Consecutive source ports (the normal lane allocation) spread across 2–4
// shards, but correctness never relies on collision-free distribution.
// Routing is strictly a CPU scheduling decision, not a trust decision.
func (s *LifecycleServer) gameIngressIndex(seg faketcp.Segment, shards *gameIngressShards) int {
	if shards == nil || seg.DstPort != s.cfg.ListenPort {
		return -1
	}
	return int(seg.SrcPort) % len(shards.in)
}

// Each worker owns one FIFO for all FakeTCP packets assigned to the shard:
// handshake, authenticated records, FIN and late retirement traffic never
// overtake one another within the same source four-tuple.
func (s *LifecycleServer) runGameIngressWorkers(
	ctx context.Context, shards *gameIngressShards, fatal chan<- error, wg *sync.WaitGroup,
) {
	if shards == nil {
		return
	}
	for _, queue := range shards.in {
		queue := queue
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case read := <-queue:
					started := time.Now()
					if s.cfg.ObserveTiming {
						s.pipeline.readyDone(len(read.seg.Payload), started.Sub(read.readyAt))
					}
					err := s.handleSegment(ctx, read.seg, started)
					if s.cfg.ObserveTiming {
						s.pipeline.handler.observe(time.Since(started))
					}
					if err != nil {
						select { case fatal <- err: default: }
						return
					}
				}
			}
		}()
	}
}
