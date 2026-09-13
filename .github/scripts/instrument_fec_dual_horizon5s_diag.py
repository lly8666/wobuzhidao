#!/usr/bin/env python3
import hashlib
import sys
from pathlib import Path


def blob_sha(data: bytes) -> str:
    h = hashlib.sha1()
    h.update(f"blob {len(data)}\0".encode())
    h.update(data)
    return h.hexdigest()


def replace_once(text: str, old: str, new: str, label: str) -> str:
    n = text.count(old)
    if n != 1:
        raise SystemExit(f"{label}: expected 1 match, got {n}")
    return text.replace(old, new, 1)


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: instrument_fec_dual_horizon5s_diag.py PRODUCT_DIR")
    root = Path(sys.argv[1])
    block_path = root / "internal/fec/block.go"
    observe_path = root / "internal/fec/decoder_observe.go"
    path_path = root / "internal/linkdata/path.go"
    fec_observe_path = root / "internal/linkdata/fec_observe.go"

    block = block_path.read_text()
    observe = observe_path.read_text()
    path = path_path.read_text()
    fec_observe = fec_observe_path.read_text()

    block = replace_once(
        block,
        "const heavyRecoveryHorizonBlocks uint32 = 64\n",
        "const heavyRecoveryHorizonBlocks uint32 = 64\n\n"
        "// The generation bound is not a wall-clock bound when BlockID progress\n"
        "// stalls. This 5s value is a test candidate for the 300ms one-way\n"
        "// transient profile and remains deliberately separate from production.\n"
        "const heavyRecoveryHorizonTime time.Duration = 5 * time.Second\n",
        "add time horizon constant",
    )
    block = replace_once(
        block,
        "\thorizonRetireEvents      uint64\n\thorizonRetiredIncomplete uint64\n",
        "\thorizonRetireEvents          uint64\n"
        "\thorizonRetiredIncomplete     uint64\n"
        "\tgenerationHorizonRetireEvents uint64\n"
        "\ttimeHorizonRetireEvents       uint64\n",
        "add categorical retirement counters",
    )
    block = replace_once(
        block,
        "func (d *BlockDecoder) InFlight() int { return len(d.blocks) }\n",
        "func (d *BlockDecoder) InFlight() int { return len(d.blocks) }\n\n"
        "// Expire is safe to call from an existing session timer even when no\n"
        "// new wire datagram arrives. It only retires already-expired heavy FEC\n"
        "// state into compact late-systematic delivery state.\n"
        "func (d *BlockDecoder) Expire(now time.Time) { d.retireExpiredHeavy(now) }\n",
        "add explicit expiry entry point",
    )
    block = replace_once(
        block,
        '''func (d *BlockDecoder) observeBlockID(id uint32) {
\tif !d.haveLatestBlockID {
\t\td.latestBlockID = id
\t\td.haveLatestBlockID = true
\t\treturn
\t}
\tif !blockIDAhead(id, d.latestBlockID) {
\t\treturn
\t}
\td.latestBlockID = id
\td.retireExpiredHeavy()
}
''',
        '''func (d *BlockDecoder) observeBlockID(id uint32, now time.Time) {
\tif !d.haveLatestBlockID {
\t\td.latestBlockID = id
\t\td.haveLatestBlockID = true
\t} else if blockIDAhead(id, d.latestBlockID) {
\t\td.latestBlockID = id
\t}
\td.retireExpiredHeavy(now)
}
''',
        "observe block id",
    )
    block = replace_once(
        block,
        '''func (d *BlockDecoder) retireExpiredHeavy() {
\tfor _, id := range d.blockOrder[d.blockHead:] {
\t\tb := d.blocks[id]
\t\tif b == nil || !d.blockPastRecoveryHorizon(id) {
\t\t\tcontinue
\t\t}
\t\td.retireBlockAfterHorizon(id, b)
\t}
\td.compactBlockOrder()
}
''',
        '''func blockPastRecoveryTimeHorizon(b *decodeBlock, now time.Time) bool {
\treturn b != nil && !b.firstAt.IsZero() && now.Sub(b.firstAt) >= heavyRecoveryHorizonTime
}

func (d *BlockDecoder) retireExpiredHeavy(now time.Time) {
\tfor _, id := range d.blockOrder[d.blockHead:] {
\t\tb := d.blocks[id]
\t\tif b == nil || (!d.blockPastRecoveryHorizon(id) && !blockPastRecoveryTimeHorizon(b, now)) {
\t\t\tcontinue
\t\t}
\t\td.retireBlockAfterHorizon(id, b, now)
\t}
\td.compactBlockOrder()
}
''',
        "dual horizon expiry",
    )
    block = replace_once(
        block,
        "\td.observeBlockID(h.BlockID)\n",
        "\tnow := time.Now()\n\td.observeBlockID(h.BlockID, now)\n",
        "observe call with time",
    )
    block = replace_once(
        block,
        "\t\tb = &decodeBlock{firstAt: time.Now()}\n",
        "\t\tb = &decodeBlock{firstAt: now}\n",
        "block firstAt",
    )
    block = replace_once(
        block,
        "func (d *BlockDecoder) retireBlockAfterHorizon(id uint32, b *decodeBlock) {\n\tif b == nil || !d.blockPastRecoveryHorizon(id) {\n\t\treturn\n\t}\n",
        "func (d *BlockDecoder) retireBlockAfterHorizon(id uint32, b *decodeBlock, now time.Time) {\n"
        "\texpiredByGeneration := d.blockPastRecoveryHorizon(id)\n"
        "\texpiredByTime := blockPastRecoveryTimeHorizon(b, now)\n"
        "\tif b == nil || (!expiredByGeneration && !expiredByTime) {\n\t\treturn\n\t}\n",
        "retire signature and reason",
    )
    block = replace_once(
        block,
        "\td.horizonRetireEvents++\n\tif incomplete {\n",
        "\td.horizonRetireEvents++\n"
        "\t// Attribute each retirement to one sufficient cause so generation +\n"
        "\t// time counters sum exactly to the total. Generation wins ties.\n"
        "\tif expiredByGeneration {\n"
        "\t\td.generationHorizonRetireEvents++\n"
        "\t} else {\n"
        "\t\td.timeHorizonRetireEvents++\n"
        "\t}\n"
        "\tif incomplete {\n",
        "categorical retire reason",
    )

    observe = replace_once(
        observe,
        '\tRecoveryHorizonBlocks    uint32 `json:"recovery_horizon_blocks"`\n',
        '\tRecoveryHorizonBlocks    uint32 `json:"recovery_horizon_blocks"`\n'
        '\tRecoveryHorizonMillis    int64  `json:"recovery_horizon_ms"`\n',
        "observe horizon ms",
    )
    observe = replace_once(
        observe,
        '\tHorizonRetiredIncomplete uint64 `json:"horizon_retired_incomplete"`\n',
        '\tHorizonRetiredIncomplete      uint64 `json:"horizon_retired_incomplete"`\n'
        '\tGenerationHorizonRetireEvents uint64 `json:"generation_horizon_retire_events"`\n'
        '\tTimeHorizonRetireEvents       uint64 `json:"time_horizon_retire_events"`\n',
        "observe categorical reasons",
    )
    observe = replace_once(
        observe,
        "\t\tRecoveryHorizonBlocks:    heavyRecoveryHorizonBlocks,\n",
        "\t\tRecoveryHorizonBlocks:    heavyRecoveryHorizonBlocks,\n"
        "\t\tRecoveryHorizonMillis:    heavyRecoveryHorizonTime.Milliseconds(),\n",
        "observe horizon ms value",
    )
    observe = replace_once(
        observe,
        "\t\tHorizonRetiredIncomplete: d.horizonRetiredIncomplete,\n",
        "\t\tHorizonRetiredIncomplete:      d.horizonRetiredIncomplete,\n"
        "\t\tGenerationHorizonRetireEvents: d.generationHorizonRetireEvents,\n"
        "\t\tTimeHorizonRetireEvents:       d.timeHorizonRetireEvents,\n",
        "observe reason values",
    )

    path = replace_once(
        path,
        '''func (p *Path) FlushDue(now time.Time) ([][]byte, error) {
\tif !p.FECEnabled() {
\t\treturn nil, nil
\t}
\twire, err := p.enc.FlushDue(now)
''',
        '''func (p *Path) FlushDue(now time.Time) ([][]byte, error) {
\tif !p.FECEnabled() {
\t\treturn nil, nil
\t}
\t// FlushDue is already driven by the session/server timer. Expire decoder
\t// state here so wall-clock retirement does not require a new inbound packet.
\tp.dec.Expire(now)
\tobserveFECDecoder(p, now)
\twire, err := p.enc.FlushDue(now)
''',
        "path timer expiry",
    )

    fec_observe = replace_once(
        fec_observe,
        'type FECObserveStats struct {\n',
        'type FECObserveStats struct {\n\tEpochUnixNano              int64                       `json:"epoch_unix_nano"`\n',
        "FEC report epoch field",
    )
    fec_observe = replace_once(
        fec_observe,
        '\tout := FECObserveStats{}\n',
        '\tout := FECObserveStats{EpochUnixNano: time.Now().UnixNano()}\n',
        "FEC report epoch value",
    )

    block_path.write_text(block)
    observe_path.write_text(observe)
    path_path.write_text(path)
    fec_observe_path.write_text(fec_observe)

    test_path = root / "internal/fec/block_dual_horizon_test.go"
    test_path.write_text(r'''package fec

import (
    "bytes"
    "testing"
    "time"
)

func makeDualHorizonZombie(t *testing.T, dec *BlockDecoder, blockID uint32) ([][]byte, [][]byte) {
    t.Helper()
    want, sources, parity := makeHorizonFastBlock(t, blockID)
    for i := 0; i < DataShards-2; i++ {
        packets, done, err := dec.Add(sources[i])
        if err != nil {
            t.Fatalf("source %d: %v", i, err)
        }
        if done || len(packets) != 1 || !bytes.Equal(packets[0], want[i]) {
            t.Fatalf("source %d done=%v delivered=%d", i, done, len(packets))
        }
    }
    if packets, done, err := dec.Add(parity[0]); err != nil || done || len(packets) != 0 {
        t.Fatalf("parity done=%v delivered=%d err=%v", done, len(packets), err)
    }
    return want, sources
}

func TestBlockDecoderTimeHorizonRetiresWithoutNewPacket(t *testing.T) {
    dec, err := NewBlockDecoder(NewReedSolomon20x20(), 1400, 640)
    if err != nil { t.Fatal(err) }
    want, sources := makeDualHorizonZombie(t, dec, 1)
    b := dec.blocks[1]
    if b == nil { t.Fatal("zombie block missing") }
    b.firstAt = time.Now().Add(-heavyRecoveryHorizonTime - time.Millisecond)

    // No Add call here: expiry must be driven solely by the existing timer path.
    dec.Expire(time.Now())
    if dec.blocks[1] != nil { t.Fatal("time-expired block still occupies heavy state") }
    if _, ok := dec.retired[1]; !ok { t.Fatal("time-expired block did not move to compact state") }
    st := dec.PressureStats()
    if st.RecoveryHorizonMillis != 5000 || st.TimeHorizonRetireEvents != 1 || st.GenerationHorizonRetireEvents != 0 || st.HorizonRetireEvents != 1 {
        t.Fatalf("time-horizon stats=%#v", st)
    }

    for _, missing := range []int{DataShards - 2, DataShards - 1} {
        packets, done, err := dec.Add(sources[missing])
        if err != nil { t.Fatalf("late source %d: %v", missing, err) }
        if len(packets) != 1 || !bytes.Equal(packets[0], want[missing]) {
            t.Fatalf("late source %d delivered=%d", missing, len(packets))
        }
        if missing == DataShards-1 && !done { t.Fatal("compact block did not finish after final late source") }
    }
    if packets, done, err := dec.Add(sources[DataShards-1]); err != nil || done || len(packets) != 0 {
        t.Fatalf("late duplicate done=%v delivered=%d err=%v", done, len(packets), err)
    }
}

func TestBlockDecoderGenerationReasonRemainsDistinct(t *testing.T) {
    dec, err := NewBlockDecoder(NewReedSolomon20x20(), 1400, 640)
    if err != nil { t.Fatal(err) }
    _, _ = makeDualHorizonZombie(t, dec, 10)
    b := dec.blocks[10]
    if b == nil { t.Fatal("zombie block missing") }
    b.firstAt = time.Now()
    dec.latestBlockID = 10 + heavyRecoveryHorizonBlocks
    dec.haveLatestBlockID = true
    dec.Expire(time.Now())
    st := dec.PressureStats()
    if st.GenerationHorizonRetireEvents != 1 || st.TimeHorizonRetireEvents != 0 || st.HorizonRetireEvents != 1 {
        t.Fatalf("generation-horizon stats=%#v", st)
    }
}
''')
    print("WBD_DIAGNOSTIC_PATCH fec_heavy_dual_horizon generation_blocks=64 wall_clock_ms=5000 no_packet_expire=flushdue late_systematic=first_delivery reason_counters=exclusive behavior_change=test_only")


if __name__ == "__main__":
    main()
