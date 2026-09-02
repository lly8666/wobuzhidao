package fec

import "sync"

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

func xorMul(out, in []byte, coef byte) {
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
	_, err := validateShards(shards)
	if err != nil {
		return err
	}
	for p := 0; p < ParityShards; p++ {
		out := shards[DataShards+p]
		clear(out)
		row := r.generator[DataShards+p]
		for d := 0; d < DataShards; d++ {
			xorMul(out, shards[d], row[d])
		}
	}
	return nil
}

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
