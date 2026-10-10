package fec

import (
	"sync"

	"github.com/klauspost/reedsolomon"
)

// FastReedSolomon20x20 is the transport-oriented 20+20 codec used by the WBD
// packet block layer. It keeps the same systematic generator and wire format as
// ReedSolomon20x20, but uses a 64 KiB GF(256) multiply table in the hot byte
// loops. Reconstruct restores all systematic data shards needed by the packet
// layer; missing parity is intentionally not regenerated because completed
// blocks are immediately delivered and discarded.
type FastReedSolomon20x20 struct {
	generator [TotalShards][DataShards]byte
}

var (
	fastMulOnce sync.Once
	fastMul     [256][256]byte
)

func initFastMul() {
	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			fastMul[a][b] = gfMul(byte(a), byte(b))
		}
	}
}

func NewFastReedSolomon20x20() *FastReedSolomon20x20 {
	fastMulOnce.Do(initFastMul)
	return &FastReedSolomon20x20{generator: buildGenerator()}
}

// lowLevel is immutable: v1.12.6 WithOptions does not persist options.
// Never call the library's non-vector fallback here: on unsupported CPUs it
// can lazily allocate a large coefficient-specific two-byte lookup table.
var lowLevel reedsolomon.LowLevel

// xorMul is shared by active parity encoding and missing-data recovery.
// Inputs and outputs are owned, same-length buffers; no partial overlap.
func xorMul(out, in []byte, coef byte) {
	if coef == 0 {
		return
	}
	if fecSIMDEnabled && len(in) >= 32 {
		lowLevel.GalMulSliceXor(coef, in, out)
		return
	}
	xorMulScalar(out, in, coef)
}

// Keep the prior 64KiB-table implementation for short spans, unsupported
// platforms/build tags, reference tests and measured fallback attribution.
func xorMulScalar(out, in []byte, coef byte) {
	if coef == 0 {
		return
	}
	if coef == 1 {
		for i, v := range in {
			out[i] ^= v
		}
		return
	}
	table := &fastMul[coef]
	for i, v := range in {
		out[i] ^= table[v]
	}
}

func (r *FastReedSolomon20x20) Encode(shards [][]byte) error {
	return r.EncodeActive(shards, DataShards, ParityShards)
}

// EncodeActive computes the leading parity rows for a shortened systematic
// block. Sources [dataCount,20) must be authoritative known-zero slots. The
// block encoder establishes that invariant before calling this method. Rows
// outside parityCount are neither computed nor used on the wire.
func (r *FastReedSolomon20x20) EncodeActive(shards [][]byte, dataCount, parityCount int) error {
	if dataCount < 1 || dataCount > DataShards || parityCount < 1 || parityCount > ParityShards {
		return ErrInvalidShardSet
	}
	_, err := validateShards(shards)
	if err != nil {
		return err
	}
	// Full 20-source blocks use native SIMD fused custom-matrix parity;
	// 1..19 active-source partials ALWAYS use the original span backend.
	// wbd_fec_span is the controlled opt-out for any real-business regression.
	if dataCount == DataShards && fecFusedEnabled && fecSIMDEnabled {
		return r.encodeFullFused(shards, parityCount)
	}
	for p := 0; p < parityCount; p++ {
		out := shards[DataShards+p]
		clear(out)
		row := r.generator[DataShards+p]
		for d := 0; d < dataCount; d++ {
			xorMul(out, shards[d], row[d])
		}
	}
	return nil
}

// IgnoresInactiveSources is a semantic capability (not a concrete-type
// shortcut). Both the span and fused full-block encoders avoid inactive slots.
// A new generic/active codec must opt in explicitly before skipping zero-fill.
func (r *FastReedSolomon20x20) IgnoresInactiveSources() bool { return true }

func (r *FastReedSolomon20x20) Reconstruct(shards [][]byte, present []bool) error {
	_, err := validateShards(shards)
	if err != nil {
		return err
	}
	if len(present) != TotalShards {
		return ErrInvalidShardSet
	}

	available := 0
	allDataPresent := true
	for i, ok := range present {
		if ok {
			available++
		}
		if i < DataShards && !ok {
			allDataPresent = false
		}
	}
	if available < DataShards {
		return ErrTooManyMissing
	}
	if allDataPresent {
		return nil
	}

	var selected [DataShards]int
	var decode [DataShards][DataShards]byte
	n := 0
	for i, ok := range present {
		if !ok {
			continue
		}
		selected[n] = i
		decode[n] = r.generator[i]
		n++
		if n == DataShards {
			break
		}
	}
	inverse, err := invertMatrix(decode)
	if err != nil {
		return err
	}

	for d := 0; d < DataShards; d++ {
		if present[d] {
			continue
		}
		out := shards[d]
		clear(out)
		row := inverse[d]
		for s, idx := range selected {
			xorMul(out, shards[idx], row[s])
		}
		present[d] = true
	}
	return nil
}
