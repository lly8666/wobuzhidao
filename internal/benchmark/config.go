package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

const (
	EngineNativeTCP        = "native-tcp"
	EngineNativeUDP        = "native-udp"
	EngineWBDReal          = "wbd-real"
	EngineWBDFECExperiment = "wbd-fec-experimental"
	EngineUDP2RawSpeeder   = "udp2raw-udpspeeder"
)

var (
	ErrInvalidSweepSpec   = errors.New("invalid benchmark sweep spec")
	ErrCaseNotRunnable    = errors.New("benchmark case is not runnable in current harness")
	ErrUnsupportedWBDMode = errors.New("unsupported WBD mode in real fault harness")
)

// NetworkConfig is the human-facing millisecond/basis-point representation of
// RealFaultProfile. Keeping JSON free of Go duration strings makes hand-edited
// sweep files predictable across languages and tools.
type NetworkConfig struct {
	Name              string `json:"name"`
	Seed              uint64 `json:"seed"`
	Samples           int    `json:"samples"`
	PayloadBytes      int    `json:"payload_bytes"`
	RTTMinMS          int    `json:"rtt_min_ms"`
	RTTMaxMS          int    `json:"rtt_max_ms"`
	ImpairBasisPoints uint16 `json:"impair_basis_points"`
	ExtraHoldMS       int    `json:"extra_hold_ms"`
	SoftDeadlineMS    int    `json:"soft_deadline_ms"`
	SourceSpacingMS   int    `json:"source_spacing_ms"`
	BurstLength       int    `json:"burst_length,omitempty"`
}

// FECConfig describes an experiment/oracle FEC ratio. It is deliberately not a
// WBD wire type. WBD FEC stays experimental until real benchmark evidence
// admits one scheme and the protocol gets a separate wire decision.
type FECConfig struct {
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	DataShards   int    `json:"data_shards"`
	ParityShards int    `json:"parity_shards"`
	Mode         int    `json:"mode"`
	TimeoutMS    int    `json:"timeout_ms"`
	InterleaveMS int    `json:"interleave_ms,omitempty"`
}

func (f FECConfig) Multiplier() float64 {
	if !f.Enabled || f.DataShards <= 0 || f.ParityShards <= 0 {
		return 1
	}
	return float64(f.DataShards+f.ParityShards) / float64(f.DataShards)
}

func FECOff() FECConfig { return FECConfig{Name: "off"} }

func UDPspeederDefaultFEC() FECConfig {
	return FECConfig{Name: "udpspeeder-20-10", Enabled: true, DataShards: 20, ParityShards: 10, Mode: 0, TimeoutMS: 8}
}

func UDPspeederOneToOneFEC() FECConfig {
	return FECConfig{Name: "udpspeeder-20-20", Enabled: true, DataShards: 20, ParityShards: 20, Mode: 0, TimeoutMS: 8}
}

type SweepSpec struct {
	Name                  string          `json:"name"`
	Networks              []NetworkConfig `json:"networks"`
	LaneCounts            []int           `json:"lane_counts"`
	WBDModes              []string        `json:"wbd_modes"`
	Windows               []int           `json:"windows"`
	FECProfiles           []FECConfig     `json:"fec_profiles"`
	IncludeNative         bool            `json:"include_native"`
	IncludeUDP2RawSpeeder bool            `json:"include_udp2raw_udpspeeder"`
}

type OracleConfig struct {
	UDP2RawTag       string    `json:"udp2raw_tag"`
	UDP2RawCommit    string    `json:"udp2raw_commit"`
	UDPspeederTag    string    `json:"udpspeeder_tag"`
	UDPspeederCommit string    `json:"udpspeeder_commit"`
	RawMode          string    `json:"raw_mode"`
	FEC              FECConfig `json:"fec"`
}

func PinnedUDP2RawSpeederOracle(fec FECConfig) OracleConfig {
	return OracleConfig{
		UDP2RawTag:       "20230206.0",
		UDP2RawCommit:    "e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63",
		UDPspeederTag:    "20230206.0",
		UDPspeederCommit: "61b24a369700c3d8248dd18fa9a524b778741454",
		RawMode:          "faketcp",
		FEC:              fec,
	}
}

type ExperimentCase struct {
	ID            string        `json:"id"`
	Engine        string        `json:"engine"`
	Runnable      bool          `json:"runnable"`
	BlockedReason string        `json:"blocked_reason,omitempty"`
	Network       NetworkConfig `json:"network"`
	LaneCount     int           `json:"lane_count,omitempty"`
	WBDMode       string        `json:"wbd_mode,omitempty"`
	Window        int           `json:"window,omitempty"`
	FEC           FECConfig     `json:"fec"`
	Oracle        *OracleConfig `json:"oracle,omitempty"`
	CommandHint   string        `json:"command_hint,omitempty"`
}

func ParseSweepJSON(data []byte) (SweepSpec, error) {
	var spec SweepSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return SweepSpec{}, err
	}
	if err := spec.Validate(); err != nil {
		return SweepSpec{}, err
	}
	return spec, nil
}

func (s SweepSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" || len(s.Networks) == 0 || len(s.LaneCounts) == 0 || len(s.WBDModes) == 0 || len(s.Windows) == 0 || len(s.FECProfiles) == 0 {
		return ErrInvalidSweepSpec
	}
	for _, n := range s.Networks {
		if _, err := n.RealProfile(1, s.Windows[0]); err != nil {
			return fmt.Errorf("%w: network %q: %v", ErrInvalidSweepSpec, n.Name, err)
		}
	}
	for _, laneCount := range s.LaneCounts {
		if laneCount < 1 || laneCount > 16 {
			return fmt.Errorf("%w: lane_count=%d", ErrInvalidSweepSpec, laneCount)
		}
	}
	for _, window := range s.Windows {
		if window < 1 {
			return fmt.Errorf("%w: window=%d", ErrInvalidSweepSpec, window)
		}
	}
	for _, mode := range s.WBDModes {
		if _, err := parseRealWBDMode(mode); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSweepSpec, err)
		}
	}
	for _, fec := range s.FECProfiles {
		if err := validateFEC(fec); err != nil {
			return fmt.Errorf("%w: fec %q: %v", ErrInvalidSweepSpec, fec.Name, err)
		}
	}
	return nil
}

func validateFEC(f FECConfig) error {
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("missing name")
	}
	if !f.Enabled {
		return nil
	}
	if f.DataShards < 1 || f.ParityShards < 1 || f.Mode < 0 || f.Mode > 1 || f.TimeoutMS < 0 || f.InterleaveMS < 0 {
		return errors.New("invalid enabled FEC parameters")
	}
	if f.Multiplier() > 2.0 {
		return fmt.Errorf("multiplier %.3fx exceeds WBD 2.0x experiment ceiling", f.Multiplier())
	}
	return nil
}

func (n NetworkConfig) RealProfile(lanes, window int) (RealFaultProfile, error) {
	if strings.TrimSpace(n.Name) == "" || n.Samples < 4 || n.PayloadBytes < 4 || n.RTTMinMS <= 0 || n.RTTMaxMS < n.RTTMinMS || n.ExtraHoldMS < 0 || n.SoftDeadlineMS <= 0 || n.SourceSpacingMS < 0 || n.ImpairBasisPoints > 10000 {
		return RealFaultProfile{}, ErrInvalidRealFaultProfile
	}
	if n.RTTMinMS%2 != 0 || n.RTTMaxMS%2 != 0 {
		return RealFaultProfile{}, fmt.Errorf("%w: RTT min/max must be even milliseconds for symmetric one-way proxy", ErrInvalidRealFaultProfile)
	}
	burst := n.BurstLength
	if burst == 0 {
		burst = 1
	}
	if burst < 1 {
		return RealFaultProfile{}, ErrInvalidRealFaultProfile
	}
	p := RealFaultProfile{
		LaneCount:         lanes,
		Seed:              n.Seed,
		Samples:           n.Samples,
		PayloadBytes:      n.PayloadBytes,
		MinOneWay:         time.Duration(n.RTTMinMS/2) * time.Millisecond,
		MaxOneWay:         time.Duration(n.RTTMaxMS/2) * time.Millisecond,
		ImpairBasisPoints: n.ImpairBasisPoints,
		ExtraHold:         time.Duration(n.ExtraHoldMS) * time.Millisecond,
		SoftDeadline:      time.Duration(n.SoftDeadlineMS) * time.Millisecond,
		SourceSpacing:     time.Duration(n.SourceSpacingMS) * time.Millisecond,
		Window:            window,
		BurstLength:       burst,
	}
	if _, err := BuildRealFaultSchedule(p); err != nil {
		return RealFaultProfile{}, err
	}
	return p, nil
}

func ExpandSweep(spec SweepSpec) ([]ExperimentCase, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	var out []ExperimentCase
	for _, n := range spec.Networks {
		if spec.IncludeNative {
			for _, engine := range []string{EngineNativeTCP, EngineNativeUDP} {
				out = append(out, ExperimentCase{ID: caseID(spec.Name, n.Name, engine), Engine: engine, Runnable: true, Network: n, LaneCount: 1, Window: 1, FEC: FECOff()})
			}
		}
		for _, lanes := range spec.LaneCounts {
			for _, mode := range spec.WBDModes {
				for _, window := range spec.Windows {
					for _, fec := range spec.FECProfiles {
						c := ExperimentCase{Network: n, LaneCount: lanes, WBDMode: mode, Window: window, FEC: fec}
						c.ID = caseID(spec.Name, n.Name, fmt.Sprintf("%dlane", lanes), mode, fmt.Sprintf("w%d", window), fec.Name)
						if fec.Enabled {
							c.Engine = EngineWBDFECExperiment
							c.BlockedReason = "FEC is generated as an experimental case but is not yet wired into the real WBD carrier path"
						} else {
							c.Engine = EngineWBDReal
							c.Runnable = true
						}
						out = append(out, c)
					}
				}
			}
		}
		if spec.IncludeUDP2RawSpeeder {
			for _, fec := range spec.FECProfiles {
				if !fec.Enabled {
					continue
				}
				oracle := PinnedUDP2RawSpeederOracle(fec)
				out = append(out, ExperimentCase{
					ID:            caseID(spec.Name, n.Name, "udp2raw", fec.Name),
					Engine:        EngineUDP2RawSpeeder,
					Runnable:      false,
					BlockedReason: "pinned external binaries must be restored and SHA-qualified in the local sandbox before execution",
					Network:       n,
					FEC:           fec,
					Oracle:        &oracle,
					CommandHint:   oracleCommandHint(oracle),
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func RunExperimentCase(ctx context.Context, c ExperimentCase) (RealFaultObservation, error) {
	if !c.Runnable || c.FEC.Enabled {
		return RealFaultObservation{}, fmt.Errorf("%w: %s", ErrCaseNotRunnable, c.BlockedReason)
	}
	p, err := c.Network.RealProfile(c.LaneCount, c.Window)
	if err != nil {
		return RealFaultObservation{}, err
	}
	sched, err := BuildRealFaultSchedule(p)
	if err != nil {
		return RealFaultObservation{}, err
	}
	switch c.Engine {
	case EngineNativeTCP:
		return RunRealFaultTCP(ctx, p, sched)
	case EngineNativeUDP:
		return RunRealFaultUDP(ctx, p, sched)
	case EngineWBDReal:
		mode, err := parseRealWBDMode(c.WBDMode)
		if err != nil {
			return RealFaultObservation{}, err
		}
		return RunRealFaultWBD(ctx, p, sched, mode)
	default:
		return RealFaultObservation{}, fmt.Errorf("%w: engine=%s", ErrCaseNotRunnable, c.Engine)
	}
}

func parseRealWBDMode(s string) (rbc.ProtectionMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "normal":
		return rbc.ModeNormal, nil
	case "auto":
		return rbc.ModeAuto, nil
	default:
		return rbc.ModeNormal, fmt.Errorf("%w: %q (real harness currently accepts normal/auto; fixed 1.5x/2.0x needs an explicit protection spender)", ErrUnsupportedWBDMode, s)
	}
}

func caseID(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		p = strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, "__")
}

func oracleCommandHint(o OracleConfig) string {
	fec := fmt.Sprintf("-f%d:%d --mode %d --timeout %d", o.FEC.DataShards, o.FEC.ParityShards, o.FEC.Mode, o.FEC.TimeoutMS)
	if o.FEC.InterleaveMS > 0 {
		fec += fmt.Sprintf(" -i %d", o.FEC.InterleaveMS)
	}
	return fmt.Sprintf("app -> speederv2 (%s) -> udp2raw (--raw-mode %s) -> internet -> udp2raw -> speederv2 -> app", fec, o.RawMode)
}
