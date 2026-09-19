package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/linkpolicy"
)

type row struct {
	CapacityMbps float64 `json:"capacity_mbps"`
	LossPct      float64 `json:"loss_pct"`
	RTTMS        float64 `json:"rtt_ms"`
	BurstLength  int     `json:"burst_length"`
	Mode         string  `json:"mode"`
	FEC          string  `json:"fec"`
	Parity       int     `json:"parity"`
	InnerMbps    float64 `json:"inner_mbps"`
	Predicted    float64 `json:"predicted_delivery"`
	ObservedMin  float64 `json:"observed_delivery_min"`
	ObservedP99  float64 `json:"observed_p99_max_ms"`
	WireUtilMax  float64 `json:"wire_util_max"`
	Claimed      bool    `json:"claimed_target"`
	ObservedPass bool    `json:"observed_pass"`
}

type summary struct {
	Schema string `json:"schema"`
	Authority string `json:"authority"`
	Rows []row `json:"rows"`
	BalancedClaims int `json:"balanced_claims"`
	BalancedObservedPass int `json:"balanced_observed_pass"`
	ConservativeClaims int `json:"conservative_claims"`
	ConservativeObservedPass int `json:"conservative_observed_pass"`
	TargetUnmet int `json:"target_unmet"`
	GameLaneEligible int `json:"game_lane_eligible"`
}

func main() {
	capsRaw := flag.String("capacities", "5,10,20,30,50,75,100,125,150", "capacity Mbit/s")
	lossRaw := flag.String("loss", "0,1,3,5,10,15,20,30", "loss percent")
	rttRaw := flag.String("rtt-ms", "20,100", "RTT ms")
	burstRaw := flag.String("burst-lengths", "1,4", "mean loss burst")
	seedsRaw := flag.String("seeds", "260826,260827,260828", "simulation seeds")
	samples := flag.Int("samples", 500, "source packets per seed")
	outPath := flag.String("out", "", "optional JSON output")
	flag.Parse()

	caps := floats(*capsRaw)
	losses := floats(*lossRaw)
	rtts := floats(*rttRaw)
	bursts := ints(*burstRaw)
	seeds := int64s(*seedsRaw)
	if *samples <= 0 || len(caps) == 0 || len(losses) == 0 || len(rtts) == 0 || len(bursts) == 0 || len(seeds) == 0 {
		fatal("invalid empty/numeric grid")
	}

	out := summary{Schema: "wbd-link-policy-formula-calibration/v1", Authority: "simulator_calibration_only_not_release_authority"}
	for _, capMbps := range caps {
		for _, lossPct := range losses {
			for _, rtt := range rtts {
				for _, burst := range bursts {
					base := linkpolicy.DefaultObservation(capMbps, lossPct/100, float64(burst))
					game, err := linkpolicy.Recommend(base, linkpolicy.ModeGame)
					if err != nil { fatal(err.Error()) }
					if game.GameLaneEligible { out.GameLaneEligible++ }
					for _, mode := range []linkpolicy.Mode{linkpolicy.ModeBalanced, linkpolicy.ModeConservative} {
						rec, err := linkpolicy.Recommend(base, mode)
						if err != nil { fatal(err.Error()) }
						r := runPoint(capMbps, lossPct, rtt, burst, mode, rec, seeds, *samples, base.TargetDelivery, base.MaxWireUtil)
						out.Rows = append(out.Rows, r)
						if !rec.MeetsTarget {
							out.TargetUnmet++
							continue
						}
						if mode == linkpolicy.ModeBalanced {
							out.BalancedClaims++
							if r.ObservedPass { out.BalancedObservedPass++ }
						} else {
							out.ConservativeClaims++
							if r.ObservedPass { out.ConservativeObservedPass++ }
						}
					}
				}
			}
		}
	}

	if *outPath != "" {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil { fatal(err.Error()) }
		if err := os.WriteFile(*outPath, append(b, '\n'), 0o644); err != nil { fatal(err.Error()) }
	}
	fmt.Printf("WBD_LINK_POLICY_FORMULA_PASS rows=%d balanced=%d/%d conservative=%d/%d target_unmet=%d game_lane_eligible=%d authority=simulator_calibration\n",
		len(out.Rows), out.BalancedObservedPass, out.BalancedClaims, out.ConservativeObservedPass, out.ConservativeClaims, out.TargetUnmet, out.GameLaneEligible)
}

func runPoint(capMbps, lossPct, rtt float64, burst int, mode linkpolicy.Mode, rec linkpolicy.Recommendation, seeds []int64, samples int, target, maxUtil float64) row {
	r := row{
		CapacityMbps: capMbps, LossPct: lossPct, RTTMS: rtt, BurstLength: burst,
		Mode: string(mode), FEC: rec.FECProfile, Parity: rec.ParityShards,
		InnerMbps: rec.InnerMbps, Predicted: rec.PredictedDelivery, Claimed: rec.MeetsTarget,
		ObservedMin: 1,
	}
	for _, seed := range seeds {
		cfg := fec.SimConfig{
			Schedule: fec.ScheduleOff, Samples: samples, PayloadBytes: 1200, HeaderBytes: 56,
			OfferedMbps: rec.InnerMbps, CapacityMbps: capMbps,
			OneWay: time.Duration((rtt/2)*float64(time.Millisecond)), Loss: lossPct/100,
			Seed: seed, BurstLength: burst, DataShards: 20, ParityShards: rec.ParityShards,
			MicroData: 5, MicroParity: 3, CausalWindow: 20,
		}
		if rec.ParityShards > 0 { cfg.Schedule = fec.ScheduleTail }
		obs, err := fec.RunScheduleSimulation(cfg)
		if err != nil { fatal(err.Error()) }
		if obs.DeliveryRatio < r.ObservedMin { r.ObservedMin = obs.DeliveryRatio }
		p99 := float64(obs.P99) / float64(time.Millisecond)
		if p99 > r.ObservedP99 { r.ObservedP99 = p99 }
		util := rec.InnerMbps * obs.WireRatio / capMbps
		if util > r.WireUtilMax { r.WireUtilMax = util }
	}
	r.ObservedPass = r.ObservedMin >= target && r.WireUtilMax <= maxUtil+1e-9
	return r
}

func floats(raw string) []float64 {
	var out []float64
	for _, s := range strings.Split(raw, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || v < 0 { fatal("bad float list: "+raw) }
		out = append(out, v)
	}
	return out
}
func ints(raw string) []int {
	var out []int
	for _, s := range strings.Split(raw, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v < 1 { fatal("bad int list: "+raw) }
		out = append(out, v)
	}
	return out
}
func int64s(raw string) []int64 {
	var out []int64
	for _, s := range strings.Split(raw, ",") {
		v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil { fatal("bad seed list: "+raw) }
		out = append(out, v)
	}
	return out
}
func fatal(v string) { fmt.Fprintln(os.Stderr, "wbd-link-policy-eval:", v); os.Exit(1) }
