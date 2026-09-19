// Package protection contains deterministic, non-wire experiments used to
// decide whether a protection mechanism earns admission into WBD.
package protection

import (
	"errors"
	"fmt"
	"sort"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

// Strategy is deliberately small. M9 compares already-available reactive
// reinjection and proactive duplication against the smallest useful XOR repair
// experiment under the same RBC entitlement.
type Strategy uint8

const (
	StrategyReinjection Strategy = iota + 1
	StrategyDuplicate
	StrategyXORRepair
)

func (s Strategy) String() string {
	switch s {
	case StrategyReinjection:
		return "reinjection"
	case StrategyDuplicate:
		return "duplicate"
	case StrategyXORRepair:
		return "xor-repair"
	default:
		return "unknown"
	}
}

// Scenario models a batch of equal-sized logical stream chunks. Normal source
// copies arrive at step 1. Entries in StallUntil delay the original carrier
// copy of that chunk until the given step, modeling TCP HOL/stall rather than
// permanent loss. Protection copies/repair symbols use a healthy path and
// arrive at step 1; gap-driven reinjection arrives one step after a gap becomes
// observable.
type Scenario struct {
	Name       string
	Chunks     [][]byte
	StallUntil map[int]int
}

// Result is intentionally a latency proxy, not a throughput benchmark. M10 will
// add real UDP/QUIC and network-profile oracles.
type Result struct {
	Scenario          string
	Strategy          Strategy
	Multiplier        rbc.MultiplierQ4
	CompletionStep    int
	ProtectionBytes   uint64
	DuplicateBytes    uint64
	ReinjectionBytes  uint64
	FECBytes          uint64
	XORRecovered      int
	ReinjectedChunks  int
	DuplicatedChunks  int
	EntitledBytes     uint64
	SourceBytes       uint64
}

var (
	ErrInvalidScenario = errors.New("invalid WBD protection experiment scenario")
	ErrInvalidStrategy = errors.New("invalid WBD protection experiment strategy")
)

type eventKind uint8

const (
	eventSource eventKind = iota + 1
	eventDuplicate
	eventReinjection
	eventParity
)

type event struct {
	kind  eventKind
	chunk int
	group int
	data  []byte
}

type parityState struct {
	data []byte
}

// Run executes one deterministic same-budget experiment.
func Run(sc Scenario, strategy Strategy, multiplier rbc.MultiplierQ4) (Result, error) {
	chunkSize, err := validateScenario(sc)
	if err != nil {
		return Result{}, err
	}
	if strategy != StrategyReinjection && strategy != StrategyDuplicate && strategy != StrategyXORRepair {
		return Result{}, ErrInvalidStrategy
	}
	if !multiplier.Valid() || multiplier < rbc.Multiplier15 || multiplier > rbc.Multiplier20 {
		return Result{}, rbc.ErrInvalidMultiplier
	}

	budget := rbc.NewBudget()
	for range sc.Chunks {
		if err := budget.AdmitSource(uint64(chunkSize), multiplier); err != nil {
			return Result{}, err
		}
	}

	events := make(map[int][]event)
	latestOriginal := 1
	for i, data := range sc.Chunks {
		arrival := 1
		if until, ok := sc.StallUntil[i]; ok {
			if until < 1 {
				return Result{}, fmt.Errorf("%w: stall step %d for chunk %d", ErrInvalidScenario, until, i)
			}
			arrival = until
		}
		if arrival > latestOriginal {
			latestOriginal = arrival
		}
		events[arrival] = append(events[arrival], event{kind: eventSource, chunk: i, data: clone(data)})
	}

	res := Result{Scenario: sc.Name, Strategy: strategy, Multiplier: multiplier}
	// Proactive protection uses the same entitlement ledger as later reactive
	// recovery. No strategy may overspend it.
	switch strategy {
	case StrategyDuplicate:
		count := int(budget.Available() / uint64(chunkSize))
		for _, i := range spreadIndices(len(sc.Chunks), count) {
			if err := budget.Spend(rbc.SpendDuplicate, uint64(chunkSize)); err != nil {
				return Result{}, err
			}
			events[1] = append(events[1], event{kind: eventDuplicate, chunk: i, data: clone(sc.Chunks[i])})
			res.DuplicatedChunks++
		}
	case StrategyXORRepair:
		for group, start := 0, 0; start+1 < len(sc.Chunks); group, start = group+1, start+2 {
			if budget.Available() < uint64(chunkSize) {
				break
			}
			if err := budget.Spend(rbc.SpendFEC, uint64(chunkSize)); err != nil {
				return Result{}, err
			}
			p := xor(sc.Chunks[start], sc.Chunks[start+1])
			events[1] = append(events[1], event{kind: eventParity, group: group, data: p})
		}
	}

	known := make([][]byte, len(sc.Chunks))
	parity := make(map[int]parityState)
	reinjectScheduled := make(map[int]bool)
	maxStep := latestOriginal + len(sc.Chunks) + 4
	for step := 1; step <= maxStep; step++ {
		for _, ev := range events[step] {
			switch ev.kind {
			case eventSource, eventDuplicate, eventReinjection:
				if known[ev.chunk] == nil {
					known[ev.chunk] = clone(ev.data)
				}
			case eventParity:
				parity[ev.group] = parityState{data: clone(ev.data)}
			}
		}

		if strategy == StrategyXORRepair {
			for changed := true; changed; {
				changed = false
				for group, p := range parity {
					left := group * 2
					right := left + 1
					if right >= len(known) {
						continue
					}
					switch {
					case known[left] == nil && known[right] != nil:
						known[left] = xor(p.data, known[right])
						res.XORRecovered++
						changed = true
					case known[right] == nil && known[left] != nil:
						known[right] = xor(p.data, known[left])
						res.XORRecovered++
						changed = true
					}
				}
			}
		}

		if allKnown(known) {
			res.CompletionStep = step
			break
		}

		// A gap is observable only after a later chunk is known. Reactive
		// reinjection is available to the reinjection baseline and to XORRepair
		// when proactive parity did not consume the full entitlement (e.g. 2.0x).
		if strategy == StrategyReinjection || strategy == StrategyXORRepair {
			for _, missing := range observableMissing(known) {
				if reinjectScheduled[missing] || budget.Available() < uint64(chunkSize) {
					continue
				}
				if err := budget.Spend(rbc.SpendReinjection, uint64(chunkSize)); err != nil {
					return Result{}, err
				}
				reinjectScheduled[missing] = true
				events[step+1] = append(events[step+1], event{kind: eventReinjection, chunk: missing, data: clone(sc.Chunks[missing])})
				res.ReinjectedChunks++
			}
		}
	}
	if res.CompletionStep == 0 {
		return Result{}, fmt.Errorf("%w: simulation failed to complete", ErrInvalidScenario)
	}

	snap := budget.Snapshot()
	res.ProtectionBytes = snap.SpentBytes
	res.DuplicateBytes = snap.DuplicateBytes
	res.ReinjectionBytes = snap.ReinjectionBytes
	res.FECBytes = snap.FECBytes
	res.EntitledBytes = snap.EntitledBytes
	res.SourceBytes = snap.SourceBytes
	return res, nil
}

func validateScenario(sc Scenario) (int, error) {
	if len(sc.Chunks) < 2 {
		return 0, fmt.Errorf("%w: need at least two chunks", ErrInvalidScenario)
	}
	size := len(sc.Chunks[0])
	if size == 0 {
		return 0, fmt.Errorf("%w: zero-sized chunk", ErrInvalidScenario)
	}
	for i, chunk := range sc.Chunks {
		if len(chunk) != size {
			return 0, fmt.Errorf("%w: chunk %d size %d want %d", ErrInvalidScenario, i, len(chunk), size)
		}
	}
	for i := range sc.StallUntil {
		if i < 0 || i >= len(sc.Chunks) {
			return 0, fmt.Errorf("%w: stall chunk %d", ErrInvalidScenario, i)
		}
	}
	return size, nil
}

func spreadIndices(n, count int) []int {
	if count <= 0 || n <= 0 {
		return nil
	}
	if count > n {
		count = n
	}
	out := make([]int, 0, count)
	seen := make(map[int]bool, count)
	for k := 0; k < count; k++ {
		i := (k * n) / count
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	sort.Ints(out)
	return out
}

func observableMissing(known [][]byte) []int {
	lastKnown := -1
	for i, b := range known {
		if b != nil {
			lastKnown = i
		}
	}
	if lastKnown <= 0 {
		return nil
	}
	out := make([]int, 0)
	for i := 0; i < lastKnown; i++ {
		if known[i] == nil {
			out = append(out, i)
		}
	}
	return out
}

func allKnown(known [][]byte) bool {
	for _, b := range known {
		if b == nil {
			return false
		}
	}
	return true
}

func xor(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }
