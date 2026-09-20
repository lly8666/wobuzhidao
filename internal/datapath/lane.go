package datapath

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
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

type paddingSelector func(headroom int) (paddingSelection, error)

type WireRecord struct {
	PN           uint64
	PaddingBytes int
	Wire         []byte
}

type InboundResult struct {
	Datagrams    [][]byte
	RecordErrors []error
	PathErrors   []error
}

type LaneStats struct {
	OutboundDatagrams  uint64
	OutboundRecords    uint64
	InboundPayloads    uint64
	InboundRecords     uint64
	DeliveredDatagrams uint64
	RecordErrors       uint64
	PathErrors         uint64
	ExpireCalls        uint64

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
	mu sync.Mutex

	cfg      LaneConfig
	txBudget pathmtu.Budget
	rxBudget pathmtu.Budget

	sealer  *tlsrecord.Sealer
	decoder *tlsrecord.Decoder
	txPath  *linkdata.FECPath
	rxPath  *linkdata.FECPath

	stats  LaneStats
	closed bool
}

func NewLane(cfg LaneConfig) (*Lane, error) {
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

	txPath, err := linkdata.NewFECPath(linkdata.FECPathConfig{
		SourceMTU:    txBudget.LinkFrameMTU,
		ParityShards: cfg.ParityShards,
		FlushAfter:   cfg.FlushAfter,
		MaxBlocks:    cfg.MaxBlocks,
	})
	if err != nil {
		return nil, err
	}
	rxPath, err := linkdata.NewFECPath(linkdata.FECPathConfig{
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
	wire, err := l.txPath.Encode(packet, now)
	if err != nil {
		return nil, err
	}
	l.stats.OutboundDatagrams++
	return l.sealLocked(wire, selector)
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
	return func(headroom int) (paddingSelection, error) {
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
		selection, err := selector(headroom)
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
		out = append(out, WireRecord{
			PN:           pn,
			PaddingBytes: padding,
			Wire:         append([]byte(nil), wire...),
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
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return InboundResult{}, ErrLaneClosed
	}
	l.stats.InboundPayloads++
	return l.inboundLocked(payload, now), nil
}

func (l *Lane) inboundLocked(payload []byte, now time.Time) InboundResult {
	var out InboundResult
	for _, decoded := range l.decoder.OpenPayload(payload) {
		if decoded.Err != nil {
			out.RecordErrors = append(out.RecordErrors, decoded.Err)
			l.stats.RecordErrors++
			continue
		}
		l.stats.InboundRecords++
		packets, err := l.rxPath.Decode(decoded.Payload, now)
		for _, packet := range packets {
			out.Datagrams = append(out.Datagrams, append([]byte(nil), packet...))
			l.stats.DeliveredDatagrams++
		}
		if err != nil {
			out.PathErrors = append(out.PathErrors, err)
			l.stats.PathErrors++
		}
	}
	return out
}

// Expire is the owner timer hook. It is explicit so FEC/LINK retirement still
// runs after traffic stops; it does not generate traffic or wait for work.
func (l *Lane) Expire(now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrLaneClosed
	}
	l.rxPath.Expire(now)
	l.stats.ExpireCalls++
	return nil
}

func (l *Lane) Stats() LaneStats {
	l.mu.Lock()
	defer l.mu.Unlock()
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
