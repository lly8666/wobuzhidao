package benchmark

import (
	"errors"
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

const StressProfileVersion = 1

// StressProfile is a deterministic two-carrier weak-network model. It is kept
// separate from the small M10-001 Profile micro-model so large jitter/loss
// experiments can't silently change the semantics of the original baseline.
type StressProfile struct {
	Version           int
	Name              string
	Seed              uint64
	Step              time.Duration
	ChunkBytes        int
	PrimaryArrival    []int
	AlternateDelay    []int
	PrimaryUDPDropped []bool
	SoftDeadlineStep  int
}

type StressResult struct {
	Profile          string        `json:"profile"`
	Seed             uint64        `json:"seed"`
	Strategy         string        `json:"strategy"`
	Multiplier       string        `json:"multiplier"`
	P50              time.Duration `json:"p50_ns"`
	P95              time.Duration `json:"p95_ns"`
	P99              time.Duration `json:"p99_ns"`
	Completion       time.Duration `json:"completion_ns"`
	SourceBytes      uint64        `json:"source_bytes"`
	ProtectionBytes  uint64        `json:"protection_bytes"`
	DuplicateBytes   uint64        `json:"duplicate_bytes"`
	ReinjectionBytes uint64        `json:"reinjection_bytes"`
	FECBytes         uint64        `json:"fec_bytes"`
	DeliveredChunks  int           `json:"delivered_chunks"`
	DroppedChunks    int           `json:"dropped_chunks"`
	DeliveryPPM      uint64        `json:"delivery_ppm"`
}

var ErrInvalidStressProfile = errors.New("invalid WBD stress profile")

func StressProfiles() []StressProfile {
	type spec struct {
		name                         string
		seed                         uint64
		minDelayMS, maxDelayMS       int
		lossPermille                 int
		recoveryMinMS, recoveryMaxMS int
		softDeadlineMS               int
	}
	specs := []spec{
		{name: "normal-40-60ms", seed: 3001, minDelayMS: 40, maxDelayMS: 60, softDeadlineMS: 90},
		{name: "mobile-80-150ms-2pct", seed: 3002, minDelayMS: 80, maxDelayMS: 150, lossPermille: 20, recoveryMinMS: 180, recoveryMaxMS: 320, softDeadlineMS: 220},
		{name: "weak-150-300ms-10pct-a", seed: 3010, minDelayMS: 150, maxDelayMS: 300, lossPermille: 100, recoveryMinMS: 300, recoveryMaxMS: 600, softDeadlineMS: 380},
		{name: "weak-150-300ms-10pct-b", seed: 3011, minDelayMS: 150, maxDelayMS: 300, lossPermille: 100, recoveryMinMS: 300, recoveryMaxMS: 600, softDeadlineMS: 380},
		{name: "very-weak-150-300ms-20pct-a", seed: 3020, minDelayMS: 150, maxDelayMS: 300, lossPermille: 200, recoveryMinMS: 450, recoveryMaxMS: 900, softDeadlineMS: 420},
		{name: "very-weak-150-300ms-20pct-b", seed: 3021, minDelayMS: 150, maxDelayMS: 300, lossPermille: 200, recoveryMinMS: 450, recoveryMaxMS: 900, softDeadlineMS: 420},
		{name: "extreme-250-600ms-30pct-a", seed: 3030, minDelayMS: 250, maxDelayMS: 600, lossPermille: 300, recoveryMinMS: 800, recoveryMaxMS: 1600, softDeadlineMS: 750},
		{name: "extreme-250-600ms-30pct-b", seed: 3031, minDelayMS: 250, maxDelayMS: 600, lossPermille: 300, recoveryMinMS: 800, recoveryMaxMS: 1600, softDeadlineMS: 750},
	}
	out := make([]StressProfile, 0, len(specs))
	for _, sp := range specs {
		out = append(out, makeStressProfile(sp.name, sp.seed, sp.minDelayMS, sp.maxDelayMS, sp.lossPermille, sp.recoveryMinMS, sp.recoveryMaxMS, sp.softDeadlineMS))
	}
	return out
}

func makeStressProfile(name string, seed uint64, minDelayMS, maxDelayMS, lossPermille, recoveryMinMS, recoveryMaxMS, softDeadlineMS int) StressProfile {
	const chunks, chunkBytes, stepMS = 64, 1024, 10
	rng := stressRNG{state: seed}
	losses := (chunks*lossPermille + 999) / 1000
	primaryLost := stressLossMask(&rng, chunks, losses)
	alternateLost := stressLossMask(&rng, chunks, losses)
	primary := make([]int, chunks)
	alternate := make([]int, chunks)
	for i := 0; i < chunks; i++ {
		d := stressUniform(&rng, minDelayMS, maxDelayMS)
		if primaryLost[i] {
			d += stressUniform(&rng, recoveryMinMS, recoveryMaxMS)
		}
		primary[i] = stressCeilDiv(d, stepMS)

		a := stressUniform(&rng, minDelayMS, maxDelayMS)
		if alternateLost[i] {
			a += stressUniform(&rng, recoveryMinMS, recoveryMaxMS)
		}
		alternate[i] = stressCeilDiv(a, stepMS)
	}
	return StressProfile{
		Version: StressProfileVersion, Name: name, Seed: seed, Step: stepMS * time.Millisecond, ChunkBytes: chunkBytes,
		PrimaryArrival: primary, AlternateDelay: alternate, PrimaryUDPDropped: primaryLost,
		SoftDeadlineStep: stressCeilDiv(softDeadlineMS, stepMS),
	}
}

func RunStress(p StressProfile, strategy Strategy, multiplier rbc.MultiplierQ4) (StressResult, error) {
	if err := validateStressProfile(p); err != nil {
		return StressResult{}, err
	}
	if strategy < StrategyNativeTCP || strategy > StrategyWBDXOR || !multiplier.Valid() {
		return StressResult{}, ErrInvalidStressProfile
	}
	if (strategy == StrategyNativeTCP || strategy == StrategyNativeUDP) && multiplier != rbc.Multiplier10 {
		return StressResult{}, ErrInvalidStressProfile
	}

	n := len(p.PrimaryArrival)
	known := append([]int(nil), p.PrimaryArrival...)
	budget := rbc.NewBudget()
	for i := 0; i < n; i++ {
		if err := budget.AdmitSource(uint64(p.ChunkBytes), multiplier); err != nil {
			return StressResult{}, err
		}
	}

	switch strategy {
	case StrategyNativeTCP:
		stressPrefixOrder(known)
	case StrategyNativeUDP:
		for i := range known {
			if p.PrimaryUDPDropped[i] {
				known[i] = 0
			}
		}
	case StrategyWBDDuplicate:
		count := int(budget.Available() / uint64(p.ChunkBytes))
		for _, i := range stressSpreadIndices(n, count) {
			if err := budget.Spend(rbc.SpendDuplicate, uint64(p.ChunkBytes)); err != nil {
				return StressResult{}, err
			}
			known[i] = stressMin(known[i], p.AlternateDelay[i])
		}
		stressPrefixOrder(known)
	case StrategyWBDXOR:
		for i := 0; i+1 < n && budget.Available() >= uint64(p.ChunkBytes); i += 2 {
			if err := budget.Spend(rbc.SpendFEC, uint64(p.ChunkBytes)); err != nil {
				return StressResult{}, err
			}
			a, b, parity := known[i], known[i+1], p.AlternateDelay[i]
			known[i] = stressMin(a, stressMax(b, parity))
			known[i+1] = stressMin(b, stressMax(a, parity))
		}
		known = stressReinject(p, known, budget, false)
		stressPrefixOrder(known)
	case StrategyWBDReinjection, StrategyWBDTailDeadline:
		known = stressReinject(p, known, budget, strategy == StrategyWBDTailDeadline)
		stressPrefixOrder(known)
	}

	snap := budget.Snapshot()
	lat := make([]time.Duration, 0, n)
	for _, step := range known {
		if step > 0 {
			lat = append(lat, time.Duration(step)*p.Step)
		}
	}
	delivered := len(lat)
	return StressResult{
		Profile: p.Name, Seed: p.Seed, Strategy: strategy.String(), Multiplier: multiplier.String(),
		P50: stressQuantile(lat, 50), P95: stressQuantile(lat, 95), P99: stressQuantile(lat, 99), Completion: stressMaxDuration(lat),
		SourceBytes: snap.SourceBytes, ProtectionBytes: snap.SpentBytes, DuplicateBytes: snap.DuplicateBytes,
		ReinjectionBytes: snap.ReinjectionBytes, FECBytes: snap.FECBytes,
		DeliveredChunks: delivered, DroppedChunks: n - delivered, DeliveryPPM: uint64(delivered) * 1_000_000 / uint64(n),
	}, nil
}

func StressMatrix() ([]StressResult, error) {
	variants := []struct {
		s Strategy
		m rbc.MultiplierQ4
	}{
		{StrategyNativeTCP, rbc.Multiplier10}, {StrategyNativeUDP, rbc.Multiplier10},
		{StrategyWBDReinjection, rbc.Multiplier15}, {StrategyWBDTailDeadline, rbc.Multiplier15},
		{StrategyWBDDuplicate, rbc.Multiplier15}, {StrategyWBDXOR, rbc.Multiplier15},
		{StrategyWBDReinjection, rbc.Multiplier20}, {StrategyWBDTailDeadline, rbc.Multiplier20},
		{StrategyWBDDuplicate, rbc.Multiplier20}, {StrategyWBDXOR, rbc.Multiplier20},
	}
	var out []StressResult
	for _, p := range StressProfiles() {
		for _, v := range variants {
			r, err := RunStress(p, v.s, v.m)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
	}
	return out, nil
}

func stressReinject(p StressProfile, original []int, budget *rbc.Budget, tail bool) []int {
	known := append([]int(nil), original...)
	scheduled := make([]bool, len(known))
	maxStep := p.SoftDeadlineStep
	for i := range known {
		maxStep = stressMax(maxStep, known[i]+p.AlternateDelay[i])
	}
	maxStep += len(known) + 4
	for step := 1; step <= maxStep; step++ {
		highest := -1
		for i, at := range known {
			if at <= step && i > highest {
				highest = i
			}
		}
		for i := 0; i < highest; i++ {
			if known[i] <= step || scheduled[i] || budget.Available() < uint64(p.ChunkBytes) {
				continue
			}
			if budget.Spend(rbc.SpendReinjection, uint64(p.ChunkBytes)) == nil {
				scheduled[i] = true
				known[i] = stressMin(known[i], step+p.AlternateDelay[i])
			}
		}
		if tail && step == p.SoftDeadlineStep {
			for i := range known {
				if known[i] <= step || scheduled[i] || budget.Available() < uint64(p.ChunkBytes) {
					continue
				}
				if budget.Spend(rbc.SpendReinjection, uint64(p.ChunkBytes)) == nil {
					scheduled[i] = true
					known[i] = stressMin(known[i], step+p.AlternateDelay[i])
				}
			}
		}
	}
	return known
}

func validateStressProfile(p StressProfile) error {
	if p.Version != StressProfileVersion || p.Name == "" || p.Step <= 0 || p.ChunkBytes <= 0 || len(p.PrimaryArrival) < 2 || len(p.AlternateDelay) != len(p.PrimaryArrival) || len(p.PrimaryUDPDropped) != len(p.PrimaryArrival) || p.SoftDeadlineStep < 1 {
		return ErrInvalidStressProfile
	}
	for i := range p.PrimaryArrival {
		if p.PrimaryArrival[i] < 1 || p.AlternateDelay[i] < 1 {
			return ErrInvalidStressProfile
		}
	}
	return nil
}

type stressRNG struct{ state uint64 }

func (r *stressRNG) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func stressLossMask(r *stressRNG, n, losses int) []bool {
	out := make([]bool, n)
	for marked := 0; marked < losses; {
		i := int(r.next() % uint64(n))
		if out[i] {
			continue
		}
		out[i], marked = true, marked+1
	}
	return out
}

func stressUniform(r *stressRNG, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + int(r.next()%uint64(hi-lo+1))
}

func stressCeilDiv(v, d int) int { return (v + d - 1) / d }
func stressMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func stressMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func stressPrefixOrder(v []int) {
	for i := 1; i < len(v); i++ {
		if v[i] < v[i-1] {
			v[i] = v[i-1]
		}
	}
}

func stressSpreadIndices(n, count int) []int {
	if count > n {
		count = n
	}
	out := make([]int, 0, count)
	seen := make(map[int]bool, count)
	for k := 0; k < count; k++ {
		i := (k * n) / count
		for seen[i] {
			i = (i + 1) % n
		}
		seen[i] = true
		out = append(out, i)
	}
	return out
}

func stressQuantile(in []time.Duration, pct int) time.Duration {
	if len(in) == 0 {
		return 0
	}
	v := append([]time.Duration(nil), in...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	idx := (pct*len(v)+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(v) {
		idx = len(v) - 1
	}
	return v[idx]
}

func stressMaxDuration(v []time.Duration) time.Duration {
	var out time.Duration
	for _, d := range v {
		if d > out {
			out = d
		}
	}
	return out
}
