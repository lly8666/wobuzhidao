package runtimeentry

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestGameIngressShardsOneGlobalBudgetNormalUnchanged(t *testing.T) {
	for _, desired := range []int{1, 2, 3, 4} {
		shards, control := newGameIngressShards(desired)
		total := control
		if desired == 1 {
			if shards != nil || control != serverReadQueueDepth {
				t.Fatalf("Normal changed: shards=%v control=%d", shards, control)
			}
			continue
		}
		if shards == nil || len(shards.in) != desired || control != gameIngressControlDepth {
			t.Fatalf("Game %d shards=%v control=%d", desired, shards, control)
		}
		for _, q := range shards.in {
			total += cap(q)
			if cap(q) == 0 {
				t.Fatal("unbuffered Game shard")
			}
		}
		if total != serverReadQueueDepth {
			t.Fatalf("%d lanes changed total 4096 ingress capacity: got %d", desired, total)
		}
	}
}

func TestGameIngressStableFourTupleAcrossHandshakeGenerationAndFIN(t *testing.T) {
	s := &LifecycleServer{cfg: LifecycleServerConfig{ListenPort: 443}}
	for _, n := range []int{2, 3, 4} {
		shards, _ := newGameIngressShards(n)
		mapping := make(map[int]bool)
		for lane := 0; lane < n; lane++ {
			seg := faketcp.Segment{
				SrcIP: [4]byte{10, 0, 0, byte(1+lane)},
				DstIP: [4]byte{10, 0, 1, 1},
				SrcPort: uint16(32000+lane), DstPort: 443,
			}
			index := s.gameIngressIndex(seg, shards)
			if index < 0 || index >= n || mapping[index] {
				t.Fatalf("%d lane worker hash not distributed for sequential ports: %d", n, index)
			}
			mapping[index] = true
			// SYN, established packet, FIN and replacement handoff are
			// scheduled by the same immutable flow tuple, before auth.
			for i := 0; i < 100; i++ {
				seg.Flags = uint8(i)
				seg.Seq = uint32(i)
				if got := s.gameIngressIndex(seg, shards); got != index {
					t.Fatalf("four-tuple shifted shards %d -> %d", index, got)
				}
			}
			seg.DstPort = 444
			if got := s.gameIngressIndex(seg, shards); got != -1 {
				t.Fatalf("off-port tuple entered Game worker %d", got)
			}
		}
	}
	shards, _ := newGameIngressShards(1)
	if s.gameIngressIndex(faketcp.Segment{DstPort: 443}, shards) != -1 {
		t.Fatal("Normal must stay single-threaded")
	}
}

func TestGameIngressSlowLaneCannotConsumeOtherLaneBudget(t *testing.T) {
	shards, _ := newGameIngressShards(4)
	first, second := shards.in[0], shards.in[1]
	for i := 0; i < cap(first); i++ {
		if _, _, accepted := offerLatestBounded(first, segmentRead{bytes: i}); !accepted {
			t.Fatal("could not fill lane 1 budget")
		}
	}
	if len(second) != 0 {
		t.Fatal("slow lane 1 consumed lane 2 capacity")
	}
	_, old, ok := offerLatestBounded(first, segmentRead{bytes: 99999})
	if !ok || !old || len(first) != cap(first) {
		t.Fatalf("full lane must shed exactly one oldest item; accepted=%t dropped=%t len=%d", ok, old, len(first))
	}
	if got := (<-first).bytes; got != 1 {
		t.Fatalf("lane FIFO after oldest shedding got=%d, want 1", got)
	}
	if _, old, ok = offerLatestBounded(second, segmentRead{bytes: 234}); !ok || old {
		t.Fatal("healthy lane blocked because another lane was full")
	}
	if got := (<-second).bytes; got != 234 {
		t.Fatalf("healthy lane got wrong packet %d", got)
	}
}
