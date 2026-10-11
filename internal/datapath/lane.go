package datapath

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

type Role uint8

const (
	RoleClient Role = 1
	RoleServer Role = 2
)

var (
	ErrLaneClosed     = errors.New("datapath: lane is closed")
	ErrConfigMismatch = errors.New("datapath: immutable lane configuration mismatch")
	ErrInvalidRole    = errors.New("datapath: invalid lane role")
	ErrRecordOversize = errors.New("datapath: sealed record exceeds lane budget")
)

type LaneConfig struct {
	Role Role

	IncarnationNonce [16]byte
	TunnelID         []byte

	ClientRecordLimit int
	ServerRecordLimit int
	Keys              tlsrecord.KeyPair

	// ParityShards is immutable for one lane incarnation. Zero means FEC off;
	// otherwise it must be one of the live fixed 20:R profiles.
	ParityShards int
	FlushAfter   time.Duration
	MaxBlocks    int

	// TxMTU describes this endpoint as sender. RxMTU is the mirrored sender
	// budget expected from the peer for this direction. Both must carry the
	// same immutable FEC profile and direction-specific negotiated limit.
	TxMTU pathmtu.Config
	RxMTU pathmtu.Config
}

type PaddingRequest struct {
	// Bytes is the desired encrypted padding per emitted record. It is a
	// non-blocking policy-layer request: when actual record headroom is smaller,
	// the owner immediately selects zero and records a budget skip. Exact
	// padding validation lives in tlsrecord.SealWithPadding.
	Bytes int
}

type paddingSelection struct {
	requested int
	bytes     int
	skipped   bool
	finish    func(success bool)
}

type paddingSelector func(headroom int, source bool) (paddingSelection, error)

// FECRecordMeta is sender-local, never serialized. The source header carries
// a provisional k=20; only emitted parity has the authoritative CLOSED k.
// Explicit Known distinguishes FEC-off from an uninstrumented/unknown record.
type FECRecordMeta struct {
	Known bool
	Enabled bool
	BlockID uint32
	ShardIndex uint8
	ConfiguredParity uint8
	ClosedK uint8
	ShardLengthBin uint8 // 0: <=256B, 1: <=512B, 2: >512B, final parity only
}

type WireRecord struct {
	// Ref is local ownership metadata, never serialized or encrypted. Records
	// sealed before promotion cannot be relabeled onto a new association.
	Ref          logicaltunnel.LaneRef
	Control      bool
	PN           uint64
	FEC          FECRecordMeta
	PaddingBytes int
	Wire         []byte
}

// QualityEchoInbound indexes the same authenticated quality control in
// InboundResult.Quality. No echo exists on legacy or 104-byte health.
type QualityEchoInbound struct {
    Index int
    Echo QualityEchoV4
}

type InboundResult struct {
	Authenticated uint64
	Health        []HealthMessage
	Quality       []QualityHealthReport
	// QualityControlPN parallels Quality: authenticated sender-side seal PN
	// of the KindHealth marker, not a receiver-local or LINK record PN.
	QualityControlPN []uint64
	QualityEcho []QualityEchoInbound
	Datagrams     [][]byte
	RecordErrors  []error
	PathErrors    []error
}

type LaneStats struct {
	OutboundDatagrams  uint64
	OutboundRecords    uint64
	InboundPayloads    uint64
	InboundRecords     uint64
	DeliveredDatagrams uint64
	RecordErrors       uint64
	PathErrors         uint64
	LastPathError      string
	ExpireCalls        uint64

	TimingEnabled        bool
	InboundTimingSamples uint64
	InboundLockWaitNS    uint64
	InboundLockWaitMaxNS uint64
	InboundDecodeNS      uint64
	InboundDecodeMaxNS   uint64
	ExpireNS             uint64
	ExpireMaxNS          uint64

	PaddingRequests       uint64
	RequestedPaddingBytes uint64
	PaddedRecords         uint64
	PaddingBytes          uint64
	PaddingBudgetSkips    uint64
	PaddingRejected       uint64

	Decoder tlsrecord.DecoderStats
	TxPath  linkdata.FECPathState
	RxPath  linkdata.FECPathState
	Closed  bool
}

type Lane struct {
	// mu owns TX: encoder/sealer/PN and outbound/padding counters. RX never
	// waits on an unrelated TX encode/seal operation; rxMu owns decoder,
	// recovery/reassembly and inbound/expiry counters.
	mu   sync.Mutex
	rxMu sync.Mutex

	cfg      LaneConfig
	txBudget pathmtu.Budget
	rxBudget pathmtu.Budget

	sealer  *tlsrecord.Sealer
	decoder *tlsrecord.Decoder
	txPath  *linkdata.FECPath
	rxPath  *linkdata.FECPath
	adaptiveN2Test bool
	// Only the explicitly verified protected-policy test construction fills
	// this immutable incarnation policy. Legacy/fixed and free-form test
	// adaptive lanes never gain an implicit auto admission capability.
	n2ProtectedPolicy realityfront.AdmissionPolicy
	// A single buffered owner signal is emitted only when a first/earlier
	// partial-FEC deadline is created. Nonblocking: never delay fresh data.
	fecDeadlineWake chan<- struct{}

	stats         LaneStats
	timingEnabled atomic.Bool
	qualityHealthV3 atomic.Bool // enabled only after authenticated V3 admission
	qualityRX qualityRXWindow // rxMu; KindLINK-only PN evidence
	// closed is read under either direction's lock and written only while
	// holding both. Stats and Close always acquire mu before rxMu; ordinary
	// operations acquire one direction only. Configuration/budgets are immutable.
	closed bool
}

func NewLane(cfg LaneConfig) (*Lane, error) {
	return newLaneWithN2TestPath(cfg,false)
}

// NewN2AdaptiveLaneForTests tests authenticated mixed on/off LINK paths.
// Production admission continues to construct fixed NewLane instances.
func NewN2AdaptiveLaneForTests(cfg LaneConfig) (*Lane, error) {
	if cfg.ParityShards != fec.ParityShards {return nil,ErrConfigMismatch}
	return newLaneWithN2TestPath(cfg,true)
}

func newLaneWithN2TestPath(cfg LaneConfig, adaptive bool) (*Lane, error) {
	txLimit, rxLimit, txKeys, rxKeys, err := directionParams(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.TxMTU.RecordWireLimit != txLimit || cfg.RxMTU.RecordWireLimit != rxLimit {
		return nil, fmt.Errorf("%w: role=%d tx_limit=%d/%d rx_limit=%d/%d",
			ErrConfigMismatch, cfg.Role,
			cfg.TxMTU.RecordWireLimit, txLimit,
			cfg.RxMTU.RecordWireLimit, rxLimit)
	}
	if cfg.TxMTU.ParityShards != cfg.ParityShards || cfg.RxMTU.ParityShards != cfg.ParityShards {
		return nil, fmt.Errorf("%w: lane parity=%d tx=%d rx=%d",
			ErrConfigMismatch, cfg.ParityShards, cfg.TxMTU.ParityShards, cfg.RxMTU.ParityShards)
	}
	if cfg.ParityShards != 0 && (cfg.FlushAfter <= 0 || cfg.MaxBlocks <= 0) {
		return nil, fmt.Errorf("%w: fec runtime flush=%s max_blocks=%d",
			ErrConfigMismatch, cfg.FlushAfter, cfg.MaxBlocks)
	}

	txBudget, err := pathmtu.Derive(cfg.TxMTU)
	if err != nil {
		return nil, err
	}
	rxBudget, err := pathmtu.Derive(cfg.RxMTU)
	if err != nil {
		return nil, err
	}
	if txBudget.ParityShards != cfg.ParityShards || rxBudget.ParityShards != cfg.ParityShards {
		return nil, fmt.Errorf("%w: derived parity tx=%d rx=%d lane=%d",
			ErrConfigMismatch, txBudget.ParityShards, rxBudget.ParityShards, cfg.ParityShards)
	}

	txPathCtor:=linkdata.NewFECPath
	if adaptive {txPathCtor=linkdata.NewAdaptiveFECPath}
	txPath, err := txPathCtor(linkdata.FECPathConfig{
		SourceMTU:    txBudget.LinkFrameMTU,
		ParityShards: cfg.ParityShards,
		FlushAfter:   cfg.FlushAfter,
		MaxBlocks:    cfg.MaxBlocks,
	})
	if err != nil {
		return nil, err
	}
	rxPathCtor:=linkdata.NewFECPath
	if adaptive {rxPathCtor=linkdata.NewAdaptiveFECPath}
	rxPath, err := rxPathCtor(linkdata.FECPathConfig{
		SourceMTU:    rxBudget.LinkFrameMTU,
		ParityShards: cfg.ParityShards,
		FlushAfter:   cfg.FlushAfter,
		MaxBlocks:    cfg.MaxBlocks,
	})
	if err != nil {
		return nil, err
	}
	sealer, err := tlsrecord.NewSealer(txKeys, txBudget.RecordWireMTU)
	if err != nil {
		return nil, err
	}
	decoder, err := tlsrecord.NewDecoder(rxKeys, rxBudget.RecordWireMTU)
	if err != nil {
		return nil, err
	}

	cfg.TunnelID = append([]byte(nil), cfg.TunnelID...)
	return &Lane{
		cfg:      cfg,
		txBudget: txBudget,
		rxBudget: rxBudget,
		sealer:   sealer,
		decoder:  decoder,
		txPath:   txPath,
		rxPath:   rxPath,
		adaptiveN2Test: adaptive,
	}, nil
}

func directionParams(cfg LaneConfig) (txLimit, rxLimit int, txKeys, rxKeys tlsrecord.Keys, err error) {
	switch cfg.Role {
	case RoleClient:
		return cfg.ServerRecordLimit, cfg.ClientRecordLimit, cfg.Keys.C2S, cfg.Keys.S2C, nil
	case RoleServer:
		return cfg.ClientRecordLimit, cfg.ServerRecordLimit, cfg.Keys.S2C, cfg.Keys.C2S, nil
	default:
		return 0, 0, tlsrecord.Keys{}, tlsrecord.Keys{}, ErrInvalidRole
	}
}

func (l *Lane) Config() LaneConfig {
	l.mu.Lock()
	defer l.mu.Unlock()
	cfg := l.cfg
	cfg.TunnelID = append([]byte(nil), l.cfg.TunnelID...)
	return cfg
}

func (l *Lane) TxBudget() pathmtu.Budget {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.txBudget
}

func (l *Lane) RxBudget() pathmtu.Budget {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rxBudget
}

func (l *Lane) rejectInvalidPadding(request PaddingRequest) error {
	if request.Bytes >= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrLaneClosed
	}
	l.stats.PaddingRejected++
	return tlsrecord.ErrInvalidPadding
}

// Outbound owns all returned wire bytes. It preserves the production-default
// padding=0 path and uses tlsrecord.Seal, so existing zero-padding vectors stay
// byte-for-byte unchanged. A caller may retain a WireRecord and retransmit its
// Wire at the same TCP sequence without invoking Seal again.
// setFECDeadlineWake is called only before an incarnation is published.
// A candidate may notify before promotion; the scheduler rechecks active
// generation and never emits parity from an unpublished candidate.
func (l *Lane) setFECDeadlineWake(ch chan<- struct{}) {
	l.mu.Lock()
	l.fecDeadlineWake = ch
	l.mu.Unlock()
}

func (l *Lane) Outbound(packet []byte, now time.Time) ([]WireRecord, error) {
	return l.outbound(packet, now, nil)
}

// OutboundWithPadding is the explicit P3 padding API. The fixed request is
// evaluated independently for each already-formed LINK/FEC record payload.
// Insufficient headroom selects zero immediately; no wait, extra record,
// fragment, timer, or FEC change is introduced.
func (l *Lane) OutboundWithPadding(packet []byte, now time.Time, request PaddingRequest) ([]WireRecord, error) {
	if err := l.rejectInvalidPadding(request); err != nil {
		return nil, err
	}
	return l.outbound(packet, now, fixedPaddingSelector(request))
}

// outboundWithPaddingSelector is the P4 owner hook. The selector runs once per
// already-formed record payload after exact MTU headroom is known. It may reserve
// tunnel-wide budget and receives a success/failure finalizer after sealing.
func (l *Lane) outboundWithPaddingSelector(packet []byte, now time.Time, selector paddingSelector) ([]WireRecord, error) {
	return l.outbound(packet, now, selector)
}

func (l *Lane) outbound(packet []byte, now time.Time, selector paddingSelector) ([]WireRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrLaneClosed
	}
	previous := l.txPath.NextFlushDeadline()
	wire, err := l.txPath.Encode(packet, now)
	if err != nil {
		return nil, err
	}
	if next := l.txPath.NextFlushDeadline(); !next.IsZero() &&
		(previous.IsZero() || next.Before(previous)) && l.fecDeadlineWake != nil {
		select {
		case l.fecDeadlineWake <- struct{}{}:
		default:
		}
	}
	l.stats.OutboundDatagrams++
	return l.sealLocked(wire, selector)
}

// NextFlushDeadline is read under the TX mutex, and never waits for RX.
// A retired/closed generation reports no pending outbound work. The caller
// must still fence the lane generation when actually flushing/issuing wire.
// SwitchN2ParityForTests applies a new parity to NEW blocks only. The
// explicit opt-in constructor is not reachable from production admission.
// PrepareN2ParityForTests only reads immutable geometry/codec and allocates
// an unused encoder. It holds NO Lane TX, TunnelOwner or generation lock.
// The active encoder remains authoritative until CommitN2ParityForTests.
func (l *Lane) PrepareN2ParityForTests(parity int) (*linkdata.PreparedParitySwitch,error) {
	if l==nil {return nil,ErrLaneClosed}
	if !l.adaptiveN2Test {return nil,linkdata.ErrAdaptiveSwitchUnavailable}
	return l.txPath.PrepareParitySwitch(parity)
}

// CommitN2ParityForTests is short and only replaces the encoder for NEW
// blocks. The caller owning a Runtime must pin the active generation across
// this call. Existing partial deadlines and shared RX remain untouched.
func (l *Lane) CommitN2ParityForTests(prepared *linkdata.PreparedParitySwitch) error {
	if l==nil {return ErrLaneClosed}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {return ErrLaneClosed}
	if !l.adaptiveN2Test {return linkdata.ErrAdaptiveSwitchUnavailable}
	return l.txPath.CommitPreparedParitySwitch(prepared)
}

func (l *Lane) SwitchN2ParityForTests(parity int) error {
	prepared,err:=l.PrepareN2ParityForTests(parity)
	if err!=nil{return err}
	return l.CommitN2ParityForTests(prepared)
}

func (l *Lane) NextFlushDeadline() time.Time {
	if l == nil {
		return time.Time{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return time.Time{}
	}
	return l.txPath.NextFlushDeadline()
}

func (l *Lane) FlushDue(now time.Time) ([]WireRecord, error) {
	return l.flushDue(now, nil)
}

func (l *Lane) FlushDueWithPadding(now time.Time, request PaddingRequest) ([]WireRecord, error) {
	if err := l.rejectInvalidPadding(request); err != nil {
		return nil, err
	}
	return l.flushDue(now, fixedPaddingSelector(request))
}

func (l *Lane) flushDue(now time.Time, selector paddingSelector) ([]WireRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrLaneClosed
	}
	wire, err := l.txPath.FlushDue(now)
	if err != nil {
		return nil, err
	}
	return l.sealLocked(wire, selector)
}

func (l *Lane) Flush() ([]WireRecord, error) {
	return l.flush(nil)
}

func (l *Lane) FlushWithPadding(request PaddingRequest) ([]WireRecord, error) {
	if err := l.rejectInvalidPadding(request); err != nil {
		return nil, err
	}
	return l.flush(fixedPaddingSelector(request))
}

func (l *Lane) flush(selector paddingSelector) ([]WireRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrLaneClosed
	}
	wire, err := l.txPath.Flush()
	if err != nil {
		return nil, err
	}
	return l.sealLocked(wire, selector)
}

func fixedPaddingSelector(request PaddingRequest) paddingSelector {
	return func(headroom int, _ bool) (paddingSelection, error) {
		if request.Bytes > headroom {
			return paddingSelection{requested: request.Bytes, skipped: true}, nil
		}
		return paddingSelection{requested: request.Bytes, bytes: request.Bytes}, nil
	}
}

func (l *Lane) sealLocked(datagrams [][]byte, selector paddingSelector) ([]WireRecord, error) {
	if len(datagrams) == 0 {
		return nil, nil
	}
	selections := make([]paddingSelection, len(datagrams))
	cancelPending := func(from int) {
		for i := from; i < len(selections); i++ {
			if selections[i].finish != nil {
				selections[i].finish(false)
				selections[i].finish = nil
			}
		}
	}

	var requestedBytes uint64
	var skips uint64
	for i, datagram := range datagrams {
		if len(datagram) > l.txBudget.RecordPayloadMTU {
			cancelPending(0)
			return nil, fmt.Errorf("%w: payload=%d record_payload_mtu=%d",
				ErrRecordOversize, len(datagram), l.txBudget.RecordPayloadMTU)
		}
		if selector == nil {
			continue
		}
		headroom, err := l.txBudget.PaddingHeadroom(len(datagram))
		if err != nil {
			cancelPending(0)
			return nil, err
		}
		source := true
		if l.adaptiveN2Test {
			// Old parity can flush AFTER the active profile has gone off.
			if len(datagram)>=2 && datagram[0]=='W' && datagram[1]=='F' {
				source=len(datagram)>=fec.HeaderSize && datagram[8]<fec.DataShards
			}
		} else if l.cfg.ParityShards!=0 {
			source=len(datagram)>=fec.HeaderSize && datagram[8]<fec.DataShards
		}
		selection, err := selector(headroom, source)
		if err != nil {
			cancelPending(0)
			return nil, err
		}
		if selection.requested < 0 || selection.bytes < 0 || selection.bytes > selection.requested || selection.bytes > headroom {
			if selection.finish != nil {
				selection.finish(false)
			}
			cancelPending(0)
			l.stats.PaddingRejected++
			return nil, tlsrecord.ErrInvalidPadding
		}
		selections[i] = selection
		if selection.requested > 0 {
			requestedBytes += uint64(selection.requested)
		}
		if selection.skipped {
			skips++
		}
	}

	out := make([]WireRecord, 0, len(datagrams))
	var appliedBytes uint64
	var paddedRecords uint64
	for i, datagram := range datagrams {
		selection := selections[i]
		padding := selection.bytes
		var (
			wire []byte
			pn   uint64
			err  error
		)
		if selector == nil {
			wire, pn, err = l.sealer.Seal(datagram)
		} else {
			wire, pn, err = l.sealer.SealWithPadding(datagram, padding)
		}
		if err != nil {
			if selection.finish != nil {
				selection.finish(false)
				selections[i].finish = nil
			}
			cancelPending(i + 1)
			return nil, err
		}
		if len(wire) > l.txBudget.RecordWireMTU {
			if selection.finish != nil {
				selection.finish(false)
				selections[i].finish = nil
			}
			cancelPending(i + 1)
			return nil, fmt.Errorf("%w: wire=%d record_wire_mtu=%d",
				ErrRecordOversize, len(wire), l.txBudget.RecordWireMTU)
		}
		if selection.finish != nil {
			selection.finish(true)
			selections[i].finish = nil
		}
		metadata := FECRecordMeta{Known: true}
		if (l.adaptiveN2Test && len(datagram)>=2 && datagram[0]=='W' && datagram[1]=='F') ||
			(!l.adaptiveN2Test && l.cfg.ParityShards!=0) {
			// Owned v1 shard bytes are still available BEFORE encryption. This
			// local witness does not decrypt a record or alter the wire.
			if len(datagram) < fec.HeaderSize {
				return nil, ErrConfigMismatch
			}
			metadata.Enabled = true
			metadata.BlockID = uint32(datagram[4])<<24 | uint32(datagram[5])<<16 | uint32(datagram[6])<<8 | uint32(datagram[7])
			metadata.ShardIndex = datagram[8]
			metadata.ConfiguredParity = datagram[10]
			if metadata.ShardIndex >= fec.DataShards {
				metadata.ClosedK = datagram[11]
				maxShard := uint16(datagram[12])<<8 | uint16(datagram[13])
				switch {
				case maxShard <= 256:
					metadata.ShardLengthBin = 0
				case maxShard <= 512:
					metadata.ShardLengthBin = 1
				default:
					metadata.ShardLengthBin = 2
				}
			}
		}
		out = append(out, WireRecord{
			PN:           pn,
			FEC:          metadata,
			PaddingBytes: padding,
			// Sealer returns a fresh owned allocation, including its AEAD body.
			Wire: wire,
		})
		l.stats.OutboundRecords++
		if padding > 0 {
			appliedBytes += uint64(padding)
			paddedRecords++
		}
	}
	if selector != nil {
		l.stats.PaddingRequests += uint64(len(datagrams))
		l.stats.RequestedPaddingBytes += requestedBytes
		l.stats.PaddingBudgetSkips += skips
		l.stats.PaddingBytes += appliedBytes
		l.stats.PaddedRecords += paddedRecords
	}
	return out, nil
}

// InboundPayload accepts one FakeTCP payload. It intentionally continues after
// an independently bounded bad record or bad FEC/LINK datagram, so one damaged
// item cannot become a lane-wide receive barrier.
func (l *Lane) InboundPayload(payload []byte, now time.Time) (InboundResult, error) {
	observeTiming := l.timingEnabled.Load()
	var waitStarted time.Time
	if observeTiming {
		waitStarted = time.Now()
	}
	l.rxMu.Lock()
	if l.closed {
		l.rxMu.Unlock()
		return InboundResult{}, ErrLaneClosed
	}
	var decodeStarted time.Time
	if observeTiming {
		wait := time.Since(waitStarted)
		l.stats.TimingEnabled = true
		l.stats.InboundTimingSamples++
		l.stats.InboundLockWaitNS += uint64(wait)
		if uint64(wait) > l.stats.InboundLockWaitMaxNS {
			l.stats.InboundLockWaitMaxNS = uint64(wait)
		}
		decodeStarted = time.Now()
	}
	l.stats.InboundPayloads++
	out := l.inboundLocked(payload, now)
	if observeTiming {
		elapsed := time.Since(decodeStarted)
		l.stats.InboundDecodeNS += uint64(elapsed)
		if uint64(elapsed) > l.stats.InboundDecodeMaxNS {
			l.stats.InboundDecodeMaxNS = uint64(elapsed)
		}
	}
	l.rxMu.Unlock()
	return out, nil
}

func (l *Lane) inboundLocked(payload []byte, now time.Time) InboundResult {
	var out InboundResult
	for _, decoded := range l.decoder.OpenPayload(payload) {
		if decoded.Err != nil {
			if l.qualityHealthV3.Load() && decoded.Kind == tlsrecord.KindLINK && errors.Is(decoded.Err, tlsrecord.ErrDuplicate) {
				l.qualityRX.observe(decoded.PN, true, now)
			}
			out.RecordErrors = append(out.RecordErrors, decoded.Err)
			l.stats.RecordErrors++
			continue
		}
		l.stats.InboundRecords++
		if decoded.Kind == tlsrecord.KindHealth {
            // The non-live N1 codec becomes accepted only after the lane has
            // completed protected V3 admission and opted into quality health.
            // V1/V2 and any unqualified lane retain strict 9-byte parsing.
            if l.qualityHealthV3.Load() && len(decoded.Payload)==QualityEchoV4Size &&
                decoded.Payload[0]==QualityEchoV4Version {
                quality,err:=DecodeQualityEchoV4(decoded.Payload)
                if err!=nil {
                    out.RecordErrors=append(out.RecordErrors,err)
                    l.stats.RecordErrors++
                    continue
                }
                index:=len(out.Quality)
                out.Authenticated++
                out.Quality=append(out.Quality,quality.Report)
                out.QualityControlPN=append(out.QualityControlPN,decoded.PN)
                out.QualityEcho=append(out.QualityEcho,QualityEchoInbound{Index:index,Echo:quality})
                out.Health=append(out.Health,HealthMessage{PN:decoded.PN,IdleFor:quality.Report.IdleFor})
                continue
            }
            if l.qualityHealthV3.Load() && len(decoded.Payload) == QualityHealthV3Size &&
                decoded.Payload[0] == QualityHealthV3Version {
                quality, err := DecodeQualityHealthV3(decoded.Payload)
                if err != nil {
                    out.RecordErrors = append(out.RecordErrors, err)
                    l.stats.RecordErrors++
                    continue
                }
                out.Authenticated++
                out.Quality = append(out.Quality, quality)
                out.QualityControlPN = append(out.QualityControlPN, decoded.PN)
                out.Health = append(out.Health, HealthMessage{PN: decoded.PN, IdleFor: quality.IdleFor})
                continue
            }
			health, err := parseHealth(decoded.PN, decoded.Payload)
			if err != nil {
				out.RecordErrors = append(out.RecordErrors, err)
				l.stats.RecordErrors++
				continue
			}
			out.Authenticated++
			out.Health = append(out.Health, health)
			continue
		}
		out.Authenticated++
		if l.qualityHealthV3.Load() && decoded.Kind == tlsrecord.KindLINK {
			l.qualityRX.observe(decoded.PN, false, now)
		}
		packets, err := l.rxPath.Decode(decoded.Payload, now)
		for _, packet := range packets {
			// FECPath.Decode already transfers owned LINK output; its borrowed
			// shard buffers are copied inside that boundary, not here again.
			out.Datagrams = append(out.Datagrams, packet)
			l.stats.DeliveredDatagrams++
		}
		if err != nil {
			out.PathErrors = append(out.PathErrors, err)
			l.stats.PathErrors++
			l.stats.LastPathError = err.Error()
			if len(l.stats.LastPathError) > 512 {
				l.stats.LastPathError = l.stats.LastPathError[:512]
			}
		}
	}
	return out
}

// Expire is the owner timer hook. It is explicit so FEC/LINK retirement still
// runs after traffic stops; it does not generate traffic or wait for work.
func (l *Lane) Expire(now time.Time) error {
	observeTiming := l.timingEnabled.Load()
	l.rxMu.Lock()
	if l.closed {
		l.rxMu.Unlock()
		return ErrLaneClosed
	}
	var started time.Time
	if observeTiming {
		started = time.Now()
	}
	l.rxPath.Expire(now)
	l.stats.ExpireCalls++
	if observeTiming {
		elapsed := time.Since(started)
		l.stats.TimingEnabled = true
		l.stats.ExpireNS += uint64(elapsed)
		if uint64(elapsed) > l.stats.ExpireMaxNS {
			l.stats.ExpireMaxNS = uint64(elapsed)
		}
	}
	l.rxMu.Unlock()
	return nil
}

// SetTimingDiagnostics enables bounded timing counters only; it does not alter
// packet ownership, FEC scheduling, or delivery order.
func (l *Lane) SetTimingDiagnostics(enabled bool) {
	if l == nil {
		return
	}
	l.timingEnabled.Store(enabled)
}

func (l *Lane) Stats() LaneStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rxMu.Lock()
	defer l.rxMu.Unlock()
	out := l.stats
	out.Closed = l.closed
	if l.decoder != nil {
		out.Decoder = l.decoder.Stats()
	}
	if l.txPath != nil {
		out.TxPath = l.txPath.State()
	}
	if l.rxPath != nil {
		out.RxPath = l.rxPath.State()
	}
	return out
}

// Close releases lane-local crypto/FEC/reassembly state only. It has no global
// registry side effect and cannot close a sibling lane.
func (l *Lane) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rxMu.Lock()
	defer l.rxMu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	l.sealer = nil
	l.decoder = nil
	l.txPath = nil
	l.rxPath = nil
	l.stats.Closed = true
}

// N2ProtectedPolicyForTests returns the exact protected V3 policy attached
// at staged admission; no run-time profile change mutates this identity.
// Production auto is still disallowed by realityfront.SupportedNow.
func (l *Lane) N2ProtectedPolicyForTests() (realityfront.AdmissionPolicy,bool) {
	if l==nil {return realityfront.AdmissionPolicy{},false}
	l.mu.Lock()
	defer l.mu.Unlock()
	p:=l.n2ProtectedPolicy
	return p,l.adaptiveN2Test && !l.closed &&
		p.Schema==realityfront.PolicySchemaV3
}
