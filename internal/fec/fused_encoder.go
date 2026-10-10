package fec

import (
	"fmt"
	"sync"

	"github.com/klauspost/reedsolomon"
)

// Separate 20:P encoder instances: passing all 20 parity matrix rows to a
// smaller P instance would corrupt/overrun the encoder's matrix allocation.
// The cached instances own only immutable matrices and synchronized pool
// workspace. No per-block New(), goroutine, timer or inversion-cache growth.
type fusedEncoderSlot struct {
	once sync.Once
	encoder reedsolomon.Encoder
	err error
}
var fusedEncoders [ParityShards+1]fusedEncoderSlot

func (r *FastReedSolomon20x20) encodeFullFused(shards [][]byte, parityCount int) error {
	if !validParityCount(parityCount) || len(shards) != TotalShards {
		return ErrInvalidShardSet
	}
	slot := &fusedEncoders[parityCount]
	slot.once.Do(func() {
		matrix := make([][]byte, parityCount)
		for p := 0; p < parityCount; p++ {
			// WBD's generator (not the dependency's default matrix) defines
			// the exact v1 wire parity coefficients for every 20:P profile.
			matrix[p] = append([]byte(nil), r.generator[DataShards+p][:]...)
		}
		slot.encoder, slot.err = reedsolomon.New(
			DataShards, parityCount,
			reedsolomon.WithCustomMatrix(matrix),
			reedsolomon.WithMaxGoroutines(1),
			reedsolomon.WithInversionCache(false),
		)
	})
	if slot.err != nil {
		return fmt.Errorf("fec: custom fused 20:%d: %w", parityCount, slot.err)
	}
	return slot.encoder.Encode(shards[:DataShards+parityCount])
}
