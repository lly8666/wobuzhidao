package datapath

import (
	"errors"
	"fmt"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

var ErrAdmissionHandoff = errors.New("datapath: invalid admission handoff")

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

	tx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   params.TxIPv4HeaderLen,
		TCPHeaderLen:    params.TxTCPHeaderLen,
		PeerMSS:         peerMSS,
		PeerMSSSet:      params.PeerMSSSet,
		RecordWireLimit: int(n.ServerLimit),
		ParityShards:    params.ParityShards,
	}
	rx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   params.RxIPv4HeaderLen,
		TCPHeaderLen:    params.RxTCPHeaderLen,
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

	tx := pathmtu.Config{
		ConnectionMTU:   params.ConnectionMTU,
		LocalPacketMTU:  params.LocalPacketMTU,
		IPv4HeaderLen:   params.TxIPv4HeaderLen,
		TCPHeaderLen:    params.TxTCPHeaderLen,
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
		IPv4HeaderLen:   params.RxIPv4HeaderLen,
		TCPHeaderLen:    params.RxTCPHeaderLen,
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
