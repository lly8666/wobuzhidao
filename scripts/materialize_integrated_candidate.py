from pathlib import Path
import re


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    n = s.count(old)
    if n != 1:
        raise SystemExit(f"{path}: expected exactly one match, got {n}: {old[:120]!r}")
    p.write_text(s.replace(old, new, 1))


def replace_func(path, name, new_body):
    p = Path(path)
    s = p.read_text()
    marker = f"func {name}("
    start = s.find(marker)
    if start < 0:
        raise SystemExit(f"{path}: function {name} not found")
    i = s.find("{", start)
    if i < 0:
        raise SystemExit(f"{path}: function {name} opening brace not found")
    depth = 0
    end = None
    for j in range(i, len(s)):
        if s[j] == "{":
            depth += 1
        elif s[j] == "}":
            depth -= 1
            if depth == 0:
                end = j + 1
                break
    if end is None:
        raise SystemExit(f"{path}: function {name} closing brace not found")
    p.write_text(s[:start] + new_body.rstrip() + s[end:])


# --- FakeTCP: cumulative-head-only fresh-evidence RACK. ---
replace_func(
    "internal/faketcp/arq.go",
    "(s *Sender) sackLossCandidate",
    r'''func (s *Sender) sackLossCandidate() *Pending {
	candidate := s.oldest()
	if candidate == nil || candidate.Seq != s.lastAck || candidate.SACKed || candidate.WasRetried {
		return nil
	}
	sackedAbove := 0
	for i := candidate.slot + 1; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil || !p.SACKed {
			continue
		}
		sackedAbove++
		if sackedAbove >= 3 {
			return candidate
		}
	}
	return nil
}''',
)
replace_func(
    "internal/faketcp/arq.go",
    "(s *Sender) rackLossCandidate",
    r'''func (s *Sender) rackLossCandidate(now time.Time) *Pending {
	if s.rackLatestTx.IsZero() {
		return nil
	}
	p := s.oldest()
	if p == nil || p.Seq != s.lastAck || p.SACKed || p.LastSent.IsZero() || !p.WasRetried {
		return nil
	}
	// A repeated repair is allowed only after the receiver proves delivery of a
	// transmission sent after the previous repair. Replaying an unchanged SACK
	// scoreboard therefore cannot self-clock retransmissions. Restricting the
	// candidate to the cumulative-ACK boundary also prevents four-block SACK
	// omission from being treated as proof that arbitrary non-head data was lost.
	if !p.LastSent.Before(s.rackLatestTx) {
		return nil
	}
	if now.Sub(p.LastSent) < s.rackReorderingWindow() {
		return nil
	}
	return p
}''',
)

replace_func(
    "internal/faketcp/arq_test.go",
    "TestSenderRACKDetectsLostRetransmission",
    r'''func TestSenderRACKDetectsLostRetransmission(t *testing.T) {
	now := time.Unix(7, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100 lost
	_ = s.Enqueue(make([]byte, 10), now)   // 110 delivered
	_ = s.Enqueue(make([]byte, 10), now)   // 120 delivered
	_ = s.Enqueue(make([]byte, 10), now)   // 130 delivered

	// Three later SACKed originals prove the cumulative hole and trigger repair #1.
	if got := s.AckSelective(100, []SACKBlock{{Start:110, End:140}}, now.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first scoreboard repair=%#v", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("first repair retries=%d", p1.Retries)
	}

	// New transmissions sent after repair #1 are delivered. Their LastSent time
	// is fresh RACK evidence, so repair #2 may happen after the reordering window.
	_ = s.Enqueue(make([]byte, 10), now.Add(20*time.Millisecond))
	_ = s.Enqueue(make([]byte, 10), now.Add(20*time.Millisecond))
	_ = s.Enqueue(make([]byte, 10), now.Add(20*time.Millisecond))
	if got := s.AckSelective(100, []SACKBlock{{Start:110, End:170}}, now.Add(30*time.Millisecond)); got != p1 {
		t.Fatalf("second fresh-evidence repair=%#v", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("second repair retries=%d", p1.Retries)
	}

	// Replaying exactly the same SACK evidence cannot repair again: rackLatestTx
	// has not advanced beyond the LastSent timestamp of repair #2.
	if got := s.AckSelective(100, []SACKBlock{{Start:110, End:170}}, now.Add(50*time.Millisecond)); got != nil {
		t.Fatalf("stale SACK self-clocked repair: %#v", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("stale evidence changed retries=%d", p1.Retries)
	}

	// A later batch sent after repair #2 and newly SACKed advances rackLatestTx.
	// That fresh delivery evidence permits repair #3 without waiting for 60s RTO.
	_ = s.Enqueue(make([]byte, 10), now.Add(60*time.Millisecond))
	_ = s.Enqueue(make([]byte, 10), now.Add(60*time.Millisecond))
	_ = s.Enqueue(make([]byte, 10), now.Add(60*time.Millisecond))
	if got := s.AckSelective(100, []SACKBlock{{Start:110, End:200}}, now.Add(80*time.Millisecond)); got != p1 {
		t.Fatalf("third fresh-evidence repair=%#v", got)
	}
	if p1.Retries != 3 {
		t.Fatalf("third repair retries=%d want 3", p1.Retries)
	}
	if st := s.Stats(); st.FastRetransmits != 3 || st.RTOTransmits != 0 || st.LossMarked != 1 {
		t.Fatalf("unexpected fresh-evidence RACK accounting: %#v", st)
	}
}''',
)

# Add permanent non-head safety regression if absent on the 89a baseline.
arq_test = Path("internal/faketcp/arq_test.go")
s = arq_test.read_text()
if "func TestSACKRACKDoesNotRepairNonHeadHoleBeforeCumulativeAdvance" not in s:
    anchor = "\nfunc TestSenderRTOBacksOffLikeTCP"
    if anchor not in s:
        raise SystemExit("arq_test.go: RTO anchor not found")
    test = r'''
func TestSACKRACKDoesNotRepairNonHeadHoleBeforeCumulativeAdvance(t *testing.T) {
	now := time.Unix(8, 0)
	s := NewSender(100, time.Second)
	p1 := s.Enqueue(make([]byte, 10), now) // 100 missing
	p2 := s.Enqueue(make([]byte, 10), now) // 110 missing
	_ = s.Enqueue(make([]byte, 10), now)   // 120 received
	_ = s.Enqueue(make([]byte, 10), now)   // 130 received
	_ = s.Enqueue(make([]byte, 10), now)   // 140 received

	if got := s.AckSelective(100, []SACKBlock{{Start:120, End:150}}, now.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first cumulative-hole repair=%#v want p1", got)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start:120, End:150}}, now.Add(20*time.Millisecond)); got != nil {
		t.Fatalf("non-head hole repaired before cumulative advance: %#v", got)
	}
	if p2.Retries != 0 {
		t.Fatalf("non-head retries=%d want 0", p2.Retries)
	}
	if got := s.AckSelective(110, []SACKBlock{{Start:120, End:150}}, now.Add(30*time.Millisecond)); got != p2 {
		t.Fatalf("new cumulative-hole repair=%#v want p2", got)
	}
}
'''
    arq_test.write_text(s.replace(anchor, "\n" + test + anchor, 1))

# --- FEC: preserve 89a bounded retired history, add 5411 safe forward progress. ---
replace_once(
    "internal/fec/block.go",
    "type retiredBlock struct {\n\tdelivered uint32\n\tdataCount uint8\n}",
    "type retiredBlock struct {\n\tdelivered uint32\n\tdataCount uint8\n\tfinal     bool\n\tshardSize uint16\n\tlengths   [DataShards]uint16\n}",
)
replace_once(
    "internal/fec/block.go",
    "\tb := d.blocks[h.BlockID]\n\tif b == nil {\n\t\tif err := d.makeRoom(); err != nil {\n\t\t\treturn nil, false, err\n\t\t}\n\t\tb = &decodeBlock{}\n\t\td.blocks[h.BlockID] = b\n\t\td.blockOrder = append(d.blockOrder, h.BlockID)\n\t}",
    "\tb := d.blocks[h.BlockID]\n\tif b == nil {\n\t\tif len(d.blocks) >= d.maxBlocks {\n\t\t\tif !streaming {\n\t\t\t\treturn nil, false, ErrDecoderFull\n\t\t\t}\n\t\t\tif id, victim, ok := d.oldestDegradableBefore(h.BlockID); ok {\n\t\t\t\td.retireBlock(id, victim)\n\t\t\t} else {\n\t\t\t\tr := retiredBlock{}\n\t\t\t\td.storeRetired(h.BlockID, r)\n\t\t\t\treturn d.addRetired(h, datagram[HeaderSize:], true, r)\n\t\t\t}\n\t\t}\n\t\tb = &decodeBlock{}\n\t\td.blocks[h.BlockID] = b\n\t\td.blockOrder = append(d.blockOrder, h.BlockID)\n\t}",
)

replace_func(
    "internal/fec/block.go",
    "(d *BlockDecoder) addRetired",
    r'''func (d *BlockDecoder) addRetired(h BlockHeader, payload []byte, streaming bool, r retiredBlock) ([][]byte, bool, error) {
	if streaming {
		idx := int(h.ShardIndex)
		if r.final {
			if idx >= int(r.dataCount) || r.lengths[idx] != uint16(len(payload)) {
				return nil, false, ErrHeaderMismatch
			}
		}
		bit := uint32(1) << uint(idx)
		if r.delivered&bit != 0 {
			return nil, false, nil
		}
		if !r.final {
			r.lengths[idx] = uint16(len(payload))
		}
		r.delivered |= bit
		d.storeRetired(h.BlockID, r)
		out := [][]byte{append([]byte(nil), payload...)}
		if retiredAllDelivered(r) {
			d.markCompleted(h.BlockID)
			return out, true, nil
		}
		return out, false, nil
	}

	if !r.final {
		for i := 0; i < DataShards; i++ {
			bit := uint32(1) << uint(i)
			if r.delivered&bit == 0 {
				continue
			}
			if i >= int(h.DataCount) || r.lengths[i] != h.OriginalLengths[i] {
				return nil, false, ErrHeaderMismatch
			}
		}
		r.final = true
		r.dataCount = h.DataCount
		r.shardSize = h.ShardSize
		r.lengths = h.OriginalLengths
	} else if r.dataCount != h.DataCount || r.shardSize != h.ShardSize || r.lengths != h.OriginalLengths {
		return nil, false, ErrHeaderMismatch
	}

	var out [][]byte
	idx := int(h.ShardIndex)
	if idx < int(h.DataCount) {
		bit := uint32(1) << uint(idx)
		if r.delivered&bit == 0 {
			n := int(h.OriginalLengths[idx])
			if n > len(payload) {
				return nil, false, ErrHeaderMismatch
			}
			r.delivered |= bit
			out = append(out, append([]byte(nil), payload[:n]...))
		}
	}
	d.storeRetired(h.BlockID, r)
	if retiredAllDelivered(r) {
		d.markCompleted(h.BlockID)
		return out, true, nil
	}
	return out, false, nil
}''',
)
replace_func(
    "internal/fec/block.go",
    "retiredAllDelivered",
    r'''func retiredAllDelivered(r retiredBlock) bool {
	count := DataShards
	if r.final {
		count = int(r.dataCount)
	}
	if count <= 0 || count > DataShards {
		return false
	}
	mask := uint32(1)<<uint(count) - 1
	return r.delivered&mask == mask
}''',
)

# Replace 89a's provisional-only pressure implementation with safe-degrade + bounded retired.
p = Path("internal/fec/block.go")
s = p.read_text()
start = s.find("func (d *BlockDecoder) makeRoom() error {")
end = s.find("func (d *BlockDecoder) trimRetired() {", start)
if start < 0 or end < 0:
    raise SystemExit("block.go: pressure helper range not found")
helpers = r'''func canDegradeBlock(b *decodeBlock) bool {
	if b == nil {
		return false
	}
	if !b.final {
		return true
	}
	for i := 0; i < int(b.header.DataCount); i++ {
		if b.present[i] && !b.delivered[i] {
			return false
		}
	}
	return true
}

func (d *BlockDecoder) oldestDegradableBefore(limit uint32) (uint32, *decodeBlock, bool) {
	for i := d.blockHead; i < len(d.blockOrder); i++ {
		id := d.blockOrder[i]
		b := d.blocks[id]
		if b == nil {
			if i == d.blockHead {
				d.blockHead++
			}
			continue
		}
		if id >= limit || !canDegradeBlock(b) {
			continue
		}
		d.compactBlockOrder()
		return id, b, true
	}
	d.compactBlockOrder()
	return 0, nil, false
}

func (d *BlockDecoder) storeRetired(id uint32, r retiredBlock) {
	if _, exists := d.retired[id]; !exists {
		d.retiredOrder = append(d.retiredOrder, id)
	}
	d.retired[id] = r
	d.trimRetired()
}

func (d *BlockDecoder) retireBlock(id uint32, b *decodeBlock) {
	if b == nil || !canDegradeBlock(b) {
		return
	}
	r := retiredBlock{final: b.final}
	if b.final {
		r.dataCount = b.header.DataCount
		r.shardSize = b.header.ShardSize
		r.lengths = b.header.OriginalLengths
	}
	for i := 0; i < DataShards; i++ {
		if b.delivered[i] {
			r.delivered |= uint32(1) << uint(i)
		}
		if !b.final && b.sourcePresent[i] {
			r.lengths[i] = uint16(len(b.sources[i]))
		}
	}
	delete(d.blocks, id)
	d.storeRetired(id, r)
	if retiredAllDelivered(r) {
		d.markCompleted(id)
	}
}

'''
p.write_text(s[:start] + helpers + s[end:])

# Permanent pressure regressions adapted to bounded retired storage.
Path("internal/fec/window_pressure_test.go").write_text(r'''package fec

import (
	"bytes"
	"testing"
	"time"
)

func fastTestBlockWire(t *testing.T, codec Codec, blockID uint32) [][]byte {
	t.Helper()
	enc, err := NewFastBlockEncoder(codec, 1400, time.Second, blockID)
	if err != nil {
		t.Fatal(err)
	}
	var wire [][]byte
	for i, packet := range testPackets(DataShards) {
		out, err := enc.Add(packet, time.Unix(1, int64(i)))
		if err != nil {
			t.Fatal(err)
		}
		for _, datagram := range out {
			wire = append(wire, append([]byte(nil), datagram...))
		}
	}
	if len(wire) != TotalShards {
		t.Fatalf("block %d wire=%d want=%d", blockID, len(wire), TotalShards)
	}
	return wire
}

func TestBlockDecoderStreamingWindowPressureUsesBoundedRetiredState(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil { t.Fatal(err) }
	one := fastTestBlockWire(t, codec, 1)
	two := fastTestBlockWire(t, codec, 2)
	three := fastTestBlockWire(t, codec, 3)
	for id, wire := range map[uint32][][]byte{1: one, 2: two} {
		packets, done, err := dec.Add(wire[0])
		if err != nil || done || len(packets) != 1 {
			t.Fatalf("block %d packets=%d done=%t err=%v", id, len(packets), done, err)
		}
	}
	packets, done, err := dec.Add(three[0])
	if err != nil || done || len(packets) != 1 {
		t.Fatalf("new block under pressure packets=%d done=%t err=%v", len(packets), done, err)
	}
	if dec.InFlight() != 2 || dec.blocks[1] != nil || dec.retired[1].delivered == 0 {
		t.Fatalf("pressure state in_flight=%d blocks=%v retired=%v", dec.InFlight(), dec.blocks, dec.retired)
	}

	// Late final metadata does not recreate heavy state; later systematic ARQ
	// retransmissions finish the retired block exactly once.
	if packets, done, err = dec.Add(one[DataShards]); err != nil || done || len(packets) != 0 || dec.InFlight() != 2 {
		t.Fatalf("late parity packets=%d done=%t err=%v in_flight=%d", len(packets), done, err, dec.InFlight())
	}
	want := testPackets(DataShards)
	for i := 1; i < DataShards; i++ {
		packets, done, err = dec.Add(one[i])
		if err != nil || len(packets) != 1 || !bytes.Equal(packets[0], want[i]) {
			t.Fatalf("late source %d packets=%d done=%t err=%v", i, len(packets), done, err)
		}
		if i == 1 {
			dup, dupDone, dupErr := dec.Add(one[i])
			if dupErr != nil || dupDone || len(dup) != 0 {
				t.Fatalf("duplicate late source packets=%d done=%t err=%v", len(dup), dupDone, dupErr)
			}
		}
	}
	if _, ok := dec.retired[1]; ok || !dec.completed.contains(1) {
		t.Fatalf("completed retired block retained=%t completed=%t", ok, dec.completed.contains(1))
	}
}

func TestBlockDecoderStreamingPressureDoesNotEvictUnsafeReferenceState(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 2)
	if err != nil { t.Fatal(err) }
	for id := uint32(10); id <= 11; id++ {
		wire := testBlockWire(t, codec, id)
		if _, _, err := dec.Add(wire[0]); err != nil { t.Fatal(err) }
	}
	if canDegradeBlock(dec.blocks[10]) || canDegradeBlock(dec.blocks[11]) {
		t.Fatal("reference blocks unexpectedly degradable")
	}
	streaming := fastTestBlockWire(t, codec, 12)
	packets, done, err := dec.Add(streaming[0])
	if err != nil || done || len(packets) != 1 {
		t.Fatalf("streaming fallback packets=%d done=%t err=%v", len(packets), done, err)
	}
	if dec.InFlight() != 2 || dec.blocks[10] == nil || dec.blocks[11] == nil {
		t.Fatalf("unsafe heavy state was evicted: in_flight=%d", dec.InFlight())
	}
	if _, ok := dec.retired[12]; !ok {
		t.Fatal("new streaming block did not enter bounded retired fallback")
	}
}

func TestBlockDecoderPressureCanRetireSafeFinalStreamingState(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 1)
	if err != nil { t.Fatal(err) }
	one := fastTestBlockWire(t, codec, 1)
	two := fastTestBlockWire(t, codec, 2)
	if _, _, err := dec.Add(one[0]); err != nil { t.Fatal(err) }
	if _, _, err := dec.Add(one[DataShards]); err != nil { t.Fatal(err) }
	if !dec.blocks[1].final || !canDegradeBlock(dec.blocks[1]) {
		t.Fatal("streaming final state should be safe to degrade")
	}
	if _, _, err := dec.Add(two[0]); err != nil { t.Fatalf("new block: %v", err) }
	if dec.blocks[1] != nil || dec.retired[1].dataCount != DataShards {
		t.Fatalf("safe final state not retired: blocks=%v retired=%v", dec.blocks, dec.retired)
	}
}

func TestBlockDecoderRetiredPressureHistoryIsHardBounded(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	dec, err := NewBlockDecoder(codec, 1400, 1)
	if err != nil { t.Fatal(err) }
	for id := uint32(1); id <= maxRetiredBlocks+64; id++ {
		dec.storeRetired(id, retiredBlock{delivered: 1})
	}
	if got := len(dec.retired); got != maxRetiredBlocks {
		t.Fatalf("retired=%d want=%d", got, maxRetiredBlocks)
	}
	for id := uint32(1); id <= 64; id++ {
		if !dec.completed.contains(id) {
			t.Fatalf("trimmed retired block %d not fenced completed", id)
		}
	}
}
''')

print("WBD_INTEGRATED_PATCH_APPLIED")
