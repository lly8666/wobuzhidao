//go:build linux

package openwrtclient

import (
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformflow"
)

func testIngressItem(client string, payload byte, when time.Time) udpIngressItem {
	return udpIngressItem{
		client:   netip.MustParseAddrPort(client),
		target:   netip.MustParseAddrPort("10.50.0.2:18080"),
		payload:  []byte{payload},
		queuedAt: when,
	}
}

func TestUDPIngressQueueGlobalBoundDropsOldestKeepsNewest(t *testing.T) {
	q := newUDPIngressQueue(3)
	defer q.close()
	base := time.Unix(200, 0)
	client := "10.40.0.2:24000"
	for i := 1; i <= 4; i++ {
		item := testIngressItem(client, byte(i), base.Add(time.Duration(i)*time.Millisecond))
		if !q.enqueue(item, item.queuedAt) {
			t.Fatalf("enqueue %d rejected", i)
		}
	}
	d := q.snapshot()
	if d.QueueCapacityRecords != 3 || d.QueueCapacityBytes != 3*platformflow.MaxPayload {
		t.Fatalf("capacity diagnostic=%+v", d)
	}
	if d.QueueCurrent != 3 || d.TotalPeak != 3 || d.OverflowDrops != 1 || d.OverflowBytes != 1 {
		t.Fatalf("bounded queue diagnostic=%+v", d)
	}
	if d.Workers != udpIngressWorkers || d.EvictionMaxScan > udpIngressWorkers {
		t.Fatalf("worker/scan diagnostic=%+v", d)
	}
	shard := q.shardFor(netip.MustParseAddrPort(client))
	for want := byte(2); want <= 4; want++ {
		item, ok := q.popShard(shard)
		if !ok || len(item.payload) != 1 || item.payload[0] != want {
			t.Fatalf("want payload=%d item=%+v ok=%v", want, item, ok)
		}
		q.finish(item, false, false)
	}
}

func TestUDPIngressQueueClientShardIsDeterministicFIFO(t *testing.T) {
	q := newUDPIngressQueue(8)
	defer q.close()
	base := time.Unix(210, 0)
	client := netip.MustParseAddrPort("10.40.0.2:25000")
	wantShard := q.shardFor(client)
	for i := 0; i < 4; i++ {
		item := testIngressItem(client.String(), byte(i), base.Add(time.Duration(i)*time.Millisecond))
		if got := q.shardFor(item.client); got != wantShard {
			t.Fatalf("client changed shard %d -> %d", wantShard, got)
		}
		if !q.enqueue(item, item.queuedAt) {
			t.Fatal("enqueue rejected")
		}
	}
	for i := 0; i < 4; i++ {
		item, ok := q.popShard(wantShard)
		if !ok || len(item.payload) != 1 || item.payload[0] != byte(i) {
			t.Fatalf("pop %d item=%+v ok=%v", i, item, ok)
		}
		q.finish(item, false, false)
	}
	d := q.snapshot()
	if d.QueueCurrent != 0 || d.InFlightCurrent != 0 {
		t.Fatalf("queue not drained: %+v", d)
	}
}

func TestUDPIngressQueueInflightCountsAgainstGlobalBudget(t *testing.T) {
	q := newUDPIngressQueue(2)
	defer q.close()
	base := time.Unix(220, 0)
	first := testIngressItem("10.40.0.2:26000", 1, base)
	second := testIngressItem("10.40.0.2:26001", 2, base)
	if !q.enqueue(first, base) || !q.enqueue(second, base) {
		t.Fatal("initial enqueue rejected")
	}
	item, ok := q.popShard(q.shardFor(first.client))
	if !ok {
		t.Fatal("pop failed")
	}
	third := testIngressItem("10.40.0.2:26002", 3, base.Add(time.Millisecond))
	if !q.enqueue(third, third.queuedAt) {
		t.Fatal("replacement enqueue rejected")
	}
	d := q.snapshot()
	if got := d.QueueCurrent + d.InFlightCurrent; got != 2 {
		t.Fatalf("global pending=%d want=2 diagnostic=%+v", got, d)
	}
	if d.QueueBytes+d.InFlightBytes > d.QueueCapacityBytes {
		t.Fatalf("byte budget exceeded: %+v", d)
	}
	if d.OverflowDrops != 1 {
		t.Fatalf("overflow drops=%d want=1", d.OverflowDrops)
	}
	q.finish(item, false, false)
}
