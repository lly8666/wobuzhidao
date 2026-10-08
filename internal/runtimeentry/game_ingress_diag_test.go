package runtimeentry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGameIngressDiagnosticOneFixedBudgetFourShards(t *testing.T) {
	var p serverPipelineTiming
	if got := p.snapshot(true).GameIngressShards; len(got) != 0 {
		t.Fatalf("normal mode must not publish Game shard metrics: %+v", got)
	}
	p.gameIngressDesired.Store(4)
	p.gameIngressEnqueue(2, 800)
	p.gameIngressEnqueue(2, 960)
	p.gameIngressDrop(2)
	p.gameIngressReject(2)
	p.gameIngressHandled(2, 12*time.Millisecond, 3*time.Millisecond)
	p.gameIngressEnqueue(0, 4)
	p.gameIngressHandled(0, 50*time.Microsecond, 40*time.Microsecond)
	got := p.snapshot(true).GameIngressShards
	if len(got) != 4 {
		t.Fatalf("Game4 shard count=%d", len(got))
	}
	total := gameIngressControlDepth
	for i, shard := range got {
		if shard.Index != i || shard.Capacity != 960 {
			t.Fatalf("shard %d index/capacity incorrect: %+v", i, shard)
		}
		total += shard.Capacity
	}
	if total != serverReadQueueDepth {
		t.Fatalf("sharded diagnostics invented capacity: %d", total)
	}
	busy := got[2]
	if busy.Enqueued != 2 || busy.Handled != 1 ||
		busy.OldestDropped != 1 || busy.Rejected != 1 ||
		busy.QueuePeak != 960 ||
		busy.QueueAge.Samples != 1 || busy.QueueAge.TotalNS != uint64(12*time.Millisecond) ||
		busy.QueueAge.Over10MS != 1 ||
		busy.Handler.TotalNS != uint64(3*time.Millisecond) {
		t.Fatalf("per-shard loss/age/handle counters incorrect: %+v", busy)
	}
	if got[1].Enqueued != 0 || got[3].OldestDropped != 0 || got[0].Enqueued != 1 {
		t.Fatalf("shard counters mixed with neighbors: %+v", got)
	}
	payload, err := json.Marshal(p.snapshot(true))
	if err != nil { t.Fatal(err) }
	var decoded struct {
		Shards []GameIngressShardDiagnostic `json:"game_ingress_shards"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil { t.Fatal(err) }
	if len(decoded.Shards) != 4 || decoded.Shards[2].OldestDropped != 1 {
		t.Fatalf("game ingress JSON missing or inconsistent: %s", payload)
	}
}

func TestGameIngressDiagnosticsIgnoredOutsideShards(t *testing.T) {
	var p serverPipelineTiming
	p.gameIngressEnqueue(-1, 500)
	p.gameIngressDrop(-1)
	p.gameIngressReject(8)
	p.gameIngressHandled(8, time.Second, time.Second)
	for i := 0; i < 4; i++ {
		q := &p.gameIngressShards[i]
		if q.enqueued.Load() != 0 || q.handled.Load() != 0 ||
			q.oldestDropped.Load() != 0 || q.rejected.Load() != 0 {
			t.Fatalf("invalid lane index polluted shard %d metrics", i)
		}
	}
}
