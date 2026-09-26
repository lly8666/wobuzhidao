package fec

import "fmt"

// ReedSolomon20xR generalizes the already-qualified systematic 20+20 matrix to
// a fixed K=20 family with 1..20 parity shards. Parity rows are the first R
// parity rows of the same systematic generator used by ReedSolomon20x20, so the
// 20:20 profile remains wire/math compatible at the codec level.
//
// This type establishes correctness for the fixed preset family. The existing
// FastReedSolomon20x20 remains the qualified hot implementation until each
// smaller-R profile gets its own performance qualification.
type ReedSolomon20xR struct {
	parity    int
	generator [TotalShards][DataShards]byte
}

func NewReedSolomon20xR(parity int) (*ReedSolomon20xR, error) {
	if parity <= 0 || parity > ParityShards {
		return nil, fmt.Errorf("%w: parity=%d want 1..%d", ErrInvalidShardSet, parity, ParityShards)
	}
	return &ReedSolomon20xR{parity: parity, generator: buildGenerator()}, nil
}

func (r *ReedSolomon20xR) ParityShards() int { return r.parity }
func (r *ReedSolomon20xR) TotalShards() int  { return DataShards + r.parity }

func (r *ReedSolomon20xR) Encode(shards [][]byte) error {
	shardSize, err := validateProfileShards(shards, r.TotalShards())
	if err != nil {
		return err
	}
	for p := 0; p < r.parity; p++ {
		idx := DataShards + p
		out := shards[idx]
		clear(out)
		row := r.generator[idx]
		for d := 0; d < DataShards; d++ {
			coef := row[d]
			if coef == 0 {
				continue
			}
			in := shards[d]
			for i := 0; i < shardSize; i++ {
				out[i] ^= gfMul(coef, in[i])
			}
		}
	}
	return nil
}

func (r *ReedSolomon20xR) Reconstruct(shards [][]byte, present []bool) error {
	total := r.TotalShards()
	shardSize, err := validateProfileShards(shards, total)
	if err != nil {
		return err
	}
	if len(present) != total {
		return fmt.Errorf("%w: present=%d want=%d", ErrInvalidShardSet, len(present), total)
	}
	available := 0
	for _, ok := range present {
		if ok {
			available++
		}
	}
	if available < DataShards {
		return ErrTooManyMissing
	}
	if available == total {
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
		return fmt.Errorf("fec: invert 20x%d decode matrix: %w", r.parity, err)
	}

	// Product delivery only needs missing systematic data, but regenerating the
	// admitted parity set keeps this Codec contract compatible with the original
	// reference implementation and simplifies deterministic qualification.
	for d := 0; d < DataShards; d++ {
		if present[d] {
			continue
		}
		out := shards[d]
		clear(out)
		row := inverse[d]
		for s, idx := range selected {
			coef := row[s]
			if coef == 0 {
				continue
			}
			in := shards[idx]
			for i := 0; i < shardSize; i++ {
				out[i] ^= gfMul(coef, in[i])
			}
		}
		present[d] = true
	}
	for idx := DataShards; idx < total; idx++ {
		if present[idx] {
			continue
		}
		out := shards[idx]
		clear(out)
		row := r.generator[idx]
		for d := 0; d < DataShards; d++ {
			coef := row[d]
			if coef == 0 {
				continue
			}
			in := shards[d]
			for i := 0; i < shardSize; i++ {
				out[i] ^= gfMul(coef, in[i])
			}
		}
		present[idx] = true
	}
	return nil
}

func validateProfileShards(shards [][]byte, total int) (int, error) {
	if total < DataShards || total > TotalShards || len(shards) != total {
		return 0, fmt.Errorf("%w: shards=%d want=%d", ErrInvalidShardSet, len(shards), total)
	}
	if len(shards) == 0 || len(shards[0]) == 0 {
		return 0, fmt.Errorf("%w: zero shard size", ErrInvalidShardSet)
	}
	sz := len(shards[0])
	for i, shard := range shards {
		if len(shard) != sz {
			return 0, fmt.Errorf("%w: shard %d size=%d want=%d", ErrInvalidShardSet, i, len(shard), sz)
		}
	}
	return sz, nil
}
