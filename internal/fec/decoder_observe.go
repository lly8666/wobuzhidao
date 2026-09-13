package fec

import "time"

// DecoderPressureStats is a read-only snapshot for diagnostics. It does not
// change decoder admission, retirement, reconstruction, or wire behavior.
type DecoderPressureStats struct {
	InFlight                  int    `json:"in_flight"`
	MaxBlocks                 int    `json:"max_blocks"`
	Retired                   int    `json:"retired"`
	RetiredIncomplete         int    `json:"retired_incomplete"`
	RetiredMissingSources     int    `json:"retired_missing_sources"`
	ActiveFinal               int    `json:"active_final"`
	ActiveProvisional         int    `json:"active_provisional"`
	ActiveMissingSources      int    `json:"active_missing_sources"`
	RecoveryHorizonBlocks     uint32 `json:"recovery_horizon_blocks"`
	NewestBlockID             uint32 `json:"newest_block_id"`
	OldestActiveAgeBlocks     uint32 `json:"oldest_active_age_blocks"`
	OldestActiveAgeMillis     int64  `json:"oldest_active_age_ms"`
	HorizonRetireEvents       uint64 `json:"horizon_retire_events"`
	HorizonRetiredIncomplete uint64 `json:"horizon_retired_incomplete"`
}

func (d *BlockDecoder) PressureStats() DecoderPressureStats {
	s := DecoderPressureStats{
		InFlight:                  len(d.blocks),
		MaxBlocks:                 d.maxBlocks,
		Retired:                   len(d.retired),
		RecoveryHorizonBlocks:     heavyRecoveryHorizonBlocks,
		NewestBlockID:             d.latestBlockID,
		HorizonRetireEvents:       d.horizonRetireEvents,
		HorizonRetiredIncomplete: d.horizonRetiredIncomplete,
	}
	now := time.Now()
	for id, b := range d.blocks {
		if b == nil {
			continue
		}
		if age, ok := blockIDAge(d.latestBlockID, id); d.haveLatestBlockID && ok && age > s.OldestActiveAgeBlocks {
			s.OldestActiveAgeBlocks = age
		}
		if !b.firstAt.IsZero() {
			ageMS := now.Sub(b.firstAt).Milliseconds()
			if ageMS > s.OldestActiveAgeMillis {
				s.OldestActiveAgeMillis = ageMS
			}
		}
		count := DataShards
		if b.final {
			s.ActiveFinal++
			count = int(b.header.DataCount)
		} else {
			s.ActiveProvisional++
		}
		for i := 0; i < count; i++ {
			if !b.delivered[i] {
				s.ActiveMissingSources++
			}
		}
	}
	for _, r := range d.retired {
		count := DataShards
		if r.final {
			count = int(r.header.DataCount)
		}
		missing := 0
		for i := 0; i < count; i++ {
			if r.delivered&(uint32(1)<<uint(i)) == 0 {
				missing++
			}
		}
		if missing != 0 {
			s.RetiredIncomplete++
			s.RetiredMissingSources += missing
		}
	}
	return s
}
