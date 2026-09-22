package datapath

import (
	"errors"
	"fmt"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

var ErrAdmissionHandoff = errors.New("datapath: invalid admission handoff")

const (
	DefaultFECFlushAfter = 8 * time.Millisecond
	DefaultFECMaxBlocks  = 8
)

// FixedFECRuntimeDefaults returns the single active runtime timing/bound set
// used when an operator explicitly enables one of the already-qualified fixed
// 20:R profiles. Parity 0 remains production-default FEC off.
func FixedFECRuntimeDefaults(parity int) (time.Duration, int, error) {
	if parity == 0 {
		return 0, 0, nil
	}
	if !fec.IsSupportedParityShards(parity) {
		return 0, 0, fmt.Errorf("%w: unsupported fixed FEC 20:%d", ErrAdmissionHandoff, parity)
	}
	return DefaultFECFlushAfter, DefaultFECMaxBlocks, nil
}

type ClientLaneParams struct {
	ConnectionMTU  int
	LocalPacketMTU int

	TxIPv4HeaderLen int
	TxTCPHeaderLen  int
	RxIPv4HeaderLen int
	RxTCPHeaderLen  int

	PeerMSS    uint16
	PeerMSSSet bool

	ParityShards int
	FlushAfter   time.Duration
	MaxBlocks    int
}

// ClientLaneConfigFromAdmission binds immutable protected-admission results to
// the client side of the same FakeTCP association. The caller supplies the
// SYN-ACK peer MSS observation; receive MTU is bounded by the MSS that this WBD
// client advertised on its SYN.
func ClientLaneConfigFromAdmission(session *realityfront.ClientAdmissionSession, params ClientLaneParams) (LaneConfig, error) {
	if session == nil || session.Negotiated.RecordVersion != realityfront.RecordVersionV1 {
		return LaneConfig{}, ErrAdmissionHandoff
	}
	n := session.Negotiated
	peerMSS := params.PeerMSS
	if !params.PeerMSSSet {
		peerMSS = faketcp.DefaultIPv4PeerMSS
	}
	ipHeaderLen := faketcp.SteadyIPv4HeaderLen()
	tcpHeaderLen := faketcp.SteadyDataTCPHeaderLen()
	for name, configured := range map[string]int{
		"tx_ipv4": params.TxIPv4HeaderLen,
		"rx_ipv4": params.RxIPv4HeaderLen,
	} {
		if configured != 0 && configured != ipHeaderLen {
			return LaneConfig{}, fmt.Errorf("%w: %s header=%d actual=%d", ErrAdmissionHandoff, name, configured, ipHeaderLen)
		}
	}
	for name, configured := range map[string]int{
		"tx_tcp": params.TxTCPHeaderLen,
		"rx_tcp": params.RxTCPHeaderLen,
	} {
		if configured != 0 && configured != tcpHeaderLen {
			return LaneConfig{}, fmt.Errorf("%w: %s header=%d actual=%d", ErrAdmissionHandoff, name, configured, tcpHeaderLen)
		}
	}

	tx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   ipHeaderLen,
		TCPHeaderLen:    tcpHeaderLen,
		PeerMSS:         peerMSS,
		PeerMSSSet:      params.PeerMSSSet,
		RecordWireLimit: int(n.ServerLimit),
		ParityShards:    params.ParityShards,
	}
	rx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   ipHeaderLen,
		TCPHeaderLen:    tcpHeaderLen,
		PeerMSS:         faketcp.DefaultMSS,
		PeerMSSSet:      true,
		RecordWireLimit: int(n.ClientLimit),
		ParityShards:    params.ParityShards,
	}

	cfg := LaneConfig{
		Role:              RoleClient,
		IncarnationNonce:  n.IncarnationNonce,
		TunnelID:          append([]byte(nil), n.TunnelID...),
		ClientRecordLimit: int(n.ClientLimit),
		ServerRecordLimit: int(n.ServerLimit),
		Keys:              n.Keys,
		ParityShards:      params.ParityShards,
		FlushAfter:        params.FlushAfter,
		MaxBlocks:         params.MaxBlocks,
		TxMTU:             tx,
		RxMTU:             rx,
	}
	probe, err := NewLane(cfg)
	if err != nil {
		return LaneConfig{}, fmt.Errorf("%w: %v", ErrAdmissionHandoff, err)
	}
	probe.Close()
	return cfg, nil
}

type ServerLaneParams struct {
	ConnectionMTU  int
	LocalPacketMTU int

	TxIPv4HeaderLen int
	TxTCPHeaderLen  int
	RxIPv4HeaderLen int
	RxTCPHeaderLen  int

	ParityShards int
	FlushAfter   time.Duration
	MaxBlocks    int
}

// ServerLaneConfigFromAdmission binds immutable protected-admission results to
// the existing FakeTCP association without modifying P2 ownership. The server
// sends under ClientLimit/S2C and receives under ServerLimit/C2S.
func ServerLaneConfigFromAdmission(session *realityfront.ServerAdmissionSession, assoc *faketcp.ServerAssociation, params ServerLaneParams) (LaneConfig, error) {
	if session == nil || assoc == nil || session.Negotiated.RecordVersion != realityfront.RecordVersionV1 {
		return LaneConfig{}, ErrAdmissionHandoff
	}
	n := session.Negotiated
	peer := assoc.PeerTCPProfile()
	ipHeaderLen := faketcp.SteadyIPv4HeaderLen()
	tcpHeaderLen := faketcp.SteadyDataTCPHeaderLen()
	for name, configured := range map[string]int{
		"tx_ipv4": params.TxIPv4HeaderLen,
		"rx_ipv4": params.RxIPv4HeaderLen,
	} {
		if configured != 0 && configured != ipHeaderLen {
			return LaneConfig{}, fmt.Errorf("%w: %s header=%d actual=%d", ErrAdmissionHandoff, name, configured, ipHeaderLen)
		}
	}
	for name, configured := range map[string]int{
		"tx_tcp": params.TxTCPHeaderLen,
		"rx_tcp": params.RxTCPHeaderLen,
	} {
		if configured != 0 && configured != tcpHeaderLen {
			return LaneConfig{}, fmt.Errorf("%w: %s header=%d actual=%d", ErrAdmissionHandoff, name, configured, tcpHeaderLen)
		}
	}

	tx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   ipHeaderLen,
		TCPHeaderLen:    tcpHeaderLen,
		PeerMSS:         peer.MSS,
		PeerMSSSet:      peer.AdvertisedMSS,
		RecordWireLimit: int(n.ClientLimit),
		ParityShards:    params.ParityShards,
	}
	// Client->server is constrained by the MSS advertised by the server on
	// SYN-ACK. The current extracted P2 server persona advertises DefaultMSS.
	rx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   ipHeaderLen,
		TCPHeaderLen:    tcpHeaderLen,
		PeerMSS:         faketcp.DefaultMSS,
		PeerMSSSet:      true,
		RecordWireLimit: int(n.ServerLimit),
		ParityShards:    params.ParityShards,
	}

	cfg := LaneConfig{
		Role:              RoleServer,
		IncarnationNonce:  n.IncarnationNonce,
		TunnelID:          append([]byte(nil), n.TunnelID...),
		ClientRecordLimit: int(n.ClientLimit),
		ServerRecordLimit: int(n.ServerLimit),
		Keys:              n.Keys,
		ParityShards:      params.ParityShards,
		FlushAfter:        params.FlushAfter,
		MaxBlocks:         params.MaxBlocks,
		TxMTU:             tx,
		RxMTU:             rx,
	}
	probe, err := NewLane(cfg)
	if err != nil {
		return LaneConfig{}, fmt.Errorf("%w: %v", ErrAdmissionHandoff, err)
	}
	probe.Close()
	return cfg, nil
}

// InboundTransition consumes the bounded copied payloads returned by P2
// DetachTransition. Seq is intentionally not turned into record ordering;
// tlsrecord PN and FEC/LINK identities remain independent.
func (l *Lane) InboundTransition(packets []faketcp.TransitionPacket, now time.Time) (InboundResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return InboundResult{}, ErrLaneClosed
	}
	var out InboundResult
	for _, packet := range packets {
		l.stats.InboundPayloads++
		item := l.inboundLocked(packet.Payload, now)
		out.Datagrams = append(out.Datagrams, item.Datagrams...)
		out.RecordErrors = append(out.RecordErrors, item.RecordErrors...)
		out.PathErrors = append(out.PathErrors, item.PathErrors...)
	}
	return out, nil
}
