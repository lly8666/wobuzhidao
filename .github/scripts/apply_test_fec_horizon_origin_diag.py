#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: apply_test_fec_horizon_origin_diag.py PRODUCT_DIR")
root = Path(sys.argv[1])
block = root / "internal/fec/block.go"
s = block.read_text()

def replace_once(old, new, label):
    global s
    n = s.count(old)
    if n != 1:
        raise SystemExit(f"horizon-origin diag {label} marker drift: {n}")
    s = s.replace(old, new, 1)

replace_once('''type retiredBlock struct {
\theader    BlockHeader
\tfinal     bool
\tdelivered uint32
}
''', '''const (
\tdiagRetiredOriginNone uint8 = iota
\tdiagRetiredOriginIncomingFallback
\tdiagRetiredOriginStaleHeavy
)

type retiredBlock struct {
\theader    BlockHeader
\tfinal     bool
\tdelivered uint32

\t// Diagnostic-only attribution. diagSeen remembers only shard indexes, never
\t// payload bytes, so it cannot change compact-state recovery behavior.
\tdiagOrigin      uint8
\tdiagSeen        uint64
\tdiagWouldDecode bool
}
''', 'retired block')

replace_once('''\tcompleted completedBlockSet
}
''', '''\tcompleted completedBlockSet

\t// Diagnostic-only counters for bounded-horizon attribution.
\tdiagIncomingCompactFallbacks    uint64
\tdiagStaleHeavyRetires           uint64
\tdiagStaleRetireMissingSources   uint64
\tdiagStaleWouldDecodeAfterRetire uint64
\tdiagFallbackWouldDecodeCompact  uint64
}
''', 'decoder counters')

replace_once('''\t\t\t} else if streaming {
\t\t\t\tr := retiredBlock{}
\t\t\t\td.addRetiredState(h.BlockID, r)
''', '''\t\t\t} else if streaming {
\t\t\t\tr := retiredBlock{diagOrigin: diagRetiredOriginIncomingFallback}
\t\t\t\td.diagIncomingCompactFallbacks++
\t\t\t\td.addRetiredState(h.BlockID, r)
''', 'incoming compact fallback')

replace_once('''func (d *BlockDecoder) retireBlock(id uint32, b *decodeBlock) {
\tif b == nil {
\t\treturn
\t}
\tdelete(d.blocks, id)
\tr := retiredBlock{header: b.header, final: b.final}
''', '''func (d *BlockDecoder) retireBlock(id uint32, b *decodeBlock) {
\tif b == nil {
\t\treturn
\t}
\tdelete(d.blocks, id)
\torigin := diagRetiredOriginNone
\tif !canRetireBlock(b) {
\t\torigin = diagRetiredOriginStaleHeavy
\t\td.diagStaleHeavyRetires++
\t\tcount := DataShards
\t\tif b.final {
\t\t\tcount = int(b.header.DataCount)
\t\t}
\t\tfor i := 0; i < count; i++ {
\t\t\tif !b.delivered[i] {
\t\t\t\td.diagStaleRetireMissingSources++
\t\t\t}
\t\t}
\t}
\tr := retiredBlock{header: b.header, final: b.final, diagOrigin: origin}
\tif origin == diagRetiredOriginStaleHeavy {
\t\tfor i, present := range b.present {
\t\t\tif present {
\t\t\t\tr.diagSeen |= uint64(1) << uint(i)
\t\t\t}
\t\t}
\t\tfor i, present := range b.sourcePresent {
\t\t\tif present {
\t\t\t\tr.diagSeen |= uint64(1) << uint(i)
\t\t\t}
\t\t}
\t}
''', 'stale retirement')

replace_once('''\t\tr.delivered |= bit
\t\td.retired[h.BlockID] = r
\t\tout := [][]byte{append([]byte(nil), payload...)}
''', '''\t\tr.delivered |= bit
\t\td.diagObserveRetiredShard(&r, idx)
\t\td.retired[h.BlockID] = r
\t\tout := [][]byte{append([]byte(nil), payload...)}
''', 'retired streaming observation')

replace_once('''\t\tr.header = h
\t\tr.final = true
\t} else if !sameBlockHeader(r.header, h) {
''', '''\t\tr.header = h
\t\tr.final = true
\t\tfor i := int(h.DataCount); i < DataShards; i++ {
\t\t\tr.diagSeen |= uint64(1) << uint(i)
\t\t}
\t} else if !sameBlockHeader(r.header, h) {
''', 'retired final metadata')

replace_once('''\td.retired[h.BlockID] = r
\tif retiredAllDelivered(r) {
''', '''\td.diagObserveRetiredShard(&r, idx)
\td.retired[h.BlockID] = r
\tif retiredAllDelivered(r) {
''', 'retired final shard observation')

replace_once('''func retiredAllDelivered(r retiredBlock) bool {
''', '''func (d *BlockDecoder) diagObserveRetiredShard(r *retiredBlock, idx int) {
\tif r == nil || idx < 0 || idx >= TotalShards {
\t\treturn
\t}
\tr.diagSeen |= uint64(1) << uint(idx)
\tif r.diagWouldDecode || retiredAllDelivered(*r) {
\t\treturn
\t}
\tpresent := 0
\tfor i := 0; i < TotalShards; i++ {
\t\tif r.diagSeen&(uint64(1)<<uint(i)) != 0 {
\t\t\tpresent++
\t\t}
\t}
\tif present < DataShards {
\t\treturn
\t}
\tr.diagWouldDecode = true
\tswitch r.diagOrigin {
\tcase diagRetiredOriginStaleHeavy:
\t\td.diagStaleWouldDecodeAfterRetire++
\tcase diagRetiredOriginIncomingFallback:
\t\td.diagFallbackWouldDecodeCompact++
\t}
}

func retiredAllDelivered(r retiredBlock) bool {
''', 'decode opportunity helper')

block.write_text(s)

obs = root / "internal/fec/decoder_observe.go"
o = obs.read_text()
def orepl(old, new, label):
    global o
    n = o.count(old)
    if n != 1:
        raise SystemExit(f"horizon-origin diag observer {label} marker drift: {n}")
    o = o.replace(old, new, 1)

orepl('''\tActiveMissingSources  int `json:"active_missing_sources"`
}
''', '''\tActiveMissingSources  int `json:"active_missing_sources"`

\tRetiredFallbackIncomplete     int    `json:"retired_fallback_incomplete"`
\tRetiredFallbackMissingSources int    `json:"retired_fallback_missing_sources"`
\tRetiredStaleIncomplete        int    `json:"retired_stale_incomplete"`
\tRetiredStaleMissingSources    int    `json:"retired_stale_missing_sources"`
\tIncomingCompactFallbacks      uint64 `json:"incoming_compact_fallbacks"`
\tStaleHeavyRetires             uint64 `json:"stale_heavy_retires"`
\tStaleRetireMissingSources     uint64 `json:"stale_retire_missing_sources"`
\tStaleWouldDecodeAfterRetire   uint64 `json:"stale_would_decode_after_retire"`
\tFallbackWouldDecodeCompact    uint64 `json:"fallback_would_decode_compact"`
}
''', 'stats fields')

orepl('''\ts := DecoderPressureStats{
\t\tInFlight:  len(d.blocks),
\t\tMaxBlocks: d.maxBlocks,
\t\tRetired:   len(d.retired),
\t}
''', '''\ts := DecoderPressureStats{
\t\tInFlight:                    len(d.blocks),
\t\tMaxBlocks:                   d.maxBlocks,
\t\tRetired:                     len(d.retired),
\t\tIncomingCompactFallbacks:    d.diagIncomingCompactFallbacks,
\t\tStaleHeavyRetires:           d.diagStaleHeavyRetires,
\t\tStaleRetireMissingSources:   d.diagStaleRetireMissingSources,
\t\tStaleWouldDecodeAfterRetire: d.diagStaleWouldDecodeAfterRetire,
\t\tFallbackWouldDecodeCompact:  d.diagFallbackWouldDecodeCompact,
\t}
''', 'stats init')

orepl('''\t\tif missing != 0 {
\t\t\ts.RetiredIncomplete++
\t\t\ts.RetiredMissingSources += missing
\t\t}
''', '''\t\tif missing != 0 {
\t\t\ts.RetiredIncomplete++
\t\t\ts.RetiredMissingSources += missing
\t\t\tswitch r.diagOrigin {
\t\t\tcase diagRetiredOriginIncomingFallback:
\t\t\t\ts.RetiredFallbackIncomplete++
\t\t\t\ts.RetiredFallbackMissingSources += missing
\t\t\tcase diagRetiredOriginStaleHeavy:
\t\t\t\ts.RetiredStaleIncomplete++
\t\t\t\ts.RetiredStaleMissingSources += missing
\t\t\t}
\t\t}
''', 'retired attribution')
obs.write_text(o)

test = root / "internal/fec/horizon_origin_diag_test.go"
test.write_text(r'''package fec

import (
    "testing"
    "time"
)

func TestHorizonOriginDiagnosticsSeeLostDecodeOpportunity(t *testing.T) {
    codec := NewReedSolomon20x20()
    makeBlock := func(blockID uint32) ([][]byte, [][]byte) {
        t.Helper()
        enc, err := NewFastBlockEncoder(codec, 1400, time.Millisecond, blockID)
        if err != nil { t.Fatal(err) }
        want := testPackets(DataShards)
        var sources, parity [][]byte
        for i, packet := range want {
            out, err := enc.Add(packet, time.Unix(0, int64(i)))
            if err != nil { t.Fatal(err) }
            if len(out) == 0 { t.Fatalf("block %d source %d emitted no systematic shard", blockID, i) }
            sources = append(sources, append([]byte(nil), out[0]...))
            for _, wire := range out[1:] { parity = append(parity, append([]byte(nil), wire...)) }
        }
        return sources, parity
    }

    source1, parity1 := makeBlock(1)
    source2, parity2 := makeBlock(2)
    source3, _ := makeBlock(3)
    dec, err := NewBlockDecoder(codec, 1400, 1)
    if err != nil { t.Fatal(err) }

    const missing = 7
    for i, wire := range source1 {
        if i == missing { continue }
        if _, _, err := dec.Add(wire); err != nil { t.Fatalf("block1 source %d: %v", i, err) }
    }

    if _, _, err := dec.Add(source2[0]); err != nil { t.Fatalf("block2 fallback: %v", err) }
    if got := dec.PressureStats().IncomingCompactFallbacks; got != 1 {
        t.Fatalf("incoming compact fallbacks=%d want=1", got)
    }
    for i := 0; i < DataShards-1; i++ {
        if _, _, err := dec.Add(parity2[i]); err != nil { t.Fatalf("block2 parity %d: %v", i, err) }
    }
    if got := dec.PressureStats().FallbackWouldDecodeCompact; got != 1 {
        t.Fatalf("fallback decode opportunities=%d want=1", got)
    }

    if _, _, err := dec.Add(source3[0]); err != nil { t.Fatalf("block3 stale retire: %v", err) }
    stats := dec.PressureStats()
    if stats.StaleHeavyRetires != 1 || stats.StaleRetireMissingSources != 1 {
        t.Fatalf("stale retire stats=%+v", stats)
    }
    if _, _, err := dec.Add(parity1[0]); err != nil { t.Fatalf("block1 late parity: %v", err) }
    stats = dec.PressureStats()
    if stats.StaleWouldDecodeAfterRetire != 1 {
        t.Fatalf("stale decode opportunities=%d want=1 stats=%+v", stats.StaleWouldDecodeAfterRetire, stats)
    }
}
''')
print("WBD_TEST_FEC_HORIZON_ORIGIN_DIAG_PATCHED", block)
