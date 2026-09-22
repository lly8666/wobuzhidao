package runtimeentry

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/runtimeowner"
)

type LaneDiagnostic struct {
	Ref       logicaltunnel.LaneRef       `json:"ref"`
	Lane      datapath.LaneStats          `json:"lane"`
	Transport runtimeowner.TransportStats `json:"transport"`
}

type TunnelDiagnostic struct {
	Lifecycle *LifecycleStats           `json:"lifecycle,omitempty"`
	Owner     datapath.TunnelOwnerStats `json:"owner"`
	Lanes     []LaneDiagnostic          `json:"lanes"`
}

func diagnosticSnapshot(owner *datapath.TunnelOwner, rt *runtimeowner.Runtime, now time.Time) TunnelDiagnostic {
	if owner == nil || rt == nil {
		return TunnelDiagnostic{}
	}
	out := TunnelDiagnostic{Owner: owner.Stats()}
	for _, lane := range owner.ActiveLanes() {
		laneStats, laneOK := owner.LaneStats(lane.Ref)
		transportStats, transportOK := rt.TransportStatsAt(lane.Ref, now)
		if !laneOK || !transportOK {
			continue
		}
		out.Lanes = append(out.Lanes, LaneDiagnostic{
			Ref: lane.Ref, Lane: laneStats, Transport: transportStats,
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
	return diagnosticSnapshot(group.owner, group.rt, now), true
}
