package fec

import "testing"

// The physical Windows acceptance matrix exercises all negotiated parity
// profiles against the same 20-shard data block. Keep the codec's maximum
// 20:20 geometry stable so control-plane profiles remain wire-compatible.
func TestPhysicalAcceptanceShardContract(t *testing.T) {
	if DataShards != 20 {
		t.Fatalf("data shards=%d want 20", DataShards)
	}
	if ParityShards != 20 {
		t.Fatalf("maximum parity shards=%d want 20", ParityShards)
	}
	if TotalShards != 40 {
		t.Fatalf("total shards=%d want 40", TotalShards)
	}
}
