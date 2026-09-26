package fec

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"time"
)

var ErrInvalidSimulationConfig = errors.New("fec: invalid simulation config")

type ScheduleKind string

const (
	ScheduleOff    ScheduleKind = "off"
	ScheduleTail   ScheduleKind = "tail"
	ScheduleMicro  ScheduleKind = "micro"
	ScheduleCausal ScheduleKind = "causal"
)

type SimConfig struct {
	Schedule ScheduleKind

	Samples      int
	PayloadBytes int
	HeaderBytes  int
	OfferedMbps  float64
	CapacityMbps float64
	OneWay       time.Duration

	Loss        float64
	Seed        int64
	BurstLength int

	DataShards   int
	ParityShards int
	MicroData    int
	MicroParity  int
	CausalWindow int

	// ForceDropSourceIDs exists for deterministic scheduler-order tests. Random
	// loss is still applied to all other transmissions.
	ForceDropSourceIDs []int
}

type SimObservation struct {
	Schedule ScheduleKind `json:"schedule"`
	Samples  int          `json:"samples"`

	Delivered   int `json:"delivered"`
	Direct      int `json:"direct"`
	Recovered   int `json:"recovered"`
	Unrecovered int `json:"unrecovered"`

	SourceTx  int `json:"source_tx"`
	RepairTx  int `json:"repair_tx"`
	DroppedTx int `json:"dropped_tx"`

	DeliveryRatio       float64 `json:"delivery_ratio"`
	WireRatio           float64 `json:"wire_ratio"`
	OfferedUtilization  float64 `json:"offered_utilization"`
	MaxReadyRepairs     int     `json:"max_ready_repairs"`
	P50                 time.Duration `json:"-"`
	P95                 time.Duration `json:"-"`
	P99                 time.Duration `json:"-"`
	Max                 time.Duration `json:"-"`
	Mean                time.Duration `json:"-"`
	Drain               time.Duration `json:"-"`
}

type txKind uint8

const (
	txSource txKind = iota
	txRepair
)

type txEvent struct {
	kind txKind

	ideal    time.Duration
	sourceID int

	blockID   int
	dataCount int
	coverage  []int
	repairID  int
}

type rxEvent struct {
	tx      txEvent
	arrival time.Duration
	dropped bool
}

type blockState struct {
	dataCount int
	successes int
	sources   []int
}

type linearEquation struct {
	coverage []int
	repairID int
}

// RunScheduleSimulation evaluates one fixed FEC schedule. It deliberately does
// not contain an Auto controller. Source generation times are fixed by the
// offered payload rate, sources are always preferred over ready repairs, and
// repair traffic therefore shows up as serialization delay, backlog, recovery
// delay, or drain time instead of silently throttling source generation.
func RunScheduleSimulation(cfg SimConfig) (SimObservation, error) {
	if err := validateSimConfig(cfg); err != nil {
		return SimObservation{}, err
	}

	future := buildSchedule(cfg)
	rx, maxReady, drain := transmit(cfg, future)
	obs := decodeSimulation(cfg, rx)

	obs.Schedule = cfg.Schedule
	obs.Samples = cfg.Samples
	obs.MaxReadyRepairs = maxReady
	obs.Drain = drain

	shardBytes := cfg.PayloadBytes + cfg.HeaderBytes
	obs.WireRatio = float64((obs.SourceTx+obs.RepairTx)*shardBytes) /
		float64(cfg.Samples*cfg.PayloadBytes)
	obs.OfferedUtilization = cfg.OfferedMbps * (1 + repairRatioFor(cfg)) / cfg.CapacityMbps
	return obs, nil
}

func validateSimConfig(cfg SimConfig) error {
	if cfg.Samples <= 0 || cfg.PayloadBytes <= 0 || cfg.HeaderBytes < 0 ||
		cfg.OfferedMbps <= 0 || cfg.CapacityMbps <= 0 || cfg.OneWay < 0 ||
		cfg.Loss < 0 || cfg.Loss >= 1 || cfg.BurstLength < 0 {
		return ErrInvalidSimulationConfig
	}

	switch cfg.Schedule {
	case ScheduleOff:
	case ScheduleTail:
		if cfg.DataShards <= 0 || cfg.ParityShards < 0 {
			return ErrInvalidSimulationConfig
		}
	case ScheduleMicro:
		if cfg.MicroData <= 0 || cfg.MicroParity < 0 {
			return ErrInvalidSimulationConfig
		}
	case ScheduleCausal:
		if cfg.DataShards <= 0 || cfg.ParityShards <= 0 || cfg.CausalWindow <= 0 {
			return ErrInvalidSimulationConfig
		}
	default:
		return ErrInvalidSimulationConfig
	}
	return nil
}

func repairRatioFor(cfg SimConfig) float64 {
	switch cfg.Schedule {
	case ScheduleOff:
		return 0
	case ScheduleTail, ScheduleCausal:
		return float64(cfg.ParityShards) / float64(cfg.DataShards)
	case ScheduleMicro:
		return float64(cfg.MicroParity) / float64(cfg.MicroData)
	default:
		return 0
	}
}

func sourceInterval(cfg SimConfig) time.Duration {
	sec := float64(cfg.PayloadBytes*8) / (cfg.OfferedMbps * 1e6)
	return time.Duration(sec * float64(time.Second))
}

func serializationTime(cfg SimConfig) time.Duration {
	sec := float64((cfg.PayloadBytes+cfg.HeaderBytes)*8) / (cfg.CapacityMbps * 1e6)
	d := time.Duration(sec * float64(time.Second))
	if d <= 0 {
		return time.Nanosecond
	}
	return d
}

func buildSchedule(cfg SimConfig) []txEvent {
	interval := sourceInterval(cfg)
	events := make([]txEvent, 0, cfg.Samples*2)
	for i := 0; i < cfg.Samples; i++ {
		events = append(events, txEvent{
			kind:     txSource,
			ideal:    time.Duration(i) * interval,
			sourceID: i,
			blockID:  -1,
		})
	}

	switch cfg.Schedule {
	case ScheduleTail:
		events = append(events, buildBlockRepairs(
			cfg.Samples, cfg.DataShards, cfg.ParityShards, interval,
		)...)
	case ScheduleMicro:
		events = append(events, buildBlockRepairs(
			cfg.Samples, cfg.MicroData, cfg.MicroParity, interval,
		)...)
	case ScheduleCausal:
		acc := 0
		repairID := 0
		for i := 0; i < cfg.Samples; i++ {
			acc += cfg.ParityShards
			for acc >= cfg.DataShards {
				start := i - cfg.CausalWindow + 1
				if start < 0 {
					start = 0
				}
				coverage := make([]int, i-start+1)
				for j := range coverage {
					coverage[j] = start + j
				}
				events = append(events, txEvent{
					kind:     txRepair,
					ideal:    time.Duration(i) * interval,
					coverage: coverage,
					repairID: repairID,
					blockID:  -1,
				})
				repairID++
				acc -= cfg.DataShards
			}
		}

		// Flush fractional repair debt at the end of a finite experiment so the
		// simulator does not make a short sample artificially stronger/weaker
		// only because its length was not an exact K multiple.
		if acc > 0 {
			i := cfg.Samples - 1
			start := i - cfg.CausalWindow + 1
			if start < 0 {
				start = 0
			}
			coverage := make([]int, i-start+1)
			for j := range coverage {
				coverage[j] = start + j
			}
			events = append(events, txEvent{
				kind:     txRepair,
				ideal:    time.Duration(i) * interval,
				coverage: coverage,
				repairID: repairID,
				blockID:  -1,
			})
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].ideal != events[j].ideal {
			return events[i].ideal < events[j].ideal
		}
		// Sources win ties. This is a core product invariant: a repair that
		// becomes available at the same instant must not intentionally hold an
		// already-ready systematic source.
		return events[i].kind < events[j].kind
	})
	return events
}

func buildBlockRepairs(samples, k, r int, interval time.Duration) []txEvent {
	if r == 0 {
		return nil
	}

	out := make([]txEvent, 0, (samples+k-1)/k*r)
	blockID := 0
	for start := 0; start < samples; start += k {
		end := start + k
		if end > samples {
			end = samples
		}
		count := end - start
		parity := int(math.Ceil(float64(r) * float64(count) / float64(k)))
		if parity < 1 && r > 0 {
			parity = 1
		}

		coverage := make([]int, count)
		for j := range coverage {
			coverage[j] = start + j
		}
		ideal := time.Duration(end-1) * interval
		for j := 0; j < parity; j++ {
			out = append(out, txEvent{
				kind:      txRepair,
				ideal:     ideal,
				blockID:   blockID,
				dataCount: count,
				coverage:  append([]int(nil), coverage...),
				repairID:  j,
			})
		}
		blockID++
	}
	return out
}

type simulationLoss struct {
	rng *rand.Rand
	p   float64

	burstLength int
	bad         bool
	pGoodBad    float64
	pBadGood    float64

	forceSources map[int]bool
}

func newSimulationLoss(cfg SimConfig) *simulationLoss {
	force := make(map[int]bool, len(cfg.ForceDropSourceIDs))
	for _, id := range cfg.ForceDropSourceIDs {
		force[id] = true
	}

	m := &simulationLoss{
		rng:          rand.New(rand.NewSource(cfg.Seed)),
		p:            cfg.Loss,
		burstLength:  cfg.BurstLength,
		forceSources: force,
	}

	// BurstLength > 1 uses a two-state Gilbert-style trace with a 100% loss
	// bad state. The transition rates are chosen so the stationary bad-state
	// probability is approximately p and the mean bad run is BurstLength.
	if cfg.BurstLength > 1 && cfg.Loss > 0 {
		m.pBadGood = 1 / float64(cfg.BurstLength)
		m.pGoodBad = cfg.Loss / (1-cfg.Loss) * m.pBadGood
		if m.pGoodBad > 1 {
			m.pGoodBad = 1
		}
		m.bad = m.rng.Float64() < cfg.Loss
	}
	return m
}

func (m *simulationLoss) drop(ev txEvent) bool {
	if ev.kind == txSource && m.forceSources[ev.sourceID] {
		return true
	}
	if m.p == 0 {
		return false
	}
	if m.burstLength <= 1 {
		return m.rng.Float64() < m.p
	}

	drop := m.bad
	if m.bad {
		if m.rng.Float64() < m.pBadGood {
			m.bad = false
		}
	} else if m.rng.Float64() < m.pGoodBad {
		m.bad = true
	}
	return drop
}

func transmit(cfg SimConfig, future []txEvent) ([]rxEvent, int, time.Duration) {
	ser := serializationTime(cfg)
	loss := newSimulationLoss(cfg)

	var now time.Duration
	idx := 0
	sources := make([]txEvent, 0)
	repairs := make([]txEvent, 0)
	sourceHead := 0
	repairHead := 0
	maxRepairs := 0
	out := make([]rxEvent, 0, len(future))

	enqueueReady := func() {
		for idx < len(future) && future[idx].ideal <= now {
			if future[idx].kind == txSource {
				sources = append(sources, future[idx])
			} else {
				repairs = append(repairs, future[idx])
			}
			idx++
		}
		if ready := len(repairs) - repairHead; ready > maxRepairs {
			maxRepairs = ready
		}
	}

	for idx < len(future) || sourceHead < len(sources) || repairHead < len(repairs) {
		enqueueReady()
		if sourceHead >= len(sources) && repairHead >= len(repairs) {
			now = future[idx].ideal
			enqueueReady()
		}

		var ev txEvent
		if sourceHead < len(sources) {
			ev = sources[sourceHead]
			sourceHead++
		} else {
			ev = repairs[repairHead]
			repairHead++
		}

		if now < ev.ideal {
			now = ev.ideal
		}
		finish := now + ser
		out = append(out, rxEvent{
			tx:      ev,
			arrival: finish + cfg.OneWay,
			dropped: loss.drop(ev),
		})
		now = finish
	}

	lastSourceIdeal := time.Duration(cfg.Samples-1) * sourceInterval(cfg)
	drain := now - lastSourceIdeal
	if drain < 0 {
		drain = 0
	}
	return out, maxRepairs, drain
}

func decodeSimulation(cfg SimConfig, rx []rxEvent) SimObservation {
	completed := make([]time.Duration, cfg.Samples)
	direct := make([]bool, cfg.Samples)
	generated := make([]time.Duration, cfg.Samples)
	interval := sourceInterval(cfg)
	for i := range generated {
		generated[i] = time.Duration(i) * interval
	}

	obs := SimObservation{}
	blocks := map[int]*blockState{}
	equations := make([]linearEquation, 0)

	for _, r := range rx {
		if r.tx.kind == txSource {
			obs.SourceTx++
		} else {
			obs.RepairTx++
		}
		if r.dropped {
			obs.DroppedTx++
			continue
		}

		switch r.tx.kind {
		case txSource:
			id := r.tx.sourceID
			if completed[id] == 0 {
				completed[id] = r.arrival
				direct[id] = true
			}

			if cfg.Schedule == ScheduleTail || cfg.Schedule == ScheduleMicro {
				k := cfg.DataShards
				if cfg.Schedule == ScheduleMicro {
					k = cfg.MicroData
				}
				blockID := id / k
				st := blocks[blockID]
				if st == nil {
					start := blockID * k
					end := start + k
					if end > cfg.Samples {
						end = cfg.Samples
					}
					sources := make([]int, end-start)
					for i := range sources {
						sources[i] = start + i
					}
					st = &blockState{dataCount: end - start, sources: sources}
					blocks[blockID] = st
				}
				st.successes++
				if st.successes >= st.dataCount {
					recoverBlock(st, completed, r.arrival)
				}
			}

		case txRepair:
			if cfg.Schedule == ScheduleTail || cfg.Schedule == ScheduleMicro {
				st := blocks[r.tx.blockID]
				if st == nil {
					st = &blockState{
						dataCount: r.tx.dataCount,
						sources:   append([]int(nil), r.tx.coverage...),
					}
					blocks[r.tx.blockID] = st
				}
				st.successes++
				if st.successes >= st.dataCount {
					recoverBlock(st, completed, r.arrival)
				}
			} else if cfg.Schedule == ScheduleCausal {
				equations = append(equations, linearEquation{
					coverage: append([]int(nil), r.tx.coverage...),
					repairID: r.tx.repairID,
				})
				recoverLinear(equations, completed, r.arrival)
			}
		}
	}

	latencies := make([]time.Duration, 0, cfg.Samples)
	var sum time.Duration
	for i, at := range completed {
		if at == 0 {
			continue
		}
		latency := at - generated[i]
		latencies = append(latencies, latency)
		sum += latency
		if direct[i] {
			obs.Direct++
		} else {
			obs.Recovered++
		}
	}

	obs.Delivered = len(latencies)
	obs.Unrecovered = cfg.Samples - len(latencies)
	obs.DeliveryRatio = float64(obs.Delivered) / float64(cfg.Samples)
	if len(latencies) == 0 {
		return obs
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	obs.P50 = durationQuantile(latencies, 0.50)
	obs.P95 = durationQuantile(latencies, 0.95)
	obs.P99 = durationQuantile(latencies, 0.99)
	obs.Max = latencies[len(latencies)-1]
	obs.Mean = time.Duration(int64(sum) / int64(len(latencies)))
	return obs
}

func recoverBlock(st *blockState, completed []time.Duration, at time.Duration) {
	for _, id := range st.sources {
		if completed[id] == 0 {
			completed[id] = at
		}
	}
}

// recoverLinear models a causal systematic linear code with deterministic
// dense equations over a finite prime field. Directly received sources are
// treated as known variables. For each connected set of unresolved variables,
// recovery is declared only when the successful received repair equations have
// full column rank. This is deliberately stricter than a repair-debt counter.
func recoverLinear(equations []linearEquation, completed []time.Duration, at time.Duration) {
	for {
		unresolved := make(map[int]struct{})
		active := make([]linearEquation, 0, len(equations))
		for _, eq := range equations {
			hasUnknown := false
			for _, id := range eq.coverage {
				if completed[id] == 0 {
					unresolved[id] = struct{}{}
					hasUnknown = true
				}
			}
			if hasUnknown {
				active = append(active, eq)
			}
		}
		if len(unresolved) == 0 || len(active) == 0 {
			return
		}

		parent := make(map[int]int, len(unresolved))
		for id := range unresolved {
			parent[id] = id
		}
		var find func(int) int
		find = func(x int) int {
			if parent[x] != x {
				parent[x] = find(parent[x])
			}
			return parent[x]
		}
		union := func(a, b int) {
			ra, rb := find(a), find(b)
			if ra != rb {
				parent[rb] = ra
			}
		}

		for _, eq := range active {
			first := -1
			for _, id := range eq.coverage {
				if _, ok := unresolved[id]; !ok {
					continue
				}
				if first < 0 {
					first = id
				} else {
					union(first, id)
				}
			}
		}

		components := map[int][]int{}
		for id := range unresolved {
			root := find(id)
			components[root] = append(components[root], id)
		}

		progress := false
		for root, variables := range components {
			sort.Ints(variables)
			rows := make([][]int, 0)
			for _, eq := range active {
				touches := false
				for _, id := range eq.coverage {
					if _, ok := unresolved[id]; ok && find(id) == root {
						touches = true
						break
					}
				}
				if !touches {
					continue
				}

				row := make([]int, len(variables))
				for j, id := range variables {
					if containsInt(eq.coverage, id) {
						row[j] = simulationCoefficient(eq.repairID, id)
					}
				}
				rows = append(rows, row)
			}

			if modularRank(rows) == len(variables) {
				for _, id := range variables {
					completed[id] = at
				}
				progress = true
			}
		}
		if !progress {
			return
		}
	}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

const simulationFieldPrime = 65521

func simulationCoefficient(repairID, sourceID int) int {
	x := 2 + ((repairID+1)*7919)%(simulationFieldPrime-3)
	return modularPow(x, sourceID+1, simulationFieldPrime)
}

func modularPow(base, exp, mod int) int {
	result := 1
	b := base % mod
	for exp > 0 {
		if exp&1 == 1 {
			result = int((int64(result) * int64(b)) % int64(mod))
		}
		b = int((int64(b) * int64(b)) % int64(mod))
		exp >>= 1
	}
	return result
}

func modularInverse(a int) int {
	return modularPow(a, simulationFieldPrime-2, simulationFieldPrime)
}

func modularRank(in [][]int) int {
	if len(in) == 0 {
		return 0
	}
	matrix := make([][]int, len(in))
	cols := 0
	for i := range in {
		matrix[i] = append([]int(nil), in[i]...)
		if len(matrix[i]) > cols {
			cols = len(matrix[i])
		}
	}

	rank := 0
	for col := 0; col < cols && rank < len(matrix); col++ {
		pivot := -1
		for row := rank; row < len(matrix); row++ {
			if matrix[row][col]%simulationFieldPrime != 0 {
				pivot = row
				break
			}
		}
		if pivot < 0 {
			continue
		}

		matrix[rank], matrix[pivot] = matrix[pivot], matrix[rank]
		inv := modularInverse((matrix[rank][col]%simulationFieldPrime + simulationFieldPrime) % simulationFieldPrime)
		for j := col; j < cols; j++ {
			matrix[rank][j] = int((int64(matrix[rank][j]) * int64(inv)) % simulationFieldPrime)
		}

		for row := 0; row < len(matrix); row++ {
			if row == rank {
				continue
			}
			factor := matrix[row][col] % simulationFieldPrime
			if factor < 0 {
				factor += simulationFieldPrime
			}
			if factor == 0 {
				continue
			}
			for j := col; j < cols; j++ {
				value := matrix[row][j] - int((int64(factor)*int64(matrix[rank][j]))%simulationFieldPrime)
				value %= simulationFieldPrime
				if value < 0 {
					value += simulationFieldPrime
				}
				matrix[row][j] = value
			}
		}
		rank++
	}
	return rank
}

func durationQuantile(xs []time.Duration, q float64) time.Duration {
	if len(xs) == 0 {
		return 0
	}
	if q <= 0 {
		return xs[0]
	}
	if q >= 1 {
		return xs[len(xs)-1]
	}
	pos := q * float64(len(xs)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return xs[lo]
	}
	fraction := pos - float64(lo)
	return time.Duration(float64(xs[lo])*(1-fraction) + float64(xs[hi])*fraction)
}
