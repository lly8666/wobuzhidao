package fec

// Compact the oldest incomplete block only when it precedes incoming work.
// The window is bounded; blockOrder avoids scanning the historical map IDs.
func (d *BlockDecoder) retireOldestRecoveryBefore(incoming uint32) bool {
	for i := d.blockHead; i < len(d.blockOrder); i++ {
		id := d.blockOrder[i]
		if d.blocks[id] == nil {
			if i == d.blockHead {
				d.blockHead++
			}
			continue
		}
		if id >= incoming {
			continue
		}
		if d.RetireRecoveryExpired(id).Retired {
			d.pressureRetirements++
			d.lastPressureRetired = id
			return true
		}
	}
	d.compactBlockOrder()
	return false
}

// LastPressureRetiredBlock returns the single heavy block retired by the most
// recent AddLive call, so the owner can remove its absolute deadline as well.
func (d *BlockDecoder) LastPressureRetiredBlock() uint32 {
	return d.lastPressureRetired
}

// RecoveryRetireResult describes one heavy decoder block compacted because its
// external recovery deadline expired. The compact retired state keeps exact
// first-delivery information but intentionally drops parity/reconstruction
// payloads so the heavy memory is released.
type RecoveryRetireResult struct {
	Retired        bool
	Final          bool
	MissingSources int
}

// IsHeavyBlock reports whether id still owns a full reconstruction slot.
func (d *BlockDecoder) IsHeavyBlock(id uint32) bool {
	if d == nil {
		return false
	}
	return d.blocks[id] != nil
}

// RetireRecoveryExpired moves an incomplete heavy block into the existing
// compact retired state after the caller's bounded recovery deadline expires.
// Unlike retireBlock, this transition is allowed while source delivery is
// incomplete: parity recovery is abandoned after the deadline, but a late
// systematic source can still be first-delivered by addRetired and duplicates
// remain suppressed.
func (d *BlockDecoder) RetireRecoveryExpired(id uint32) RecoveryRetireResult {
	if d == nil {
		return RecoveryRetireResult{}
	}
	b := d.blocks[id]
	if b == nil {
		return RecoveryRetireResult{}
	}

	count := DataShards
	if b.final {
		count = int(b.header.DataCount)
	}
	missing := 0
	for i := 0; i < count; i++ {
		if !b.delivered[i] {
			missing++
		}
	}

	delete(d.blocks, id)
	r := retiredBlock{header: b.header, final: b.final}
	for i, delivered := range b.delivered {
		if delivered {
			r.delivered |= uint32(1) << uint(i)
		}
		if !b.final && b.sourcePresent[i] {
			r.header.OriginalLengths[i] = uint16(len(b.sources[i]))
		}
	}
	d.addRetiredState(id, r)
	d.compactBlockOrder()
	if retiredAllDelivered(r) {
		d.markCompleted(id)
	}
	return RecoveryRetireResult{Retired: true, Final: b.final, MissingSources: missing}
}
