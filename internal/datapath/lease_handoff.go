package datapath

import (
	"bytes"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

// ServerLaneConfigFromLeasedAdmission binds protected admission to the stable
// Logical Tunnel identity before the Lane incarnation is constructed. A client
// cannot present another installation's TunnelID and then attach that transport
// to this owner.
func ServerLaneConfigFromLeasedAdmission(
	session *realityfront.ServerAdmissionSession,
	assoc *faketcp.ServerAssociation,
	lease logicaltunnel.Lease,
	params ServerLaneParams,
) (LaneConfig, error) {
	if session == nil {
		return LaneConfig{}, ErrAdmissionHandoff
	}
	if err := lease.Validate(); err != nil {
		return LaneConfig{}, ErrAdmissionHandoff
	}
	if !bytes.Equal(session.Negotiated.TunnelID, lease.Config.TunnelID.Bytes()) {
		return LaneConfig{}, ErrTunnelMismatch
	}
	return ServerLaneConfigFromAdmission(session, assoc, params)
}
