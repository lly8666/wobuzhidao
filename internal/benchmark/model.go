package benchmark

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

const ProfileVersion = 1

type Strategy uint8

const (
	StrategyNativeTCP Strategy = iota + 1
	StrategyNativeUDP
	StrategyWBDReinjection
	StrategyWBDTailDeadline
	StrategyWBDDuplicate
	StrategyWBDXOR
)

func (s Strategy) String() string {
	switch s {
	case StrategyNativeTCP:
		return "native-tcp"
	case StrategyNativeUDP:
		return "native-udp"
	case StrategyWBDReinjection:
		return "wbd-reinjection"
	case StrategyWBDTailDeadline:
		return "wbd-tail-deadline"
	case StrategyWBDDuplicate:
		return "wbd-duplicate"
	case StrategyWBDXOR:
		return "wbd-xor"
	default:
		return "unknown"
	}
}

type Profile struct {
	Version          int
	Name             string
	Seed             uint64
	Step             time.Duration
	ChunkBytes       int
	OriginalArrival  []int
	SoftDeadlineStep int
}

type Result struct {
	Profile          string
	Seed             uint64
	Strategy         Strategy
	Multiplier       rbc.MultiplierQ4
	ChunkUsableStep  []int
	P50              time.Duration
	P95              time.Duration
	P99              time.Duration
	Completion       time.Duration
	SourceBytes      uint64
	ProtectionBytes  uint64
	EntitledBytes    uint64
	DuplicateBytes   uint64
	ReinjectionBytes uint64
	FECBytes         uint64
}

var ErrInvalidProfile = errors.New("invalid WBD benchmark profile")

func StandardProfiles() []Profile {
	const chunkBytes = 1024
	step := 10 * time.Millisecond
	return []Profile{
		{Version: ProfileVersion, Name: "clean", Seed: 1001, Step: step, ChunkBytes: chunkBytes, OriginalArrival: []int{1, 1, 1, 1, 1, 1, 1, 1}, SoftDeadlineStep: 3},
		{Version: ProfileVersion, Name: "reordered", Seed: 1002, Step: step, ChunkBytes: chunkBytes, OriginalArrival: []int{1, 3, 1, 2, 1, 1, 2, 1}, SoftDeadlineStep: 4},
		{Version: ProfileVersion, Name: "single-stall", Seed: 1003, Step: step, ChunkBytes: chunkBytes, OriginalArrival: []int{1, 1, 8, 1, 1, 1, 1, 1}, SoftDeadlineStep: 4},
		{Version: ProfileVersion, Name: "burst-same-xor", Seed: 1004, Step: step, ChunkBytes: chunkBytes, OriginalArrival: []int{8, 8, 1, 1, 1, 1, 1, 1}, SoftDeadlineStep: 4},
		{Version: ProfileVersion, Name: "burst-cross-xor", Seed: 1005, Step: step, ChunkBytes: chunkBytes, OriginalArrival: []int{1, 8, 8, 1, 1, 1, 1, 1}, SoftDeadlineStep: 4},
		{Version: ProfileVersion, Name: "final-chunk-stall", Seed: 1006, Step: step, ChunkBytes: chunkBytes, OriginalArrival: []int{1, 1, 1, 1, 1, 1, 1, 8}, SoftDeadlineStep: 4},
	}
}

func Run(p Profile, strategy Strategy, multiplier rbc.MultiplierQ4) (Result, error) {
	if err := validateProfile(p); err != nil {
		return Result{}, err
	}
	if strategy < StrategyNativeTCP || strategy > StrategyWBDXOR {
		return Result{}, fmt.Errorf("%w: strategy", ErrInvalidProfile)
	}
	if !multiplier.Valid() {
		return Result{}, rbc.ErrInvalidMultiplier
	}
	if (strategy == StrategyNativeTCP || strategy == StrategyNativeUDP) && multiplier != rbc.Multiplier10 {
		return Result{}, fmt.Errorf("%w: native baselines require 1.0x", ErrInvalidProfile)
	}

	n := len(p.OriginalArrival)
	known := append([]int(nil), p.OriginalArrival...)
	budget := rbc.NewBudget()
	for range n {
		if err := budget.AdmitSource(uint64(p.ChunkBytes), multiplier); err != nil {
			return Result{}, err
		}
	}

	switch strategy {
	case StrategyNativeTCP:
		for i := 1; i < n; i++ {
			if known[i] < known[i-1] {
				known[i] = known[i-1]
			}
		}
	case StrategyNativeUDP:
	case StrategyWBDDuplicate:
		count := int(budget.Available() / uint64(p.ChunkBytes))
		for _, i := range spreadIndices(n, count) {
			if err := budget.Spend(rbc.SpendDuplicate, uint64(p.ChunkBytes)); err != nil {
				return Result{}, err
			}
			known[i] = minInt(known[i], 1)
		}
		prefixOrder(known)
	case StrategyWBDXOR:
		for i := 0; i+1 < n && budget.Available() >= uint64(p.ChunkBytes); i += 2 {
			if err := budget.Spend(rbc.SpendFEC, uint64(p.ChunkBytes)); err != nil {
				return Result{}, err
			}
			recovered := minInt(known[i], known[i+1])
			known[i], known[i+1] = recovered, recovered
		}
		known = simulateReinjection(p, known, budget, false)
		prefixOrder(known)
	case StrategyWBDReinjection, StrategyWBDTailDeadline:
		known = simulateReinjection(p, known, budget, strategy == StrategyWBDTailDeadline)
		prefixOrder(known)
	}

	snap := budget.Snapshot()
	durations := make([]time.Duration, n)
	for i, step := range known {
		durations[i] = time.Duration(step) * p.Step
	}
	return Result{
		Profile: p.Name, Seed: p.Seed, Strategy: strategy, Multiplier: multiplier,
		ChunkUsableStep: append([]int(nil), known...),
		P50: quantile(durations, 50), P95: quantile(durations, 95), P99: quantile(durations, 99),
		Completion: maxDuration(durations),
		SourceBytes: snap.SourceBytes, ProtectionBytes: snap.SpentBytes, EntitledBytes: snap.EntitledBytes,
		DuplicateBytes: snap.DuplicateBytes, ReinjectionBytes: snap.ReinjectionBytes, FECBytes: snap.FECBytes,
	}, nil
}

func simulateReinjection(p Profile, original []int, budget *rbc.Budget, tailDeadline bool) []int {
	n := len(original)
	knownAt := append([]int(nil), original...)
	scheduled := make([]bool, n)
	maxStep := 1
	for _, v := range original {
		if v > maxStep {
			maxStep = v
		}
	}
	maxStep += n + p.SoftDeadlineStep + 2
	for step := 1; step <= maxStep; step++ {
		highestKnown := -1
		for i, at := range knownAt {
			if at <= step && i > highestKnown {
				highestKnown = i
			}
		}
		for i := 0; i < highestKnown; i++ {
			if knownAt[i] <= step || scheduled[i] || budget.Available() < uint64(p.ChunkBytes) {
				continue
			}
			if budget.Spend(rbc.SpendReinjection, uint64(p.ChunkBytes)) == nil {
				scheduled[i] = true
				knownAt[i] = minInt(knownAt[i], step+1)
			}
		}
		if tailDeadline && step == p.SoftDeadlineStep {
			for i := 0; i < n; i++ {
				if knownAt[i] <= step || scheduled[i] || budget.Available() < uint64(p.ChunkBytes) {
					continue
				}
				if budget.Spend(rbc.SpendReinjection, uint64(p.ChunkBytes)) == nil {
					scheduled[i] = true
					knownAt[i] = minInt(knownAt[i], step+1)
				}
			}
		}
	}
	return knownAt
}

func validateProfile(p Profile) error {
	if p.Version != ProfileVersion || p.Name == "" || p.Step <= 0 || p.ChunkBytes <= 0 || len(p.OriginalArrival) < 2 || p.SoftDeadlineStep < 1 {
		return ErrInvalidProfile
	}
	for _, step := range p.OriginalArrival {
		if step < 1 {
			return ErrInvalidProfile
		}
	}
	return nil
}

func prefixOrder(v []int) {
	for i := 1; i < len(v); i++ {
		if v[i] < v[i-1] {
			v[i] = v[i-1]
		}
	}
}

func quantile(in []time.Duration, pct int) time.Duration {
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

func spreadIndices(n, count int) []int {
	if n <= 0 || count <= 0 {
		return nil
	}
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxDuration(v []time.Duration) time.Duration {
	var out time.Duration
	for _, d := range v {
		if d > out {
			out = d
		}
	}
	return out
}
