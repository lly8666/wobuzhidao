package gamecontrol

import (
	"errors"
	"fmt"
	"math"

	"github.com/lly8666/wobuzhidao/internal/linkpolicy"
)

type FECProfile string

const (
	FECOff   FECProfile = "off"
	FEC20x20 FECProfile = "20:20"
)

type Config struct {
	LinkSpeedMode       linkpolicy.LinkSpeedMode
	ManualLinkSpeedMbps float64

	RequestedLanes int
	AutoAddLanes   bool
	MaxLanes       int
	RaceTarget     float64

	FEC FECProfile

	PayloadBytes int
	FramingBytes int
	MaxWireUtil  float64
}

type Measurement struct {
	// AutoLinkSpeedMbps must come from the service-capacity estimator, not
	// application goodput. In manual mode it is retained only for diagnostics
	// and may be zero when no automatic estimate exists yet.
	AutoLinkSpeedMbps float64
	Loss              float64
	MeanBurst         float64
	CarrierExpansion  float64
}

type Plan struct {
	LinkSpeedMode        linkpolicy.LinkSpeedMode `json:"link_speed_mode"`
	AutoLinkSpeedMbps    float64                  `json:"auto_link_speed_mbps"`
	ManualLinkSpeedMbps  float64                  `json:"manual_link_speed_mbps,omitempty"`
	EffectiveLinkMbps    float64                  `json:"effective_link_mbps"`
	RequestedLanes       int                      `json:"requested_lanes"`
	ActiveLanes          int                      `json:"active_lanes"`
	AutoLaneAdded        bool                     `json:"auto_lane_added"`
	FEC                   FECProfile               `json:"fec"`
	PerLaneWireExpansion float64                  `json:"per_lane_wire_expansion"`
	TotalWireExpansion   float64                  `json:"total_wire_expansion"`
	InnerCeilingMbps     float64                  `json:"inner_ceiling_mbps"`
	Loss                  float64                  `json:"loss"`
	MeanBurst             float64                  `json:"mean_burst"`
	CarrierExpansion      float64                  `json:"carrier_expansion"`
}

func DefaultConfig() Config {
	return Config{
		LinkSpeedMode: linkpolicy.LinkSpeedAuto,
		RequestedLanes: 2,
		AutoAddLanes: false,
		MaxLanes: linkpolicy.MaxGameLanes,
		RaceTarget: 0.9995,
		FEC: FECOff,
		PayloadBytes: 1200,
		FramingBytes: 56,
		MaxWireUtil: 0.92,
	}
}

// BuildPlan is the single authority for game lane count and logical inner
// pacing. Game mode never changes into a different mode: loss/burst may cause
// auto-add to raise the lane count and FEC may be independently configured,
// but neither can replace the game race data plane.
func BuildPlan(cfg Config, m Measurement) (Plan, error) {
	if err := validate(cfg, m); err != nil {
		return Plan{}, err
	}
	obs := linkpolicy.DefaultObservation(m.AutoLinkSpeedMbps, m.Loss, m.MeanBurst)
	obs.LinkSpeedMode = cfg.LinkSpeedMode
	obs.ManualLinkSpeedMbps = cfg.ManualLinkSpeedMbps
	obs.CarrierExpansion = m.CarrierExpansion
	obs.PayloadBytes = cfg.PayloadBytes
	obs.FramingBytes = cfg.FramingBytes
	obs.MaxWireUtil = cfg.MaxWireUtil
	obs.GameRequestedLanes = cfg.RequestedLanes
	obs.GameAutoAddLanes = cfg.AutoAddLanes
	obs.GameMaxLanes = cfg.MaxLanes
	obs.GameRaceTarget = cfg.RaceTarget

	rec, err := linkpolicy.Recommend(obs, linkpolicy.ModeGame)
	if err != nil {
		return Plan{}, err
	}
	fecExpansion := 1.0
	switch cfg.FEC {
	case FECOff:
	case FEC20x20:
		fecExpansion = 2
	default:
		return Plan{}, fmt.Errorf("gamecontrol: unsupported live FEC %q", cfg.FEC)
	}
	framingExpansion := float64(cfg.PayloadBytes+cfg.FramingBytes) / float64(cfg.PayloadBytes)
	perLane := framingExpansion * fecExpansion * m.CarrierExpansion
	total := perLane * float64(rec.LaneCount)
	inner := rec.EffectiveCapacityMbps * cfg.MaxWireUtil / total

	return Plan{
		LinkSpeedMode: cfg.LinkSpeedMode,
		AutoLinkSpeedMbps: m.AutoLinkSpeedMbps,
		ManualLinkSpeedMbps: cfg.ManualLinkSpeedMbps,
		EffectiveLinkMbps: rec.EffectiveCapacityMbps,
		RequestedLanes: cfg.RequestedLanes,
		ActiveLanes: rec.LaneCount,
		AutoLaneAdded: rec.AutoLaneAdded,
		FEC: cfg.FEC,
		PerLaneWireExpansion: perLane,
		TotalWireExpansion: total,
		InnerCeilingMbps: inner,
		Loss: m.Loss,
		MeanBurst: m.MeanBurst,
		CarrierExpansion: m.CarrierExpansion,
	}, nil
}

func validate(cfg Config, m Measurement) error {
	if cfg.LinkSpeedMode != linkpolicy.LinkSpeedAuto && cfg.LinkSpeedMode != linkpolicy.LinkSpeedManual {
		return errors.New("gamecontrol: link speed mode must be auto or manual")
	}
	if math.IsNaN(m.AutoLinkSpeedMbps) || math.IsInf(m.AutoLinkSpeedMbps, 0) || m.AutoLinkSpeedMbps < 0 {
		return errors.New("gamecontrol: auto link speed observation must be finite and non-negative")
	}
	if cfg.LinkSpeedMode == linkpolicy.LinkSpeedAuto && m.AutoLinkSpeedMbps <= 0 {
		return errors.New("gamecontrol: auto link speed observation must be positive in auto mode")
	}
	if cfg.LinkSpeedMode == linkpolicy.LinkSpeedManual && (cfg.ManualLinkSpeedMbps <= 0 || math.IsNaN(cfg.ManualLinkSpeedMbps) || math.IsInf(cfg.ManualLinkSpeedMbps, 0)) {
		return errors.New("gamecontrol: manual link speed must be finite and positive")
	}
	if cfg.RequestedLanes < 1 || cfg.RequestedLanes > linkpolicy.MaxGameLanes || cfg.MaxLanes < cfg.RequestedLanes || cfg.MaxLanes > linkpolicy.MaxGameLanes {
		return errors.New("gamecontrol: lanes must satisfy 1 <= requested <= max <= 4")
	}
	if cfg.RaceTarget <= 0 || cfg.RaceTarget >= 1 {
		return errors.New("gamecontrol: race target must be in (0,1)")
	}
	if cfg.PayloadBytes <= 0 || cfg.FramingBytes < 0 || cfg.MaxWireUtil <= 0 || cfg.MaxWireUtil > 1 {
		return errors.New("gamecontrol: invalid framing/utilization")
	}
	if m.Loss < 0 || m.Loss >= 1 || m.MeanBurst < 1 || m.CarrierExpansion < 1 {
		return errors.New("gamecontrol: invalid link measurement")
	}
	if cfg.FEC != FECOff && cfg.FEC != FEC20x20 {
		return fmt.Errorf("gamecontrol: unsupported live FEC %q", cfg.FEC)
	}
	return nil
}
