package faketcp

import "testing"

func TestReceiverSACKCoverageRotatesOlderRanges(t *testing.T) {
	r := NewReceiver(100)
	for _, seq := range []uint32{110, 130, 150, 170, 190, 210, 230} {
		if deliver, sack := r.Accept(seq, 10); !deliver || !sack {
			t.Fatalf("Accept(%d): deliver=%v sack=%v", seq, deliver, sack)
		}
	}

	seen := map[uint32]bool{}
	for round := 0; round < 2; round++ {
		var blocks [4]SACKBlock
		n := r.SACKBlocks(&blocks)
		if n != 4 {
			t.Fatalf("round %d: n=%d blocks=%v", round, n, blocks[:n])
		}
		if blocks[0] != (SACKBlock{Start: 230, End: 240}) {
			t.Fatalf("round %d: first block lost latest-arrival semantics: %v", round, blocks[:n])
		}
		for _, b := range blocks[1:n] {
			seen[b.Start] = true
		}
	}
	for _, start := range []uint32{110, 130, 150, 170, 190, 210} {
		if !seen[start] {
			t.Fatalf("older received range %d was never covered; seen=%v", start, seen)
		}
	}
}

func TestReceiverSACKCoverageKeepsNormalRecentOrder(t *testing.T) {
	r := NewReceiver(100)
	for _, seq := range []uint32{110, 130, 150, 170} {
		r.Accept(seq, 10)
	}
	var blocks [4]SACKBlock
	n := r.SACKBlocks(&blocks)
	want := []SACKBlock{{170, 180}, {150, 160}, {130, 140}, {110, 120}}
	if n != len(want) {
		t.Fatalf("n=%d want=%d blocks=%v", n, len(want), blocks[:n])
	}
	for i := range want {
		if blocks[i] != want[i] {
			t.Fatalf("block[%d]=%v want=%v", i, blocks[i], want[i])
		}
	}
}
