package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

type profileSpec struct {
	Name string `json:"name"`
	K    int    `json:"k"`
	R    int    `json:"r"`
}

type aggregateRow struct {
	CapacityMbps       float64 `json:"capacity_mbps"`
	LossPct            float64 `json:"loss_pct"`
	RTTMS              float64 `json:"rtt_ms"`
	BurstLength        int     `json:"burst_length"`
	Profile            string  `json:"profile"`
	K                  int     `json:"k"`
	R                  int     `json:"r"`
	OfferedFraction    float64 `json:"offered_fraction"`
	OfferedInnerMbps   float64 `json:"offered_inner_mbps"`
	Replicas           int     `json:"replicas"`
	DeliveryMin        float64 `json:"delivery_min"`
	DeliveryMean       float64 `json:"delivery_mean"`
	P99MaxMS           float64 `json:"p99_max_ms"`
	P99MeanMS          float64 `json:"p99_mean_ms"`
	WireRatioMax       float64 `json:"wire_ratio_max"`
	WireRatioMean      float64 `json:"wire_ratio_mean"`
	PhysicalUtilMax    float64 `json:"physical_util_max"`
	DrainMaxMS         float64 `json:"drain_max_ms"`
	MaxReadyRepairsMax int     `json:"max_ready_repairs_max"`
	Eligible           bool    `json:"eligible"`
}

type recommendation struct {
	CapacityMbps     float64 `json:"capacity_mbps"`
	LossPct          float64 `json:"loss_pct"`
	RTTMS            float64 `json:"rtt_ms"`
	BurstLength      int     `json:"burst_length"`
	Profile          string  `json:"profile"`
	K                int     `json:"k"`
	R                int     `json:"r"`
	InnerMbps        float64 `json:"inner_mbps"`
	DeliveryMin      float64 `json:"delivery_min"`
	P99MaxMS         float64 `json:"p99_max_ms"`
	WireRatioMax     float64 `json:"wire_ratio_max"`
	PhysicalUtilMax  float64 `json:"physical_util_max"`
	SimulatorOnly    bool    `json:"simulator_only"`
}

type blockComparison struct {
	CapacityMbps      float64 `json:"capacity_mbps"`
	LossPct           float64 `json:"loss_pct"`
	RTTMS             float64 `json:"rtt_ms"`
	BurstLength       int     `json:"burst_length"`
	OfferedInnerMbps  float64 `json:"offered_inner_mbps"`
	Delivery20x20     float64 `json:"delivery_20x20"`
	Delivery10x10     float64 `json:"delivery_10x10"`
	DeliveryDeltaPP   float64 `json:"delivery_delta_pp_10x10_minus_20x20"`
	P99MS20x20        float64 `json:"p99_ms_20x20"`
	P99MS10x10        float64 `json:"p99_ms_10x10"`
	P99DeltaMS        float64 `json:"p99_delta_ms_10x10_minus_20x20"`
	WireRatio20x20    float64 `json:"wire_ratio_20x20"`
	WireRatio10x10    float64 `json:"wire_ratio_10x10"`
	WireRatioDelta    float64 `json:"wire_ratio_delta_10x10_minus_20x20"`
}

type output struct {
	Schema          string            `json:"schema"`
	GeneratedAtUTC  string            `json:"generated_at_utc"`
	Authority       string            `json:"authority"`
	SelectionPolicy map[string]any    `json:"selection_policy"`
	Grid            map[string]any    `json:"grid"`
	Rows            []aggregateRow    `json:"rows"`
	Recommendations []recommendation  `json:"provisional_recommendations"`
	BlockComparison []blockComparison `json:"block_comparison_20x20_vs_10x10"`
}

type rowKey struct {
	capacity float64
	loss     float64
	rtt      float64
	burst    int
	profile  string
	offered  float64
}

type groupKey struct {
	capacity float64
	loss     float64
	rtt      float64
	burst    int
}

func main() {
	capacitiesRaw := flag.String("capacities", "5,10,20,30,50,75,100,125,150", "physical link Mbit/s grid")
	lossesRaw := flag.String("loss", "0,1,3,5,10,15,20,30", "random packet loss percentages")
	rttsRaw := flag.String("rtt-ms", "20,100", "RTT milliseconds")
	burstsRaw := flag.String("burst-lengths", "1,4", "mean loss burst lengths; 1=iid")
	profilesRaw := flag.String("profiles", "off,20:4,20:8,20:12,20:16,20:20,10:5,10:10", "simulation FEC profiles")
	fractionsRaw := flag.String("offered-fractions", "0.20,0.30,0.40,0.50,0.60,0.70", "inner offered rate as a fraction of physical link rate")
	seedsRaw := flag.String("seeds", "260826,260827,260828", "deterministic simulation seeds")
	samples := flag.Int("samples", 500, "source datagrams per replica")
	payload := flag.Int("payload", 1200, "inner payload bytes")
	header := flag.Int("header", 56, "simulated per-shard FEC framing bytes")
	targetDelivery := flag.Float64("target-delivery", 0.995, "minimum delivery ratio required from every seed replica")
	maxPhysicalUtil := flag.Float64("max-physical-util", 0.92, "maximum estimated wire utilization for provisional selection")
	outPath := flag.String("out", "", "optional JSON output path; stdout when empty")
	flag.Parse()

	capacities := mustFloatList(*capacitiesRaw)
	losses := mustFloatList(*lossesRaw)
	rtts := mustFloatList(*rttsRaw)
	bursts := mustIntList(*burstsRaw)
	profiles := mustProfiles(*profilesRaw)
	fractions := mustFloatList(*fractionsRaw)
	seeds := mustInt64List(*seedsRaw)
	if *samples <= 0 || *payload <= 0 || *header < 0 || *targetDelivery <= 0 || *targetDelivery > 1 || *maxPhysicalUtil <= 0 || *maxPhysicalUtil > 1.5 {
		fatal(errors.New("invalid sweep numeric parameters"))
	}
	for _, x := range capacities {
		if x <= 0 {
			fatal(fmt.Errorf("capacity must be positive: %v", x))
		}
	}
	for _, x := range losses {
		if x < 0 || x >= 100 {
			fatal(fmt.Errorf("loss must be in [0,100): %v", x))
		}
	}
	for _, x := range rtts {
		if x < 0 {
			fatal(fmt.Errorf("RTT must be non-negative: %v", x))
		}
	}
	for _, x := range fractions {
		if x <= 0 || x > 1.5 {
			fatal(fmt.Errorf("offered fraction must be in (0,1.5]: %v", x))
		}
	}

	rows := make([]aggregateRow, 0, len(capacities)*len(losses)*len(rtts)*len(bursts)*len(profiles)*len(fractions))
	for _, capacity := range capacities {
		for _, loss := range losses {
			for _, rtt := range rtts {
				for _, burst := range bursts {
					for _, profile := range profiles {
						for _, fraction := range fractions {
							offered := capacity * fraction
							row, err := simulateAggregate(capacity, loss, rtt, burst, profile, offered, fraction, seeds, *samples, *payload, *header, *targetDelivery, *maxPhysicalUtil)
							if err != nil {
								fatal(fmt.Errorf("capacity=%.3f loss=%.3f rtt=%.3f burst=%d profile=%s offered=%.3f: %w", capacity, loss, rtt, burst, profile.Name, offered, err))
							}
							rows = append(rows, row)
						}
					}
				}
			}
		}
	}

	recs := selectRecommendations(rows)
	comparisons := compareBlocks(rows)
	result := output{
		Schema:         "wbd-fec-policy-discovery/v1",
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Authority:      "simulator_discovery_only_not_release_authority",
		SelectionPolicy: map[string]any{
			"target_delivery_min_every_seed": *targetDelivery,
			"max_estimated_physical_util":     *maxPhysicalUtil,
			"rank":                            "highest inner Mbps, then lower worst-seed p99, then lower wire ratio, then smaller block",
			"release_note":                    "recommendations remain provisional until targeted real FakeTCP/DTLS/LINK tc/netem validation passes",
		},
		Grid: map[string]any{
			"capacities_mbps": capacities,
			"loss_pct":        losses,
			"rtt_ms":          rtts,
			"burst_lengths":   bursts,
			"profiles":        profileNames(profiles),
			"offered_fraction": fractions,
			"seeds":           seeds,
			"samples_per_seed": *samples,
			"payload_bytes":    *payload,
			"header_bytes":     *header,
		},
		Rows:            rows,
		Recommendations: recs,
		BlockComparison: comparisons,
	}

	var dst *os.File
	if strings.TrimSpace(*outPath) == "" {
		dst = os.Stdout
	} else {
		f, err := os.Create(*outPath)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		dst = f
	}
	enc := json.NewEncoder(dst)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "WBD_FEC_POLICY_DISCOVERY_PASS rows=%d recommendations=%d block_comparisons=%d authority=simulator_only\n", len(rows), len(recs), len(comparisons))
}

func simulateAggregate(capacity, loss, rtt float64, burst int, profile profileSpec, offered, fraction float64, seeds []int64, samples, payload, header int, targetDelivery, maxPhysicalUtil float64) (aggregateRow, error) {
	row := aggregateRow{
		CapacityMbps:     capacity,
		LossPct:          loss,
		RTTMS:            rtt,
		BurstLength:      burst,
		Profile:          profile.Name,
		K:                profile.K,
		R:                profile.R,
		OfferedFraction:  fraction,
		OfferedInnerMbps: offered,
		Replicas:         len(seeds),
		DeliveryMin:      1,
	}
	var deliverySum, p99Sum, wireSum float64
	for _, seed := range seeds {
		cfg := fec.SimConfig{
			Schedule:      fec.ScheduleOff,
			Samples:       samples,
			PayloadBytes:  payload,
			HeaderBytes:   header,
			OfferedMbps:   offered,
			CapacityMbps:  capacity,
			OneWay:        time.Duration((rtt / 2) * float64(time.Millisecond)),
			Loss:          loss / 100,
			Seed:          seed,
			BurstLength:   burst,
			DataShards:    profile.K,
			ParityShards:  profile.R,
			MicroData:     5,
			MicroParity:   3,
			CausalWindow:  20,
		}
		if profile.R > 0 {
			cfg.Schedule = fec.ScheduleTail
		}
		obs, err := fec.RunScheduleSimulation(cfg)
		if err != nil {
			return aggregateRow{}, err
		}
		p99 := durationMS(obs.P99)
		drain := durationMS(obs.Drain)
		physicalUtil := offered * obs.WireRatio / capacity
		row.DeliveryMin = math.Min(row.DeliveryMin, obs.DeliveryRatio)
		row.P99MaxMS = math.Max(row.P99MaxMS, p99)
		row.WireRatioMax = math.Max(row.WireRatioMax, obs.WireRatio)
		row.PhysicalUtilMax = math.Max(row.PhysicalUtilMax, physicalUtil)
		row.DrainMaxMS = math.Max(row.DrainMaxMS, drain)
		if obs.MaxReadyRepairs > row.MaxReadyRepairsMax {
			row.MaxReadyRepairsMax = obs.MaxReadyRepairs
		}
		deliverySum += obs.DeliveryRatio
		p99Sum += p99
		wireSum += obs.WireRatio
	}
	row.DeliveryMean = deliverySum / float64(len(seeds))
	row.P99MeanMS = p99Sum / float64(len(seeds))
	row.WireRatioMean = wireSum / float64(len(seeds))
	row.Eligible = row.DeliveryMin >= targetDelivery && row.PhysicalUtilMax <= maxPhysicalUtil
	return row, nil
}

func selectRecommendations(rows []aggregateRow) []recommendation {
	best := map[groupKey]aggregateRow{}
	for _, row := range rows {
		if !row.Eligible {
			continue
		}
		key := groupKey{row.CapacityMbps, row.LossPct, row.RTTMS, row.BurstLength}
		old, ok := best[key]
		if !ok || betterCandidate(row, old) {
			best[key] = row
		}
	}
	keys := make([]groupKey, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.capacity != b.capacity { return a.capacity < b.capacity }
		if a.loss != b.loss { return a.loss < b.loss }
		if a.rtt != b.rtt { return a.rtt < b.rtt }
		return a.burst < b.burst
	})
	out := make([]recommendation, 0, len(keys))
	for _, key := range keys {
		r := best[key]
		out = append(out, recommendation{
			CapacityMbps: r.CapacityMbps, LossPct: r.LossPct, RTTMS: r.RTTMS, BurstLength: r.BurstLength,
			Profile: r.Profile, K: r.K, R: r.R, InnerMbps: r.OfferedInnerMbps,
			DeliveryMin: r.DeliveryMin, P99MaxMS: r.P99MaxMS, WireRatioMax: r.WireRatioMax,
			PhysicalUtilMax: r.PhysicalUtilMax, SimulatorOnly: true,
		})
	}
	return out
}

func betterCandidate(a, b aggregateRow) bool {
	const eps = 1e-9
	if math.Abs(a.OfferedInnerMbps-b.OfferedInnerMbps) > eps {
		return a.OfferedInnerMbps > b.OfferedInnerMbps
	}
	if math.Abs(a.P99MaxMS-b.P99MaxMS) > eps {
		return a.P99MaxMS < b.P99MaxMS
	}
	if math.Abs(a.WireRatioMax-b.WireRatioMax) > eps {
		return a.WireRatioMax < b.WireRatioMax
	}
	return a.K+a.R < b.K+b.R
}

func compareBlocks(rows []aggregateRow) []blockComparison {
	by := make(map[rowKey]aggregateRow, len(rows))
	for _, row := range rows {
		by[rowKey{row.CapacityMbps, row.LossPct, row.RTTMS, row.BurstLength, row.Profile, row.OfferedInnerMbps}] = row
	}
	out := make([]blockComparison, 0)
	for _, twenty := range rows {
		if twenty.Profile != "20:20" {
			continue
		}
		ten, ok := by[rowKey{twenty.CapacityMbps, twenty.LossPct, twenty.RTTMS, twenty.BurstLength, "10:10", twenty.OfferedInnerMbps}]
		if !ok {
			continue
		}
		out = append(out, blockComparison{
			CapacityMbps: twenty.CapacityMbps, LossPct: twenty.LossPct, RTTMS: twenty.RTTMS, BurstLength: twenty.BurstLength,
			OfferedInnerMbps: twenty.OfferedInnerMbps,
			Delivery20x20: twenty.DeliveryMin, Delivery10x10: ten.DeliveryMin,
			DeliveryDeltaPP: (ten.DeliveryMin - twenty.DeliveryMin) * 100,
			P99MS20x20: twenty.P99MaxMS, P99MS10x10: ten.P99MaxMS,
			P99DeltaMS: ten.P99MaxMS - twenty.P99MaxMS,
			WireRatio20x20: twenty.WireRatioMax, WireRatio10x10: ten.WireRatioMax,
			WireRatioDelta: ten.WireRatioMax - twenty.WireRatioMax,
		})
	}
	return out
}

func mustProfiles(raw string) []profileSpec {
	parts := strings.Split(raw, ",")
	out := make([]profileSpec, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		p, err := parseProfile(strings.TrimSpace(part))
		if err != nil { fatal(err) }
		if seen[p.Name] { fatal(fmt.Errorf("duplicate profile %q", p.Name)) }
		seen[p.Name] = true
		out = append(out, p)
	}
	if len(out) == 0 { fatal(errors.New("at least one profile is required")) }
	return out
}

func parseProfile(raw string) (profileSpec, error) {
	if raw == "off" {
		return profileSpec{Name: "off", K: 20, R: 0}, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return profileSpec{}, fmt.Errorf("invalid profile %q", raw)
	}
	k, err := strconv.Atoi(parts[0])
	if err != nil { return profileSpec{}, fmt.Errorf("invalid profile %q: %w", raw, err) }
	r, err := strconv.Atoi(parts[1])
	if err != nil { return profileSpec{}, fmt.Errorf("invalid profile %q: %w", raw, err) }
	if k <= 0 || r <= 0 || k+r > 255 {
		return profileSpec{}, fmt.Errorf("invalid profile %q: require K>0 R>0 K+R<=255", raw)
	}
	return profileSpec{Name: fmt.Sprintf("%d:%d", k, r), K: k, R: r}, nil
}

func profileNames(in []profileSpec) []string {
	out := make([]string, len(in))
	for i, p := range in { out[i] = p.Name }
	return out
}

func mustFloatList(raw string) []float64 {
	var out []float64
	for _, p := range strings.Split(raw, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil { fatal(err) }
		out = append(out, v)
	}
	if len(out) == 0 { fatal(errors.New("empty float list")) }
	return out
}

func mustIntList(raw string) []int {
	var out []int
	for _, p := range strings.Split(raw, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil { fatal(err) }
		if v <= 0 { fatal(fmt.Errorf("integer list values must be positive: %d", v)) }
		out = append(out, v)
	}
	if len(out) == 0 { fatal(errors.New("empty integer list")) }
	return out
}

func mustInt64List(raw string) []int64 {
	var out []int64
	for _, p := range strings.Split(raw, ",") {
		v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil { fatal(err) }
		out = append(out, v)
	}
	if len(out) == 0 { fatal(errors.New("empty seed list")) }
	return out
}

func durationMS(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "WBD_FEC_POLICY_DISCOVERY_FAIL", err)
	os.Exit(1)
}
