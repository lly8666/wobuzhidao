#!/usr/bin/env python3
import hashlib
import sys
from pathlib import Path

BLOCK_BLOB = "427b2b2efef3f4118b588478cd2f0278c38eb1e1"
OBSERVE_BLOB = "223878adbed0b8f1018c5950cdbe1add63da2e74"


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
        raise SystemExit("usage: instrument_fec_time_horizon5s_diag.py PRODUCT_DIR")
    root = Path(sys.argv[1])
    block_path = root / "internal/fec/block.go"
    observe_path = root / "internal/fec/decoder_observe.go"

    block_raw = block_path.read_bytes()
    observe_raw = observe_path.read_bytes()
    if blob_sha(block_raw) != BLOCK_BLOB:
        raise SystemExit(f"unexpected block.go blob {blob_sha(block_raw)}, want {BLOCK_BLOB}")
    if blob_sha(observe_raw) != OBSERVE_BLOB:
        raise SystemExit(f"unexpected decoder_observe.go blob {blob_sha(observe_raw)}, want {OBSERVE_BLOB}")

    block = block_raw.decode()
    block = replace_once(
        block,
        "const heavyRecoveryHorizonBlocks uint32 = 64\n",
        "const heavyRecoveryHorizonBlocks uint32 = 64\n\n"
        "// A generation horizon alone is not a wall-clock bound when block-ID\n"
        "// progression stalls under overload. Five seconds is intentionally\n"
        "// conservative for the 300ms one-way validation profile: it is over\n"
        "// eight base RTTs and above the ~1.75s oldest-heavy age seen in the\n"
        "// healthy 5M/30% recovery sample. This remains a test candidate until\n"
        "// the A/B runs justify a production value.\n"
        "const heavyRecoveryHorizonTime time.Duration = 5 * time.Second\n",
        "add time horizon constant",
    )
    block = replace_once(
        block,
        "\thorizonRetireEvents      uint64\n\thorizonRetiredIncomplete uint64\n",
        "\thorizonRetireEvents      uint64\n\thorizonRetiredIncomplete uint64\n\ttimeHorizonRetireEvents  uint64\n",
        "add time-horizon counter",
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
\t// Run expiry on every arrival, not only when BlockID advances. Otherwise a
\t// stalled stream can leave an old heavy block resident indefinitely in wall
\t// time even though packets continue to arrive for the current generation.
\td.retireExpiredHeavy(now)
}
''',
        "observeBlockID",
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

func (d *BlockDecoder) heavyRecoveryExpired(id uint32, b *decodeBlock, now time.Time) bool {
\treturn d.blockPastRecoveryHorizon(id) || blockPastRecoveryTimeHorizon(b, now)
}

func (d *BlockDecoder) retireExpiredHeavy(now time.Time) {
\tfor _, id := range d.blockOrder[d.blockHead:] {
\t\tb := d.blocks[id]
\t\tif b == nil || !d.heavyRecoveryExpired(id, b, now) {
\t\t\tcontinue
\t\t}
\t\td.retireBlockAfterHorizon(id, b, now)
\t}
\td.compactBlockOrder()
}
''',
        "retireExpiredHeavy",
    )
    block = replace_once(
        block,
        "\td.observeBlockID(h.BlockID)\n",
        "\tnow := time.Now()\n\td.observeBlockID(h.BlockID, now)\n",
        "Add observe call",
    )
    block = replace_once(
        block,
        "\t\tb = &decodeBlock{firstAt: time.Now()}\n",
        "\t\tb = &decodeBlock{firstAt: now}\n",
        "decodeBlock firstAt",
    )
    block = replace_once(
        block,
        '''func (d *BlockDecoder) retireBlockAfterHorizon(id uint32, b *decodeBlock) {
\tif b == nil || !d.blockPastRecoveryHorizon(id) {
\t\treturn
\t}
\tincomplete := !allDataDelivered(b)
\tdelete(d.blocks, id)
\tr := retiredStateFromBlock(b)
\td.addRetiredState(id, r)
\td.horizonRetireEvents++
\tif incomplete {
\t\td.horizonRetiredIncomplete++
\t}
\tif retiredAllDelivered(r) {
\t\td.markCompleted(id)
\t}
}
''',
        '''func (d *BlockDecoder) retireBlockAfterHorizon(id uint32, b *decodeBlock, now time.Time) {
\tif b == nil || !d.heavyRecoveryExpired(id, b, now) {
\t\treturn
\t}
\texpiredByTime := blockPastRecoveryTimeHorizon(b, now)
\tincomplete := !allDataDelivered(b)
\tdelete(d.blocks, id)
\tr := retiredStateFromBlock(b)
\td.addRetiredState(id, r)
\td.horizonRetireEvents++
\tif expiredByTime {
\t\td.timeHorizonRetireEvents++
\t}
\tif incomplete {
\t\td.horizonRetiredIncomplete++
\t}
\tif retiredAllDelivered(r) {
\t\td.markCompleted(id)
\t}
}
''',
        "retireBlockAfterHorizon",
    )

    observe = observe_raw.decode()
    observe = replace_once(
        observe,
        '\tRecoveryHorizonBlocks    uint32 `json:"recovery_horizon_blocks"`\n',
        '\tRecoveryHorizonBlocks    uint32 `json:"recovery_horizon_blocks"`\n'
        '\tRecoveryHorizonMillis    int64  `json:"recovery_horizon_ms"`\n',
        "observe horizon ms field",
    )
    observe = replace_once(
        observe,
        '\tHorizonRetiredIncomplete uint64 `json:"horizon_retired_incomplete"`\n',
        '\tHorizonRetiredIncomplete uint64 `json:"horizon_retired_incomplete"`\n'
        '\tTimeHorizonRetireEvents  uint64 `json:"time_horizon_retire_events"`\n',
        "observe time event field",
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
        "\t\tHorizonRetiredIncomplete: d.horizonRetiredIncomplete,\n"
        "\t\tTimeHorizonRetireEvents:  d.timeHorizonRetireEvents,\n",
        "observe time event value",
    )

    block_path.write_text(block)
    observe_path.write_text(observe)

    test_path = root / "internal/fec/block_time_horizon_test.go"
    test_path.write_text(r'''package fec

import (
    "bytes"
    "testing"
    "time"
)

func makeTimeHorizonZombie(t *testing.T, dec *BlockDecoder, blockID uint32) ([][]byte, [][]byte) {
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

func TestBlockDecoderTimeHorizonRetiresWithoutBlockIDAdvance(t *testing.T) {
    dec, err := NewBlockDecoder(NewReedSolomon20x20(), 1400, 640)
    if err != nil {
        t.Fatal(err)
    }
    want, sources := makeTimeHorizonZombie(t, dec, 1)
    b := dec.blocks[1]
    if b == nil {
        t.Fatal("zombie block missing")
    }
    b.firstAt = time.Now().Add(-heavyRecoveryHorizonTime - time.Millisecond)

    // A duplicate from the same BlockID must still run wall-clock expiry; the
    // test intentionally does not advance latestBlockID.
    packets, done, err := dec.Add(sources[0])
    if err != nil || done || len(packets) != 0 {
        t.Fatalf("duplicate trigger done=%v delivered=%d err=%v", done, len(packets), err)
    }
    if dec.blocks[1] != nil {
        t.Fatal("time-expired block still occupies heavy state")
    }
    if _, ok := dec.retired[1]; !ok {
        t.Fatal("time-expired block did not move to compact state")
    }
    st := dec.PressureStats()
    if st.RecoveryHorizonMillis != 5000 || st.TimeHorizonRetireEvents != 1 || st.HorizonRetireEvents != 1 {
        t.Fatalf("time-horizon stats=%#v", st)
    }

    for _, missing := range []int{DataShards - 2, DataShards - 1} {
        packets, done, err := dec.Add(sources[missing])
        if err != nil {
            t.Fatalf("late source %d: %v", missing, err)
        }
        if len(packets) != 1 || !bytes.Equal(packets[0], want[missing]) {
            t.Fatalf("late source %d delivered=%d", missing, len(packets))
        }
        if missing == DataShards-1 && !done {
            t.Fatal("compact block did not finish after final late source")
        }
    }
    if packets, done, err := dec.Add(sources[DataShards-1]); err != nil || done || len(packets) != 0 {
        t.Fatalf("late duplicate done=%v delivered=%d err=%v", done, len(packets), err)
    }
}

func TestBlockDecoderTimeHorizonKeepsYoungHeavy(t *testing.T) {
    dec, err := NewBlockDecoder(NewReedSolomon20x20(), 1400, 640)
    if err != nil {
        t.Fatal(err)
    }
    _, sources := makeTimeHorizonZombie(t, dec, 7)
    b := dec.blocks[7]
    b.firstAt = time.Now().Add(-heavyRecoveryHorizonTime + time.Second)
    if _, _, err := dec.Add(sources[0]); err != nil {
        t.Fatal(err)
    }
    if dec.blocks[7] == nil {
        t.Fatal("young block retired before wall-clock horizon")
    }
    if st := dec.PressureStats(); st.TimeHorizonRetireEvents != 0 {
        t.Fatalf("unexpected time retirement stats=%#v", st)
    }
}
''')
    print("WBD_DIAGNOSTIC_PATCH fec_heavy_time_horizon_ms=5000 generation_horizon_blocks=64 late_systematic=first_delivery behavior_change=test_only")


if __name__ == "__main__":
    main()
