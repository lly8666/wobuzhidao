package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lly8666/wobuzhidao/internal/gamecontrol"
	"github.com/lly8666/wobuzhidao/internal/linkpolicy"
)

func main() {
	cfg := gamecontrol.DefaultConfig()
	var speedMode string
	var manualMbps float64
	var autoMbps, lossPct, meanBurst, carrierExpansion float64
	var lanes, maxLanes int
	var autoAdd bool
	var fec string
	var raceTarget float64
	flag.StringVar(&speedMode, "link-speed-mode", string(cfg.LinkSpeedMode), "auto or manual")
	flag.Float64Var(&autoMbps, "auto-link-mbps", 0, "latest automatic service-capacity estimate")
	flag.Float64Var(&manualMbps, "manual-link-mbps", 0, "forced link Mbps when mode=manual")
	flag.Float64Var(&lossPct, "loss-pct", 0, "measured packet loss percent")
	flag.Float64Var(&meanBurst, "mean-burst", 1, "measured mean consecutive loss run")
	flag.Float64Var(&carrierExpansion, "carrier-expansion", 1, "measured carrier wire expansion including retransmissions")
	flag.IntVar(&lanes, "lanes", cfg.RequestedLanes, "user requested game lane floor, 1..4")
	flag.BoolVar(&autoAdd, "auto-add-lane", cfg.AutoAddLanes, "allow controller to add lanes above the requested floor")
	flag.IntVar(&maxLanes, "max-lanes", cfg.MaxLanes, "maximum lanes auto-add may activate, <=4")
	flag.StringVar(&fec, "fec", string(cfg.FEC), "actual live lane FEC: off or 20:20")
	flag.Float64Var(&raceTarget, "race-target", cfg.RaceTarget, "target residual miss probability used only by lane auto-add")
	flag.Float64Var(&cfg.MaxWireUtil, "max-wire-util", cfg.MaxWireUtil, "fraction of measured/forced link capacity available to WBD wire traffic")
	flag.IntVar(&cfg.PayloadBytes, "payload-bytes", cfg.PayloadBytes, "logical payload size used for framing expansion")
	flag.IntVar(&cfg.FramingBytes, "framing-bytes", cfg.FramingBytes, "per-packet framing bytes used for expansion")
	flag.Parse()

	cfg.LinkSpeedMode = linkpolicy.LinkSpeedMode(speedMode)
	cfg.ManualLinkSpeedMbps = manualMbps
	cfg.RequestedLanes = lanes
	cfg.AutoAddLanes = autoAdd
	cfg.MaxLanes = maxLanes
	cfg.RaceTarget = raceTarget
	cfg.FEC = gamecontrol.FECProfile(fec)

	plan, err := gamecontrol.BuildPlan(cfg, gamecontrol.Measurement{
		AutoLinkSpeedMbps: autoMbps,
		Loss: lossPct / 100,
		MeanBurst: meanBurst,
		CarrierExpansion: carrierExpansion,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_GAME_PLAN_FAIL", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plan); err != nil {
		fmt.Fprintln(os.Stderr, "WBD_GAME_PLAN_FAIL", err)
		os.Exit(1)
	}
	fmt.Printf("WBD_GAME_PLAN_PASS speed_mode=%s effective_link_mbps=%.6f requested_lanes=%d active_lanes=%d fec=%s inner_ceiling_mbps=%.6f auto_added=%t\n",
		plan.LinkSpeedMode, plan.EffectiveLinkMbps, plan.RequestedLanes, plan.ActiveLanes, plan.FEC, plan.InnerCeilingMbps, plan.AutoLaneAdded)
}
