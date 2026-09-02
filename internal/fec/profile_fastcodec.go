package fec

// FastReedSolomon20xR is the variable-parity counterpart of
// FastReedSolomon20x20. It uses the same systematic generator and shared 64 KiB
// GF multiply table, emits only the selected first R parity rows, and restores
// only missing systematic shards. This is the intended hot codec for the fixed
// 20:4/8/12/16 profiles once their wire path is admitted.
type FastReedSolomon20xR struct {
	parity    int
	generator [TotalShards][DataShards]byte
}

func NewFastReedSolomon20xR(parity int) (*FastReedSolomon20xR, error) {
	if _, err := NewReedSolomon20xR(parity); err != nil {
		return nil, err
	}
	fastMulOnce.Do(initFastMul)
	return &FastReedSolomon20xR{parity: parity, generator: buildGenerator()}, nil
}

func (r *FastReedSolomon20xR) ParityShards() int { return r.parity }
func (r *FastReedSolomon20xR) TotalShards() int  { return DataShards + r.parity }

func (r *FastReedSolomon20xR) Encode(shards [][]byte) error {
	if _, err := validateProfileShards(shards, r.TotalShards()); err != nil {
		return err
	}
	for p := 0; p < r.parity; p++ {
		idx := DataShards + p
		out := shards[idx]
		clear(out)
		row := r.generator[idx]
		for d := 0; d < DataShards; d++ {
			xorMul(out, shards[d], row[d])
		}
	}
	return nil
}

func (r *FastReedSolomon20xR) Reconstruct(shards [][]byte, present []bool) error {
	total := r.TotalShards()
	if _, err := validateProfileShards(shards, total); err != nil {
		return err
	}
	if len(present) != total {
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
