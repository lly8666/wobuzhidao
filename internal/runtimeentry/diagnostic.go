package runtimeentry

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/runtimeowner"
)

type LaneDiagnostic struct {
	Ref          logicaltunnel.LaneRef       `json:"ref"`
	ParityShards int                         `json:"parity_shards"`
	Lane         datapath.LaneStats          `json:"lane"`
	Transport    runtimeowner.TransportStats `json:"transport"`
}

type LifecycleConfigDiagnostic struct {
	KeepaliveInterval string `json:"keepalive_interval,omitempty"`
	DeadAfter         string `json:"dead_after,omitempty"`
	ReconnectMin      string `json:"reconnect_min,omitempty"`
	ReconnectMax      string `json:"reconnect_max,omitempty"`
	DormantAfter      string `json:"dormant_after,omitempty"`
	RotateMin         string `json:"rotate_min,omitempty"`
	RotateMax         string `json:"rotate_max,omitempty"`
	ReplacementGrace  string `json:"replacement_grace,omitempty"`
}

type TunnelDiagnostic struct {
	Lifecycle *LifecycleStats            `json:"lifecycle,omitempty"`
	Config    *LifecycleConfigDiagnostic `json:"config,omitempty"`
	Lease4    string                     `json:"lease4,omitempty"`
	TunnelID  logicaltunnel.TunnelID     `json:"tunnel_id"`
	Owner     datapath.TunnelOwnerStats  `json:"owner"`
	Lanes     []LaneDiagnostic           `json:"lanes"`
}

func diagnosticSnapshot(owner *datapath.TunnelOwner, rt *runtimeowner.Runtime, now time.Time) TunnelDiagnostic {
	if owner == nil || rt == nil {
		return TunnelDiagnostic{}
	}
	out := TunnelDiagnostic{Owner: owner.Stats()}
	if lease, ok := owner.Lease(); ok {
		out.Lease4 = lease.Config.Address4
		out.TunnelID = lease.Config.TunnelID
	}
	for _, lane := range owner.ActiveLanes() {
		laneStats, laneOK := owner.LaneStats(lane.Ref)
		transportStats, transportOK := rt.TransportStatsAt(lane.Ref, now)
		if !laneOK || !transportOK {
			continue
		}
		out.Lanes = append(out.Lanes, LaneDiagnostic{
			Ref: lane.Ref, ParityShards: lane.ParityShards, Lane: laneStats, Transport: transportStats,
		})
	}
	return out
}

func (c *TunnelClient) DiagnosticSnapshot(now time.Time) TunnelDiagnostic {
	if c == nil {
		return TunnelDiagnostic{}
	}
	out := diagnosticSnapshot(c.owner, c.rt, now)
	stats := c.LifecycleStats()
	out.Lifecycle = &stats
	out.Config = &LifecycleConfigDiagnostic{
		KeepaliveInterval: c.cfg.KeepaliveInterval.String(),
		DeadAfter: c.cfg.DeadAfter.String(),
		ReconnectMin: c.cfg.ReconnectMin.String(),
		ReconnectMax: c.cfg.ReconnectMax.String(),
		DormantAfter: c.cfg.DormantAfter.String(),
		RotateMin: c.cfg.RotateMin.String(),
		RotateMax: c.cfg.RotateMax.String(),
		ReplacementGrace: c.cfg.ReplacementGrace.String(),
	}
	return out
}

func (s *LifecycleServer) TunnelDiagnosticSnapshot(id logicaltunnel.TunnelID, now time.Time) (TunnelDiagnostic, bool) {
	if s == nil {
		return TunnelDiagnostic{}, false
	}
	s.mu.Lock()
	group := s.byTunnel[id]
	s.mu.Unlock()
	if group == nil {
		return TunnelDiagnostic{}, false
	}
	out := diagnosticSnapshot(group.owner, group.rt, now)
	out.Config = &LifecycleConfigDiagnostic{
		KeepaliveInterval: s.cfg.KeepaliveInterval.String(),
		DormantAfter: s.cfg.DormantAfter.String(),
		ReplacementGrace: s.cfg.ReplacementGrace.String(),
	}
	return out, true
}
