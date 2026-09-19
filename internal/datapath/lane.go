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

type WireRecord struct {
	PN   uint64
	Wire []byte
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

// Outbound owns all returned wire bytes. A caller may retain a WireRecord and
// retransmit its Wire at the same TCP sequence without invoking Seal again.
func (l *Lane) Outbound(packet []byte, now time.Time) ([]WireRecord, error) {
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
	return l.sealLocked(wire)
}

func (l *Lane) FlushDue(now time.Time) ([]WireRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrLaneClosed
	}
	wire, err := l.txPath.FlushDue(now)
	if err != nil {
		return nil, err
	}
	return l.sealLocked(wire)
}

func (l *Lane) Flush() ([]WireRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrLaneClosed
	}
	wire, err := l.txPath.Flush()
	if err != nil {
		return nil, err
	}
	return l.sealLocked(wire)
}

func (l *Lane) sealLocked(datagrams [][]byte) ([]WireRecord, error) {
	if len(datagrams) == 0 {
		return nil, nil
	}
	out := make([]WireRecord, 0, len(datagrams))
	for _, datagram := range datagrams {
		if len(datagram) > l.txBudget.RecordPayloadMTU {
			return nil, fmt.Errorf("%w: payload=%d record_payload_mtu=%d",
				ErrRecordOversize, len(datagram), l.txBudget.RecordPayloadMTU)
		}
		wire, pn, err := l.sealer.Seal(datagram)
		if err != nil {
			return nil, err
		}
		if len(wire) > l.txBudget.RecordWireMTU {
			return nil, fmt.Errorf("%w: wire=%d record_wire_mtu=%d",
				ErrRecordOversize, len(wire), l.txBudget.RecordWireMTU)
		}
		out = append(out, WireRecord{PN: pn, Wire: append([]byte(nil), wire...)})
		l.stats.OutboundRecords++
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
