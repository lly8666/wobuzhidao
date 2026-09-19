package fec

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
