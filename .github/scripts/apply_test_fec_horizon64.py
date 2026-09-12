#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: apply_test_fec_horizon64.py PRODUCT_DIR")
root = Path(sys.argv[1])
p = root / "internal/fec/block.go"
s = p.read_text()
old = '''func (d *BlockDecoder) oldestRetirableBefore(limit uint32) (uint32, *decodeBlock, bool) {
\tfor i := d.blockHead; i < len(d.blockOrder); i++ {
\t\tid := d.blockOrder[i]
\t\tb := d.blocks[id]
\t\tif b == nil {
\t\t\tif i == d.blockHead {
\t\t\t\td.blockHead++
\t\t\t}
\t\t\tcontinue
\t\t}
\t\tif id >= limit || !canRetireBlock(b) {
\t\t\tcontinue
\t\t}
\t\td.compactBlockOrder()
\t\treturn id, b, true
\t}
\td.compactBlockOrder()
\treturn 0, nil, false
}

func (d *BlockDecoder) retireBlock(id uint32, b *decodeBlock) {
\tif b == nil || !canRetireBlock(b) {
\t\treturn
\t}
'''
new = '''func (d *BlockDecoder) oldestRetirableBefore(limit uint32) (uint32, *decodeBlock, bool) {
\tfor i := d.blockHead; i < len(d.blockOrder); i++ {
\t\tid := d.blockOrder[i]
\t\tb := d.blocks[id]
\t\tif b == nil {
\t\t\tif i == d.blockHead {
\t\t\t\td.blockHead++
\t\t\t}
\t\t\tcontinue
\t\t}
\t\tif id >= limit {
\t\t\tcontinue
\t\t}
\t\tage := limit - id
\t\tif !canRetireBlock(b) && age <= uint32(d.maxBlocks) {
\t\t\tcontinue
\t\t}
\t\td.compactBlockOrder()
\t\treturn id, b, true
\t}
\td.compactBlockOrder()
\treturn 0, nil, false
}

func (d *BlockDecoder) retireBlock(id uint32, b *decodeBlock) {
\tif b == nil {
\t\treturn
\t}
'''
if s.count(old) != 1:
    raise SystemExit(f"bounded-horizon patch marker drift: {s.count(old)}")
p.write_text(s.replace(old, new, 1))

test = root / "internal/fec/recovery_horizon_test.go"
test.write_text(r'''package fec

import (
    "bytes"
    "testing"
    "time"
)

func TestBlockDecoderPressureExpiresBeyondRecoveryHorizon(t *testing.T) {
    codec := NewReedSolomon20x20()
    makeBlock := func(blockID uint32) ([][]byte, [][]byte, [][]byte) {
        t.Helper()
        enc, err := NewFastBlockEncoder(codec, 1400, time.Millisecond, blockID)
        if err != nil { t.Fatal(err) }
        want := testPackets(DataShards)
        sources := make([][]byte, 0, DataShards)
        var parity [][]byte
        for i, packet := range want {
            out, err := enc.Add(packet, time.Unix(0, int64(i)))
            if err != nil { t.Fatal(err) }
            if len(out) == 0 { t.Fatalf("block %d source %d emitted no systematic shard", blockID, i) }
            sources = append(sources, append([]byte(nil), out[0]...))
            for _, wire := range out[1:] { parity = append(parity, append([]byte(nil), wire...)) }
        }
        return want, sources, parity
    }

    want1, source1, parity1 := makeBlock(1)
    _, source2, _ := makeBlock(2)
    _, source3, _ := makeBlock(3)
    dec, err := NewBlockDecoder(codec, 1400, 1)
    if err != nil { t.Fatal(err) }
    const missing = 7
    for i, wire := range source1 {
        if i == missing { continue }
        packets, done, err := dec.Add(wire)
        if err != nil { t.Fatalf("block1 source %d: %v", i, err) }
        if done || len(packets) != 1 { t.Fatalf("block1 source %d done=%v delivered=%d", i, done, len(packets)) }
    }

    if _, _, err := dec.Add(source2[0]); err != nil { t.Fatalf("block2: %v", err) }
    if dec.blocks[1] == nil { t.Fatal("block2 evicted still-recoverable block1 inside horizon") }
    if _, ok := dec.retired[2]; !ok { t.Fatal("block2 should use compact fallback while block1 is protected") }

    packets, done, err := dec.Add(source3[0])
    if err != nil { t.Fatalf("block3: %v", err) }
    if done || len(packets) != 1 { t.Fatalf("block3 done=%v delivered=%d", done, len(packets)) }
    if dec.blocks[1] != nil { t.Fatal("block1 still owns heavy slot after exceeding one-window recovery horizon") }
    if _, ok := dec.retired[1]; !ok { t.Fatal("expired block1 did not retain compact first-delivery state") }
    if dec.blocks[3] == nil { t.Fatal("block3 did not acquire released heavy slot") }

    packets, done, err = dec.Add(parity1[0])
    if err != nil { t.Fatalf("late block1 parity: %v", err) }
    if done || len(packets) != 0 { t.Fatalf("expired block reconstructed from discarded heavy shards: done=%v packets=%d", done, len(packets)) }

    packets, done, err = dec.Add(source1[missing])
    if err != nil { t.Fatalf("late block1 systematic: %v", err) }
    if !done || len(packets) != 1 || !bytes.Equal(packets[0], want1[missing]) {
        t.Fatalf("late systematic first delivery after expiry: done=%v packets=%d", done, len(packets))
    }
}
''')
print("WBD_TEST_FEC_HORIZON64_PATCHED", p)
