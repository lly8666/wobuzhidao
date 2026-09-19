package linkdata

import (
	"fmt"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

const FECRecoveryHorizon = 3 * time.Second

type FECPathConfig struct {
	// SourceMTU is the maximum LINK fragment frame carried as one systematic
	// FEC source shard. The later unified MTU task is responsible for deriving
	// this value from TLS-like/FakeTCP/IP overhead.
	SourceMTU int

	// ParityShards is 0 for FEC off, or one of the archived live 20:R fixed
	// profiles returned by fec.SupportedParityShards().
	ParityShards int

	FlushAfter time.Duration
	MaxBlocks  int
}

type FECRecoveryStats struct {
	HorizonMillis         int64
	PendingDeadlines      int
	ExpireEvents          uint64
	ExpiredIncomplete     uint64
	ExpiredMissingSources uint64
}

type FECPathState struct {
	FECEnabled   bool
	ParityShards int
	Decoder      fec.DecoderPressureCounts
	Recovery     FECRecoveryStats
	Reassembly   ReassemblyState
}

type FECPath struct {
	config      FECPathConfig
	fragmenter  *Fragmenter
	reassembler *Reassembler
	encoder     *fec.FastBlockEncoder
	decoder     *fec.BlockDecoder
	recovery    *fecRecoveryTracker
}

func SupportedFECProfiles() []int {
	profiles := make([]int, 1, 1+len(fec.SupportedParityShards()))
	profiles[0] = 0
	return append(profiles, fec.SupportedParityShards()...)
}

func NewFECPath(config FECPathConfig) (*FECPath, error) {
	fragmenter, err := NewFragmenter(config.SourceMTU)
	if err != nil {
		return nil, err
	}
	reassembler, err := NewReassembler(config.SourceMTU)
	if err != nil {
		return nil, err
	}
	p := &FECPath{config: config, fragmenter: fragmenter, reassembler: reassembler}
	if config.ParityShards == 0 {
		return p, nil
	}
	if !fec.IsSupportedParityShards(config.ParityShards) {
		return nil, fmt.Errorf("linkdata: unsupported fixed FEC 20:%d", config.ParityShards)
	}
	if config.FlushAfter <= 0 || config.MaxBlocks <= 0 {
		return nil, fmt.Errorf("linkdata: invalid FEC runtime flush=%s max_blocks=%d", config.FlushAfter, config.MaxBlocks)
	}
	codec := fec.NewFastReedSolomon20x20()
	encoder, err := fec.NewFastBlockEncoderWithParity(codec, config.SourceMTU, config.FlushAfter, 1, config.ParityShards)
	if err != nil {
		return nil, err
	}
	decoder, err := fec.NewBlockDecoderWithParity(codec, config.SourceMTU, config.MaxBlocks, config.ParityShards)
	if err != nil {
		return nil, err
	}
	p.encoder = encoder
	p.decoder = decoder
	p.recovery = newFECRecoveryTracker(FECRecoveryHorizon)
	return p, nil
}

func (p *FECPath) Config() FECPathConfig {
	if p == nil {
		return FECPathConfig{}
	}
	return p.config
}

func (p *FECPath) FECEnabled() bool {
	return p != nil && p.config.ParityShards != 0
}

// FECWireLimit reports the largest datagram this adapter can emit for the
// configured SourceMTU. It is an interface bound, not the final outer MTU
// authority.
func (p *FECPath) FECWireLimit() int {
	if p == nil {
		return 0
	}
	if !p.FECEnabled() {
		return p.config.SourceMTU
	}
	return fec.HeaderSize + p.config.SourceMTU
}

// Encode fragments one application datagram once at LINK, then immediately
// emits every systematic source shard. Returned datagrams are owned immutable
// copies: the caller may retain them across subsequent Add/Flush calls.
func (p *FECPath) Encode(packet []byte, now time.Time) ([][]byte, error) {
	if p == nil || p.fragmenter == nil {
		return nil, fmt.Errorf("linkdata: FEC path unavailable")
	}
	fragments, err := p.fragmenter.Fragment(packet)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(fragments))
	for _, fragment := range fragments {
		if !p.FECEnabled() {
			out = append(out, append([]byte(nil), fragment...))
			continue
		}
		wire, err := p.encoder.Add(fragment, now)
		if err != nil {
			return nil, err
		}
		out = appendOwned(out, wire)
	}
	return out, nil
}

func (p *FECPath) FlushDue(now time.Time) ([][]byte, error) {
	if p == nil {
		return nil, nil
	}
	p.Expire(now)
	if !p.FECEnabled() {
		return nil, nil
	}
	wire, err := p.encoder.FlushDue(now)
	if err != nil {
		return nil, err
	}
	return appendOwned(nil, wire), nil
}

func (p *FECPath) Flush() ([][]byte, error) {
	if p == nil || !p.FECEnabled() {
		return nil, nil
	}
	wire, err := p.encoder.Flush()
	if err != nil {
		return nil, err
	}
	return appendOwned(nil, wire), nil
}

// Decode returns complete application datagrams. FEC systematic first-arrival
// delivery occurs before LINK reassembly, so a missing FEC block/shard never
// creates a lane-wide ordering dependency.
func (p *FECPath) Decode(wire []byte, now time.Time) ([][]byte, error) {
	if p == nil || p.reassembler == nil {
		return nil, fmt.Errorf("linkdata: FEC path unavailable")
	}
	p.Expire(now)

	var candidates [][]byte
	if !p.FECEnabled() {
		candidates = [][]byte{wire}
	} else {
		h, err := fec.ParseBlockHeader(wire)
		if err != nil {
			return nil, err
		}
		decoded, _, err := p.decoder.Add(wire)
		if err != nil {
			return nil, err
		}
		if p.decoder.IsHeavyBlock(h.BlockID) {
			p.recovery.observeHeavy(h.BlockID, now)
		} else {
			p.recovery.forget(h.BlockID)
		}
		candidates = decoded
	}

	packets := make([][]byte, 0, len(candidates))
	var firstErr error
	for _, candidate := range candidates {
		packet, complete, err := p.reassembler.Push(candidate, now)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if complete {
			packets = append(packets, append([]byte(nil), packet...))
		}
	}
	return packets, firstErr
}

// Expire is designed for an owner timer. It retires both LINK incomplete
// datagrams and FEC recovery state even when no new inbound packet arrives.
func (p *FECPath) Expire(now time.Time) {
	if p == nil {
		return
	}
	if p.reassembler != nil {
		p.reassembler.Expire(now)
	}
	if p.recovery != nil && p.decoder != nil {
		p.recovery.expire(p.decoder, now)
	}
}

func (p *FECPath) State() FECPathState {
	if p == nil {
		return FECPathState{}
	}
	state := FECPathState{
		FECEnabled:   p.FECEnabled(),
		ParityShards: p.config.ParityShards,
		Reassembly:   p.reassembler.State(),
	}
	if p.decoder != nil {
		state.Decoder = p.decoder.PressureCounts()
	}
	if p.recovery != nil {
		state.Recovery = p.recovery.stats()
	}
	return state
}

func appendOwned(dst [][]byte, src [][]byte) [][]byte {
	for _, item := range src {
		dst = append(dst, append([]byte(nil), item...))
	}
	return dst
}

type fecRecoveryDeadline struct {
	blockID  uint32
	deadline time.Time
}

type fecRecoveryTracker struct {
	horizon time.Duration
	seen    map[uint32]time.Time
	queue   []fecRecoveryDeadline
	head    int

	expireEvents          uint64
	expiredIncomplete     uint64
	expiredMissingSources uint64
}

func newFECRecoveryTracker(horizon time.Duration) *fecRecoveryTracker {
	return &fecRecoveryTracker{horizon: horizon, seen: make(map[uint32]time.Time)}
}

func (r *fecRecoveryTracker) observeHeavy(id uint32, now time.Time) {
	if r == nil {
		return
	}
	if _, ok := r.seen[id]; ok {
		return
	}
	deadline := now.Add(r.horizon)
	r.seen[id] = deadline
	r.queue = append(r.queue, fecRecoveryDeadline{blockID: id, deadline: deadline})
}

func (r *fecRecoveryTracker) forget(id uint32) {
	if r == nil {
		return
	}
	delete(r.seen, id)
}

func (r *fecRecoveryTracker) expire(dec *fec.BlockDecoder, now time.Time) {
	if r == nil || dec == nil {
		return
	}
	for r.head < len(r.queue) {
		item := r.queue[r.head]
		if item.deadline.After(now) {
			break
		}
		r.head++
		deadline, ok := r.seen[item.blockID]
		if !ok || !deadline.Equal(item.deadline) {
			continue
		}
		delete(r.seen, item.blockID)
		result := dec.RetireRecoveryExpired(item.blockID)
		if !result.Retired {
			continue
		}
		r.expireEvents++
		if result.MissingSources > 0 {
			r.expiredIncomplete++
			r.expiredMissingSources += uint64(result.MissingSources)
		}
	}
	if r.head >= 1024 && r.head*2 >= len(r.queue) {
		copy(r.queue, r.queue[r.head:])
		r.queue = r.queue[:len(r.queue)-r.head]
		r.head = 0
	}
}

func (r *fecRecoveryTracker) stats() FECRecoveryStats {
	if r == nil {
		return FECRecoveryStats{}
	}
	return FECRecoveryStats{
		HorizonMillis:         r.horizon.Milliseconds(),
		PendingDeadlines:      len(r.seen),
		ExpireEvents:          r.expireEvents,
		ExpiredIncomplete:     r.expiredIncomplete,
		ExpiredMissingSources: r.expiredMissingSources,
	}
}
