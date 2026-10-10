package runtimeentry

import (
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
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
	PreAttachDrops uint64                            `json:"pre_attach_drops,omitempty"` // shared server admission drops
	Lifecycle      *LifecycleStats                   `json:"lifecycle,omitempty"`
	Config         *LifecycleConfigDiagnostic        `json:"config,omitempty"`
	ServerPipeline *ServerPipelineDiagnostic         `json:"server_pipeline,omitempty"`
	ClientPipeline *ClientPipelineDiagnostic         `json:"client_pipeline,omitempty"`
	ServerUDP      *platformflow.UDPServerDiagnostic `json:"server_udp,omitempty"`
	Lease4         string                            `json:"lease4,omitempty"`
	// Actual authenticated admission, not the requested CLI setting; optional
	// diagnostic only. Never include identity credentials or exporter material.
	RecordVersion  uint16                            `json:"record_version,omitempty"`
	AdmissionPolicy *realityfront.AdmissionPolicy     `json:"admission_policy,omitempty"`
	TunnelID       logicaltunnel.TunnelID            `json:"tunnel_id"`
	Owner          datapath.TunnelOwnerStats         `json:"owner"`
	Lanes          []LaneDiagnostic                  `json:"lanes"`
	RetiringLanes  []LaneDiagnostic                  `json:"retiring_lanes,omitempty"`
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
	for _, lane := range owner.RetiringLanes() {
		laneStats, laneOK := owner.RetiringLaneStats(lane.Ref)
		transportStats, transportOK := rt.TransportStatsAt(lane.Ref, now)
		if laneOK && transportOK {
			out.RetiringLanes = append(out.RetiringLanes, LaneDiagnostic{
				Ref: lane.Ref, ParityShards: lane.ParityShards, Lane: laneStats, Transport: transportStats,
			})
		}
	}
	return out
}

func (c *TunnelClient) DiagnosticSnapshot(now time.Time) TunnelDiagnostic {
	if c == nil {
		return TunnelDiagnostic{}
	}
	out := diagnosticSnapshot(c.owner, c.rt, now)
	c.mu.Lock()
	out.RecordVersion = c.negotiatedRecordVersion
	if c.negotiatedRecordVersion == realityfront.RecordVersionV3 {
		policy := c.negotiatedPolicy
		out.AdmissionPolicy = &policy
	}
	c.mu.Unlock()
	if c.cfg.ObserveTiming {
		pipeline := c.pipeline.snapshot()
		out.ClientPipeline = &pipeline
	}
	stats := c.LifecycleStats()
	out.Lifecycle = &stats
	out.Config = &LifecycleConfigDiagnostic{
		KeepaliveInterval: c.cfg.KeepaliveInterval.String(),
		DeadAfter:         c.cfg.DeadAfter.String(),
		ReconnectMin:      c.cfg.ReconnectMin.String(),
		ReconnectMax:      c.cfg.ReconnectMax.String(),
		DormantAfter:      c.cfg.DormantAfter.String(),
		RotateMin:         c.cfg.RotateMin.String(),
		RotateMax:         c.cfg.RotateMax.String(),
		ReplacementGrace:  c.cfg.ReplacementGrace.String(),
	}
	return out
}

func (s *LifecycleServer) TunnelDiagnosticSnapshot(id logicaltunnel.TunnelID, now time.Time) (TunnelDiagnostic, bool) {
	if s == nil {
		return TunnelDiagnostic{}, false
	}
	s.mu.Lock()
	group := s.byTunnel[id]
	preAttachDrops := s.preAttachDrops
	var recordVersion uint16
	var policy realityfront.AdmissionPolicy
	if group != nil {
		recordVersion = group.admissionVersion
		policy = group.admissionPolicy
	}
	s.mu.Unlock()
	if group == nil {
		return TunnelDiagnostic{}, false
	}
	out := diagnosticSnapshot(group.owner, group.rt, now)
	out.PreAttachDrops = preAttachDrops
	out.RecordVersion = recordVersion
	if recordVersion == realityfront.RecordVersionV3 {
		out.AdmissionPolicy = &policy
	}
	if s.cfg.ObserveTiming {
		pipeline := s.pipeline.snapshot(true)
		out.ServerPipeline = &pipeline
	}
	if group.service != nil {
		udp := group.service.UDPDiagnostic()
		out.ServerUDP = &udp
	}
	out.Config = &LifecycleConfigDiagnostic{
		KeepaliveInterval: s.cfg.KeepaliveInterval.String(),
		DormantAfter:      s.cfg.DormantAfter.String(),
		ReplacementGrace:  s.cfg.ReplacementGrace.String(),
	}
	return out, true
}

// Automatic admissions have no single configured TunnelID. Enumerate only for
// opt-in diagnostics; steady packet handling never scans all clients.
func (s *LifecycleServer) TunnelDiagnosticSnapshots(now time.Time) []TunnelDiagnostic {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	ids := make([]logicaltunnel.TunnelID, 0, len(s.byTunnel))
	for id := range s.byTunnel {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Slice(ids, func(i, j int) bool { return string(ids[i][:]) < string(ids[j][:]) })
	out := make([]TunnelDiagnostic, 0, len(ids))
	for _, id := range ids {
		if snapshot, ok := s.TunnelDiagnosticSnapshot(id, now); ok {
			out = append(out, snapshot)
		}
	}
	return out
}

func (m *SegmentMux) DiagnosticSnapshot() SegmentMuxDiagnostic {
	if m == nil {
		return SegmentMuxDiagnostic{}
	}
	out := SegmentMuxDiagnostic{Enabled: m.diagnostics.Load()}
	m.mu.Lock()
	for flow, route := range m.routes {
		out.Routes = append(out.Routes, route.timing.snapshot(flow, cap(route.in)))
	}
	m.mu.Unlock()
	sort.Slice(out.Routes, func(i, j int) bool {
		if out.Routes[i].LocalPort != out.Routes[j].LocalPort {
			return out.Routes[i].LocalPort < out.Routes[j].LocalPort
		}
		return out.Routes[i].PeerPort < out.Routes[j].PeerPort
	})
	return out
}
