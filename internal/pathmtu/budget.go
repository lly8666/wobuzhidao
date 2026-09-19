package pathmtu

import (
	"errors"
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

const (
	DefaultConnectionMTU = 1500
	MinConnectionMTU     = 576
	MaxConnectionMTU     = 9000

	MinIPv4HeaderLen = 20
	MaxIPv4HeaderLen = 60
	MinTCPHeaderLen  = 20
	MaxTCPHeaderLen  = 60

	// LinkFragmentHeaderLen is the fixed WBDLFRG1 envelope size from WIRE_SPEC.
	// Kept here as a budgeting constant so pathmtu stays below linkdata and the
	// later owner can pass Budget.LinkFrameMTU into linkdata without an import
	// cycle.
	LinkFragmentHeaderLen = 20
)

var (
	ErrConnectionMTU = errors.New("pathmtu: connection MTU cannot satisfy enabled feature budget")
	ErrInvalidHeader = errors.New("pathmtu: invalid IPv4/TCP header length")
	ErrInvalidMSS    = errors.New("pathmtu: invalid peer MSS")
	ErrRecordLimit   = errors.New("pathmtu: invalid negotiated record wire limit")
	ErrFECProfile    = errors.New("pathmtu: unsupported fixed FEC profile")
)

type Config struct {
	// ConnectionMTU is the sole operator-visible outer IPv4 packet ceiling.
	ConnectionMTU int

	// LocalPacketMTU is an optional lower local/device/path ceiling. Zero means
	// there is no additional local cap beyond ConnectionMTU.
	LocalPacketMTU int

	// Header lengths are the actual serialized steady-state IPv4/TCP header
	// lengths for the carrier payload under this lane configuration.
	IPv4HeaderLen int
	TCPHeaderLen  int

	// PeerMSSSet mirrors the peer SYN/SYN-ACK. When false, IPv4 uses the
	// protocol-required effective fallback already used by faketcp: 536 bytes.
	PeerMSS    uint16
	PeerMSSSet bool

	// RecordWireLimit is the direction-specific limit negotiated by protected
	// admission. It must already be a valid TLS-like V1 record wire limit.
	RecordWireLimit int

	// ParityShards is 0 for FEC off or one of the archived live fixed 20:R
	// profiles (4/8/10/12/16/20).
	ParityShards int
}

type Budget struct {
	ConnectionMTU   int
	LocalPacketMTU  int
	EffectivePacketMTU int

	IPv4HeaderLen int
	TCPHeaderLen  int

	PeerMSS          int
	PeerMSSAdvertised bool

	PacketPayloadMTU int
	CarrierPayloadMTU int

	NegotiatedRecordWireLimit int
	RecordWireMTU             int
	RecordPayloadMTU          int
	TLSLikeOverhead           int

	FECEnabled  bool
	ParityShards int
	FECOverhead int

	LinkFrameMTU             int
	LinkFragmentOverhead     int
	LinkFragmentPayloadMTU   int
}

func ValidateConnectionMTU(connectionMTU int) error {
	if connectionMTU < MinConnectionMTU || connectionMTU > MaxConnectionMTU {
		return fmt.Errorf("%w: connection=%d outside product range %d..%d", ErrConnectionMTU, connectionMTU, MinConnectionMTU, MaxConnectionMTU)
	}
	return nil
}

func validateHeaderLen(name string, value, min, max int) error {
	if value < min || value > max || value%4 != 0 {
		return fmt.Errorf("%w: %s=%d want multiple-of-4 in %d..%d", ErrInvalidHeader, name, value, min, max)
	}
	return nil
}

func effectivePeerMSS(cfg Config) (int, bool, error) {
	if !cfg.PeerMSSSet {
		return faketcp.DefaultIPv4PeerMSS, false, nil
	}
	if cfg.PeerMSS == 0 {
		return 0, true, fmt.Errorf("%w: advertised MSS=0", ErrInvalidMSS)
	}
	return int(cfg.PeerMSS), true, nil
}

func Derive(cfg Config) (Budget, error) {
	if err := ValidateConnectionMTU(cfg.ConnectionMTU); err != nil {
		return Budget{}, err
	}
	if cfg.LocalPacketMTU < 0 {
		return Budget{}, fmt.Errorf("%w: local packet MTU=%d", ErrConnectionMTU, cfg.LocalPacketMTU)
	}
	if cfg.LocalPacketMTU != 0 && cfg.LocalPacketMTU < MinConnectionMTU {
		return Budget{}, fmt.Errorf("%w: local packet MTU=%d below IPv4 product floor=%d", ErrConnectionMTU, cfg.LocalPacketMTU, MinConnectionMTU)
	}
	if err := validateHeaderLen("ipv4_header", cfg.IPv4HeaderLen, MinIPv4HeaderLen, MaxIPv4HeaderLen); err != nil {
		return Budget{}, err
	}
	if err := validateHeaderLen("tcp_header", cfg.TCPHeaderLen, MinTCPHeaderLen, MaxTCPHeaderLen); err != nil {
		return Budget{}, err
	}
	if cfg.RecordWireLimit < tlsrecord.FixedWireOverhead || cfg.RecordWireLimit > tlsrecord.MaxWireLen {
		return Budget{}, fmt.Errorf("%w: limit=%d allowed=%d..%d", ErrRecordLimit, cfg.RecordWireLimit, tlsrecord.FixedWireOverhead, tlsrecord.MaxWireLen)
	}
	if cfg.ParityShards != 0 && !fec.IsSupportedParityShards(cfg.ParityShards) {
		return Budget{}, fmt.Errorf("%w: 20:%d", ErrFECProfile, cfg.ParityShards)
	}

	effectivePacketMTU := cfg.ConnectionMTU
	if cfg.LocalPacketMTU != 0 && cfg.LocalPacketMTU < effectivePacketMTU {
		effectivePacketMTU = cfg.LocalPacketMTU
	}
	headers := cfg.IPv4HeaderLen + cfg.TCPHeaderLen
	if effectivePacketMTU <= headers {
		return Budget{}, fmt.Errorf("%w: outer=%d headers=%d", ErrConnectionMTU, effectivePacketMTU, headers)
	}
	packetPayloadMTU := effectivePacketMTU - headers

	peerMSS, advertised, err := effectivePeerMSS(cfg)
	if err != nil {
		return Budget{}, err
	}
	carrierPayloadMTU := packetPayloadMTU
	if peerMSS < carrierPayloadMTU {
		carrierPayloadMTU = peerMSS
	}
	if carrierPayloadMTU < tlsrecord.FixedWireOverhead {
		return Budget{}, fmt.Errorf("%w: carrier payload=%d below TLS-like minimum wire=%d", ErrConnectionMTU, carrierPayloadMTU, tlsrecord.FixedWireOverhead)
	}

	recordWireMTU := carrierPayloadMTU
	if cfg.RecordWireLimit < recordWireMTU {
		recordWireMTU = cfg.RecordWireLimit
	}
	recordPayloadMTU := recordWireMTU - tlsrecord.FixedWireOverhead

	fecOverhead := 0
	if cfg.ParityShards != 0 {
		fecOverhead = fec.HeaderSize
	}
	linkFrameMTU := recordPayloadMTU - fecOverhead
	if linkFrameMTU <= LinkFragmentHeaderLen {
		return Budget{}, fmt.Errorf("%w: record_wire=%d record_payload=%d fec_overhead=%d leaves LINK frame=%d, need >%d", ErrConnectionMTU, recordWireMTU, recordPayloadMTU, fecOverhead, linkFrameMTU, LinkFragmentHeaderLen)
	}

	return Budget{
		ConnectionMTU: cfg.ConnectionMTU,
		LocalPacketMTU: cfg.LocalPacketMTU,
		EffectivePacketMTU: effectivePacketMTU,
		IPv4HeaderLen: cfg.IPv4HeaderLen,
		TCPHeaderLen: cfg.TCPHeaderLen,
		PeerMSS: peerMSS,
		PeerMSSAdvertised: advertised,
		PacketPayloadMTU: packetPayloadMTU,
		CarrierPayloadMTU: carrierPayloadMTU,
		NegotiatedRecordWireLimit: cfg.RecordWireLimit,
		RecordWireMTU: recordWireMTU,
		RecordPayloadMTU: recordPayloadMTU,
		TLSLikeOverhead: tlsrecord.FixedWireOverhead,
		FECEnabled: cfg.ParityShards != 0,
		ParityShards: cfg.ParityShards,
		FECOverhead: fecOverhead,
		LinkFrameMTU: linkFrameMTU,
		LinkFragmentOverhead: LinkFragmentHeaderLen,
		LinkFragmentPayloadMTU: linkFrameMTU - LinkFragmentHeaderLen,
	}, nil
}

// MaxFECWireDatagram is the maximum FEC datagram presented to TLS-like record
// protection. With FEC off, the LINK frame itself occupies the record payload.
func (b Budget) MaxFECWireDatagram() int {
	if b.FECEnabled {
		return b.LinkFrameMTU + b.FECOverhead
	}
	return b.LinkFrameMTU
}

// OuterPacketLenForRecord returns the serialized IPv4 packet length for one
// steady-state record wire of n bytes under the headers used for this budget.
func (b Budget) OuterPacketLenForRecord(n int) int {
	return b.IPv4HeaderLen + b.TCPHeaderLen + n
}
