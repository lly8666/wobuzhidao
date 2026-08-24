package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/benchmark"
)

func main() {
	losses := flag.String("loss", "0,1,2,3,5,8,10,12,15", "comma-separated impairment percentages")
	seeds := flag.String("seeds", "260824,260825,260826", "comma-separated seeds")
	samples := flag.Int("samples", 200, "logical source chunks")
	payload := flag.Int("payload", 256, "bytes per source chunk")
	window := flag.Int("window", 32, "logical source window")
	rtt := flag.Int("rtt-ms", 50, "symmetric RTT in milliseconds")
	hold := flag.Int("hold-ms", 200, "extra TCP carrier hold for an impaired shard")
	timeout := flag.Duration("case-timeout", 30*time.Second, "timeout per case")
	flag.Parse()

	lossVals := parseInts(*losses)
	seedVals := parseUint64s(*seeds)
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	_ = w.Write([]string{"fec", "loss_pct", "seed", "mean_ms", "p50_ms", "p95_ms", "p99_ms", "late_ratio", "delivery_ratio", "traffic_x"})
	for _, parity := range []int{10, 20} {
		for _, loss := range lossVals {
			for _, seed := range seedVals {
				p := benchmark.RealFaultProfile{
					LaneCount:         2,
					Seed:              seed,
					Samples:           *samples,
					PayloadBytes:      *payload,
					MinOneWay:         time.Duration(*rtt/2) * time.Millisecond,
					MaxOneWay:         time.Duration(*rtt/2) * time.Millisecond,
					ImpairBasisPoints: uint16(loss * 100),
					ExtraHold:         time.Duration(*hold) * time.Millisecond,
					SoftDeadline:      100 * time.Millisecond,
					SourceSpacing:     0,
					Window:            *window,
					BurstLength:       1,
				}
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				obs, err := benchmark.RunRealFaultWBDFEC(ctx, p, benchmark.FECExperimentConfig{DataShards: 20, ParityShards: parity})
				cancel()
				if err != nil {
					fmt.Fprintf(os.Stderr, "fec=20:%d loss=%d seed=%d: %v\n", parity, loss, seed, err)
					os.Exit(1)
				}
				traffic := float64(obs.IntentionalBytes) / float64(obs.SourceBytes)
				_ = w.Write([]string{
					fmt.Sprintf("20:%d", parity), strconv.Itoa(loss), strconv.FormatUint(seed, 10),
					fmt.Sprintf("%.3f", float64(obs.Mean)/float64(time.Millisecond)),
					fmt.Sprintf("%.3f", float64(obs.P50)/float64(time.Millisecond)),
					fmt.Sprintf("%.3f", float64(obs.P95)/float64(time.Millisecond)),
					fmt.Sprintf("%.3f", float64(obs.P99)/float64(time.Millisecond)),
					fmt.Sprintf("%.4f", obs.LateRatio), fmt.Sprintf("%.4f", obs.DeliveryRatio), fmt.Sprintf("%.2f", traffic),
				})
				w.Flush()
				if err := w.Error(); err != nil { panic(err) }
			}
		}
	}
}

func parseInts(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil { panic(err) }
		out = append(out, v)
	}
	return out
}

func parseUint64s(s string) []uint64 {
	var out []uint64
	for _, p := range strings.Split(s, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64)
		if err != nil { panic(err) }
		out = append(out, v)
	}
	return out
}
