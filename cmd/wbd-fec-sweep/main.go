package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

type outputRow struct {
	Schedule           string  `json:"schedule"`
	LossPct            float64 `json:"loss_pct"`
	BurstLength        int     `json:"burst_length"`
	Seed               int64   `json:"seed"`
	K                  int     `json:"k"`
	R                  int     `json:"r"`
	Samples            int     `json:"samples"`
	Delivered          int     `json:"delivered"`
	Direct             int     `json:"direct"`
	Recovered          int     `json:"recovered"`
	DeliveryRatio      float64 `json:"delivery_ratio"`
	P50MS              float64 `json:"p50_ms"`
	P95MS              float64 `json:"p95_ms"`
	P99MS              float64 `json:"p99_ms"`
	MaxMS              float64 `json:"max_ms"`
	MeanMS             float64 `json:"mean_ms"`
	WireRatio          float64 `json:"wire_ratio"`
	OfferedUtilization float64 `json:"offered_utilization"`
	MaxReadyRepairs    int     `json:"max_ready_repairs"`
	DrainMS            float64 `json:"drain_ms"`
}

func main() {
	schedules := flag.String("schedules", "off,tail,micro,causal", "comma-separated fixed schedulers: off,tail,micro,causal")
	losses := flag.String("loss", "0,1,5,10,15,20", "comma-separated random loss percentages")
	bursts := flag.String("burst-lengths", "1,4", "comma-separated mean loss burst lengths; 1=iid")
	seeds := flag.String("seeds", "260825,260826,260827", "comma-separated deterministic seeds")
	samples := flag.Int("samples", 2000, "logical source datagrams")
	payload := flag.Int("payload", 1200, "source payload bytes")
	header := flag.Int("header", 56, "simulated per-shard framing bytes")
	offered := flag.Float64("offered-mbps", 20, "source payload offered rate")
	capacity := flag.Float64("capacity-mbps", 200, "one-way path capacity")
	rtt := flag.Float64("rtt-ms", 50, "symmetric RTT; simulator uses RTT/2 propagation")
	k := flag.Int("k", 20, "tail/causal source denominator")
	r := flag.Int("r", 10, "tail/causal repair numerator")
	microK := flag.Int("micro-k", 5, "micro-block source count")
	microR := flag.Int("micro-r", 3, "micro-block repair count")
	window := flag.Int("window", 20, "causal repair coding window")
	format := flag.String("format", "csv", "output format: csv or jsonl")
	flag.Parse()

	scheduleVals := parseSchedules(*schedules)
	lossVals := parseFloat64s(*losses)
	burstVals := parseInts(*bursts)
	seedVals := parseInt64s(*seeds)

	rows := make([]outputRow, 0, len(scheduleVals)*len(lossVals)*len(burstVals)*len(seedVals))
	for _, schedule := range scheduleVals {
		for _, lossPct := range lossVals {
			for _, burstLen := range burstVals {
				for _, seed := range seedVals {
					cfg := fec.SimConfig{
						Schedule:      schedule,
						Samples:       *samples,
						PayloadBytes:  *payload,
						HeaderBytes:   *header,
						OfferedMbps:   *offered,
						CapacityMbps:  *capacity,
						OneWay:        time.Duration((*rtt / 2) * float64(time.Millisecond)),
						Loss:          lossPct / 100,
						Seed:          seed,
						BurstLength:   burstLen,
						DataShards:    *k,
						ParityShards:  *r,
						MicroData:     *microK,
						MicroParity:   *microR,
						CausalWindow:  *window,
					}
					obs, err := fec.RunScheduleSimulation(cfg)
					if err != nil {
						fmt.Fprintf(os.Stderr, "%s loss=%.3f burst=%d seed=%d: %v\n", schedule, lossPct, burstLen, seed, err)
						os.Exit(1)
					}

					rowK, rowR := *k, *r
					switch schedule {
					case fec.ScheduleOff:
						rowR = 0
					case fec.ScheduleMicro:
						rowK, rowR = *microK, *microR
					}

					rows = append(rows, outputRow{
						Schedule:           string(schedule),
						LossPct:            lossPct,
						BurstLength:        burstLen,
						Seed:               seed,
						K:                  rowK,
						R:                  rowR,
						Samples:            *samples,
						Delivered:          obs.Delivered,
						Direct:             obs.Direct,
						Recovered:          obs.Recovered,
						DeliveryRatio:      obs.DeliveryRatio,
						P50MS:              milliseconds(obs.P50),
						P95MS:              milliseconds(obs.P95),
						P99MS:              milliseconds(obs.P99),
						MaxMS:              milliseconds(obs.Max),
						MeanMS:             milliseconds(obs.Mean),
						WireRatio:          obs.WireRatio,
						OfferedUtilization: obs.OfferedUtilization,
						MaxReadyRepairs:    obs.MaxReadyRepairs,
						DrainMS:            milliseconds(obs.Drain),
					})
				}
			}
		}
	}

	if *format == "jsonl" {
		enc := json.NewEncoder(os.Stdout)
		for _, row := range rows {
			if err := enc.Encode(row); err != nil {
				panic(err)
			}
		}
		return
	}
	if *format != "csv" {
		fmt.Fprintln(os.Stderr, "format must be csv or jsonl")
		os.Exit(2)
	}
	writeCSV(rows)
}

func writeCSV(rows []outputRow) {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	_ = w.Write([]string{
		"schedule", "loss_pct", "burst_length", "seed", "k", "r", "samples",
		"delivered", "direct", "recovered", "delivery_ratio",
		"p50_ms", "p95_ms", "p99_ms", "max_ms", "mean_ms",
		"wire_ratio", "offered_utilization", "max_ready_repairs", "drain_ms",
	})
	for _, x := range rows {
		_ = w.Write([]string{
			x.Schedule,
			floatString(x.LossPct),
			strconv.Itoa(x.BurstLength),
			strconv.FormatInt(x.Seed, 10),
			strconv.Itoa(x.K),
			strconv.Itoa(x.R),
			strconv.Itoa(x.Samples),
			strconv.Itoa(x.Delivered),
			strconv.Itoa(x.Direct),
			strconv.Itoa(x.Recovered),
			floatString(x.DeliveryRatio),
			floatString(x.P50MS),
			floatString(x.P95MS),
			floatString(x.P99MS),
			floatString(x.MaxMS),
			floatString(x.MeanMS),
			floatString(x.WireRatio),
			floatString(x.OfferedUtilization),
			strconv.Itoa(x.MaxReadyRepairs),
			floatString(x.DrainMS),
		})
	}
	if err := w.Error(); err != nil {
		panic(err)
	}
}

func milliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func floatString(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func parseSchedules(s string) []fec.ScheduleKind {
	var out []fec.ScheduleKind
	for _, part := range strings.Split(s, ",") {
		v := fec.ScheduleKind(strings.TrimSpace(part))
		switch v {
		case fec.ScheduleOff, fec.ScheduleTail, fec.ScheduleMicro, fec.ScheduleCausal:
		default:
			panic("invalid schedule: " + string(v))
		}
		out = append(out, v)
	}
	return out
}

func parseInts(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			panic(err)
		}
		out = append(out, v)
	}
	return out
}

func parseInt64s(s string) []int64 {
	var out []int64
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			panic(err)
		}
		out = append(out, v)
	}
	return out
}

func parseFloat64s(s string) []float64 {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			panic(err)
		}
		out = append(out, v)
	}
	return out
}
