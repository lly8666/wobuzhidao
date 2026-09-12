#!/usr/bin/env bash
set -euo pipefail
PRODUCT_DIR=${1:?product checkout required}
: "${PROBE_REORDER:?PROBE_REORDER=0 or 1 required}"
[[ "$PROBE_REORDER" == 0 || "$PROBE_REORDER" == 1 ]] || { echo "bad PROBE_REORDER" >&2; exit 2; }
HELPER_DIR=${WBD_HELPER_DIR:-${GITHUB_WORKSPACE:?}/helper}
python3 "$HELPER_DIR/.github/scripts/apply_test_fec_horizon64.py" "$PRODUCT_DIR"
gofmt -w "$PRODUCT_DIR/internal/fec/block.go" "$PRODUCT_DIR/internal/fec/recovery_horizon_test.go"
cat > "$PRODUCT_DIR/internal/fec/diagnostic_iid30_probe_test.go" <<'EOF'
package fec

import (
    "encoding/binary"
    "math/rand"
    "testing"
    "time"
)

type diagnosticWire struct {
    block uint32
    wire  []byte
}

func TestDiagnosticIID30BlockProbe(t *testing.T) {
    const blocks = 1200
    const loss = 0.30
    const maxPacket = 1400

    reorder := false
    if v := getenvDiagnostic("WBD_PROBE_REORDER"); v == "1" {
        reorder = true
    }

    codec, err := NewCodec(DataShards, ParityShards)
    if err != nil { t.Fatal(err) }
    enc, err := NewFastBlockEncoder(codec, maxPacket, 32*time.Millisecond, 1)
    if err != nil { t.Fatal(err) }
    dec, err := NewBlockDecoder(codec, maxPacket, 64)
    if err != nil { t.Fatal(err) }

    rng := rand.New(rand.NewSource(20260913))
    expected := make(map[string]struct{}, blocks*DataShards)
    delivered := make(map[string]int, blocks*DataShards)
    retainedPerBlock := make([]int, blocks+1)
    retained := make([]diagnosticWire, 0, blocks*TotalShards)

    now := time.Unix(1700000000, 0)
    for b := 1; b <= blocks; b++ {
        for i := 0; i < DataShards; i++ {
            // Exercise mixed payload sizes while keeping every packet uniquely identifiable.
            n := 64 + ((b*97 + i*53) % 1200)
            p := make([]byte, n)
            binary.BigEndian.PutUint32(p[0:4], uint32(b))
            binary.BigEndian.PutUint16(p[4:6], uint16(i))
            for j := 6; j < len(p); j++ { p[j] = byte((b + i + j) % 251) }
            expected[string(p)] = struct{}{}
            wires, err := enc.Add(p, now)
            if err != nil { t.Fatalf("encode block=%d source=%d: %v", b, i, err) }
            now = now.Add(time.Microsecond)
            for _, w := range wires {
                copyWire := append([]byte(nil), w...)
                h, err := ParseBlockHeader(copyWire[:HeaderSize])
                if err != nil { t.Fatal(err) }
                if rng.Float64() >= loss {
                    retained = append(retained, diagnosticWire{block: h.BlockID, wire: copyWire})
                    retainedPerBlock[int(h.BlockID)]++
                }
            }
        }
    }

    if enc.Pending() != 0 { t.Fatalf("encoder pending=%d after full blocks", enc.Pending()) }

    if reorder {
        // Shuffle only within eight-generation windows: enough to stress generation lifetime
        // and metadata arrival order without inventing an unrealistic 64+ generation delay.
        for start := uint32(1); start <= blocks; start += 8 {
            end := start + 8
            lo := -1
            hi := -1
            for i := range retained {
                if retained[i].block >= start && retained[i].block < end {
                    if lo < 0 { lo = i }
                    hi = i + 1
                }
            }
            if lo >= 0 {
                rng.Shuffle(hi-lo, func(i, j int) { retained[lo+i], retained[lo+j] = retained[lo+j], retained[lo+i] })
            }
        }
    }

    decoderErrors := 0
    duplicates := 0
    unknown := 0
    for _, dw := range retained {
        out, _, err := dec.Add(dw.wire)
        if err != nil {
            decoderErrors++
            continue
        }
        for _, p := range out {
            k := string(p)
            if _, ok := expected[k]; !ok {
                unknown++
                continue
            }
            delivered[k]++
            if delivered[k] > 1 { duplicates++ }
        }
    }

    missing := 0
    decodableBlocks := 0
    decodableMissing := 0
    for b := 1; b <= blocks; b++ {
        decodable := retainedPerBlock[b] >= DataShards
        if decodable { decodableBlocks++ }
        for i := 0; i < DataShards; i++ {
            n := 64 + ((b*97 + i*53) % 1200)
            p := make([]byte, n)
            binary.BigEndian.PutUint32(p[0:4], uint32(b))
            binary.BigEndian.PutUint16(p[4:6], uint16(i))
            for j := 6; j < len(p); j++ { p[j] = byte((b + i + j) % 251) }
            if delivered[string(p)] == 0 {
                missing++
                if decodable { decodableMissing++ }
            }
        }
    }

    total := blocks * DataShards
    t.Logf("WBD_FEC_BLOCK_PROBE reorder=%t blocks=%d total=%d delivered=%d missing=%d loss=%.8f decodable_blocks=%d decodable_missing=%d duplicates=%d unknown=%d decoder_errors=%d in_flight=%d", reorder, blocks, total, total-missing, missing, float64(missing)/float64(total), decodableBlocks, decodableMissing, duplicates, unknown, decoderErrors, dec.InFlight())
    if decodableMissing != 0 || duplicates != 0 || unknown != 0 || decoderErrors != 0 {
        t.Fatalf("FEC invariant failed: decodable_missing=%d duplicates=%d unknown=%d decoder_errors=%d", decodableMissing, duplicates, unknown, decoderErrors)
    }
}
EOF
# Add a tiny package-local env helper without importing os into the diagnostic test body generator.
python3 - "$PRODUCT_DIR/internal/fec/diagnostic_iid30_probe_test.go" <<'PY'
from pathlib import Path
p=Path(__import__('sys').argv[1]); s=p.read_text()
s=s.replace('"math/rand"\n', '"math/rand"\n    "os"\n')
s=s.replace('\ntype diagnosticWire struct {', '\nfunc getenvDiagnostic(k string) string { return os.Getenv(k) }\n\ntype diagnosticWire struct {')
p.write_text(s)
PY
gofmt -w "$PRODUCT_DIR/internal/fec/diagnostic_iid30_probe_test.go"
cd "$PRODUCT_DIR"
WBD_PROBE_REORDER="$PROBE_REORDER" go test ./internal/fec -run '^TestDiagnosticIID30BlockProbe$' -count=1 -v
