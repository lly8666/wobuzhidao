package datapath

import "errors"

const PaddingRatioScalePPM uint32 = 1_000_000

var (
	ErrPaddingPolicy       = errors.New("datapath: invalid tunnel padding policy")
	ErrPaddingPolicyLocked = errors.New("datapath: tunnel padding policy is locked after transport attach")
)

// TunnelPaddingPolicy is the optional production policy layered over the P3
// record-padding mechanism. The zero value is intentionally disabled.
//
// When enabled, every eligible already-formed record requests exactly
// BytesPerRecord encrypted padding bytes. BytesPerRecord is therefore also the
// hard per-record cap. The request is accepted only when both cumulative
// tunnel limits still permit the full amount; otherwise that record gets zero
// immediately. No waiting, token refill, batching or partial padding exists.
type TunnelPaddingPolicy struct {
	Enabled bool

	BytesPerRecord     int
	MaxPaddingBytes    uint64
	MaxPaddingRatioPPM uint32
}

func (p TunnelPaddingPolicy) Validate() error {
	if !p.Enabled {
		if p.BytesPerRecord != 0 || p.MaxPaddingBytes != 0 || p.MaxPaddingRatioPPM != 0 {
			return ErrPaddingPolicy
		}
		return nil
	}
	if p.BytesPerRecord <= 0 || p.MaxPaddingBytes == 0 ||
		p.MaxPaddingRatioPPM == 0 || p.MaxPaddingRatioPPM > PaddingRatioScalePPM {
		return ErrPaddingPolicy
	}
	return nil
}

type TunnelPaddingStats struct {
	Enabled            bool
	BytesPerRecord     int
	MaxPaddingBytes    uint64
	MaxPaddingRatioPPM uint32

	LogicalPayloads    uint64
	UsefulPayloadBytes uint64
	RecordRequests     uint64
	PaddedRecords      uint64
	PaddingBytes       uint64
	BudgetSkips        uint64
	HeadroomSkips      uint64
}

type tunnelPaddingState struct {
	policy TunnelPaddingPolicy

	logicalPayloads    uint64
	usefulPayloadBytes uint64
	recordRequests     uint64
	paddedRecords      uint64
	paddingBytes       uint64
	reservedBytes      uint64
	budgetSkips        uint64
	headroomSkips      uint64
}

// ConfigurePadding installs the immutable production policy before the first
// transport incarnation is attached. This prevents a replacement or DORMANT /
// wake cycle from resetting cumulative budget state.
func (o *TunnelOwner) ConfigurePadding(policy TunnelPaddingPolicy) error {
	if o == nil {
		return ErrTunnelOwnerClosed
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrTunnelOwnerClosed
	}
	if o.physicalLocked() != 0 || o.identified || o.game != nil ||
		o.padding.logicalPayloads != 0 || o.padding.usefulPayloadBytes != 0 ||
		o.padding.paddingBytes != 0 || o.padding.reservedBytes != 0 {
		return ErrPaddingPolicyLocked
	}
	o.padding.policy = policy
	return nil
}

func (o *TunnelOwner) PaddingPolicy() TunnelPaddingPolicy {
	if o == nil {
		return TunnelPaddingPolicy{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.padding.policy
}

func (o *TunnelOwner) paddingStatsLocked() TunnelPaddingStats {
	p := o.padding
	return TunnelPaddingStats{
		Enabled:            p.policy.Enabled,
		BytesPerRecord:     p.policy.BytesPerRecord,
		MaxPaddingBytes:    p.policy.MaxPaddingBytes,
		MaxPaddingRatioPPM: p.policy.MaxPaddingRatioPPM,
		LogicalPayloads:    p.logicalPayloads,
		UsefulPayloadBytes: p.usefulPayloadBytes,
		RecordRequests:     p.recordRequests,
		PaddedRecords:      p.paddedRecords,
		PaddingBytes:       p.paddingBytes,
		BudgetSkips:        p.budgetSkips,
		HeadroomSkips:      p.headroomSkips,
	}
}

// paddingSelectorForPayload counts one source-valid logical business payload
// once, then returns the shared tunnel allocator used by every resulting record.
// Game callers invoke the locked helper once before lane fan-out.
func (o *TunnelOwner) paddingSelectorForPayload(payloadBytes int) paddingSelector {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.paddingSelectorForPayloadLocked(payloadBytes)
}

func (o *TunnelOwner) paddingSelectorForPayloadLocked(payloadBytes int) paddingSelector {
	if !o.padding.policy.Enabled {
		return nil
	}
	o.padding.logicalPayloads++
	add := uint64(0)
	if payloadBytes > 0 {
		add = uint64(payloadBytes)
	}
	if ^uint64(0)-o.padding.usefulPayloadBytes < add {
		o.padding.usefulPayloadBytes = ^uint64(0)
	} else {
		o.padding.usefulPayloadBytes += add
	}
	return o.allocateRecordPadding
}

func (o *TunnelOwner) allocateRecordPadding(headroom int) (paddingSelection, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || !o.padding.policy.Enabled {
		return paddingSelection{}, nil
	}

	policy := o.padding.policy
	requested := policy.BytesPerRecord
	o.padding.recordRequests++
	if headroom < requested {
		o.padding.headroomSkips++
		return paddingSelection{requested: requested, skipped: true}, nil
	}

	limit := ratioPaddingLimit(o.padding.usefulPayloadBytes, policy.MaxPaddingRatioPPM)
	if policy.MaxPaddingBytes < limit {
		limit = policy.MaxPaddingBytes
	}
	used := o.padding.paddingBytes + o.padding.reservedBytes
	if used > limit || uint64(requested) > limit-used {
		o.padding.budgetSkips++
		return paddingSelection{requested: requested, skipped: true}, nil
	}

	o.padding.reservedBytes += uint64(requested)
	return paddingSelection{
		requested: requested,
		bytes:     requested,
		finish: func(success bool) {
			o.finishPaddingReservation(uint64(requested), success)
		},
	}, nil
}

func (o *TunnelOwner) finishPaddingReservation(n uint64, success bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.padding.reservedBytes >= n {
		o.padding.reservedBytes -= n
	} else {
		o.padding.reservedBytes = 0
	}
	if success {
		if ^uint64(0)-o.padding.paddingBytes < n {
			o.padding.paddingBytes = ^uint64(0)
		} else {
			o.padding.paddingBytes += n
		}
		o.padding.paddedRecords++
	}
}

func ratioPaddingLimit(useful uint64, ppm uint32) uint64 {
	if useful == 0 || ppm == 0 {
		return 0
	}
	scale := uint64(PaddingRatioScalePPM)
	ratio := uint64(ppm)
	return (useful/scale)*ratio + ((useful%scale)*ratio)/scale
}
