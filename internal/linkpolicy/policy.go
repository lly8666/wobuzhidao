package linkpolicy

import (
	"errors"
	"fmt"
	"math"
)

const (
	DataShards   = 20
	MaxGameLanes = 4
)

var paritySteps = [...]int{0, 4, 8, 12, 16, 20}

type Mode string

const (
	ModeBalanced     Mode = "balanced"
	ModeConservative Mode = "conservative"
	ModeGame         Mode = "game"
)

type LinkSpeedMode string

const (
	LinkSpeedAuto   LinkSpeedMode = "auto"
	LinkSpeedManual LinkSpeedMode = "manual"
)

type Observation struct {
	// CapacityMbps is the automatic estimator's path service-capacity result,
	// not application goodput. Packet loss is estimated independently.
	CapacityMbps float64

	// LinkSpeedMode selects the capacity authority. Auto uses CapacityMbps.
	// Manual is intentionally forceful and uses ManualLinkSpeedMbps even when
	// it disagrees with the automatic estimate.
	LinkSpeedMode       LinkSpeedMode
	ManualLinkSpeedMbps float64

	Loss      float64
	MeanBurst float64

	// CarrierExpansion is observed FakeTCP/raw transmission expansion after
	// retransmissions. 1 means one carrier byte per logical WBD shard byte.
	CarrierExpansion float64

	PayloadBytes int
	FramingBytes int

	TargetDelivery float64
	MaxWireUtil    float64

	// Game lanes and FEC are orthogonal. RequestedLanes is the user's floor.
	// When AutoAddLanes is enabled, the formula may only add lanes up to
	// GameMaxLanes; it never changes ModeGame into another mode and never drops
	// below the requested floor. All lanes race the same logical datagram.
	GameRequestedLanes int
	GameAutoAddLanes   bool
	GameMaxLanes       int
	GameRaceTarget     float64
}

type Recommendation struct {
	Mode Mode

	DataShards   int
	ParityShards int
	FECProfile   string

	LinkSpeedMode         LinkSpeedMode
	EffectiveCapacityMbps float64
	InnerMbps              float64
	PredictedDelivery      float64
	PerLaneWireExpansion   float64
	WireExpansion          float64

	LaneCount        int
	AutoLaneAdded    bool
	ExperimentalLane bool

	// GameLaneEligible is retained for calibration/report compatibility. It no
	// longer means "low-loss only"; game mode is always allowed and this field
	// simply reports whether the current plan uses more than one lane.
	GameLaneEligible bool

	MeetsTarget bool
}

func DefaultObservation(capacityMbps, loss, meanBurst float64) Observation {
	return Observation{
		CapacityMbps:       capacityMbps,
		LinkSpeedMode:      LinkSpeedAuto,
		Loss:               loss,
		MeanBurst:          meanBurst,
		CarrierExpansion:   1,
		PayloadBytes:       1200,
		FramingBytes:       56,
		TargetDelivery:     0.995,
		MaxWireUtil:        0.92,
		GameRequestedLanes: 2,
		GameAutoAddLanes:   false,
		GameMaxLanes:       MaxGameLanes,
		GameRaceTarget:     0.9995,
	}
}

func Recommend(obs Observation, mode Mode) (Recommendation, error) {
	if err := validateObservation(obs); err != nil {
		return Recommendation{}, err
	}
	if mode != ModeBalanced && mode != ModeConservative && mode != ModeGame {
		return Recommendation{}, fmt.Errorf("linkpolicy: unsupported mode %q", mode)
	}

	capacity := effectiveCapacity(obs)
	parity, predicted, meets := chooseParity(obs.Loss, obs.MeanBurst, obs.TargetDelivery)
	if mode == ModeConservative {
		parity = bumpParity(parity)
		predicted = ExpectedDelivery(DataShards, parity, obs.Loss, obs.MeanBurst)
		meets = predicted >= obs.TargetDelivery
	}

	lanes := 1
	autoAdded := false
	if mode == ModeGame {
		lanes = obs.GameRequestedLanes
		if obs.GameAutoAddLanes {
			auto := autoGameLaneCount(obs)
			if auto > lanes {
				lanes = auto
				autoAdded = true
			}
		}
	}

	perLaneExpansion := framingExpansion(obs.PayloadBytes, obs.FramingBytes) * fecExpansion(parity) * obs.CarrierExpansion
	totalExpansion := perLaneExpansion * float64(lanes)
	inner := capacity * obs.MaxWireUtil / totalExpansion

	return Recommendation{
		Mode:                  mode,
		DataShards:            DataShards,
		ParityShards:          parity,
		FECProfile:            profileName(parity),
		LinkSpeedMode:         obs.LinkSpeedMode,
		EffectiveCapacityMbps: capacity,
		InnerMbps:             inner,
		PredictedDelivery:     predicted,
		PerLaneWireExpansion:  perLaneExpansion,
		WireExpansion:         totalExpansion,
		LaneCount:             lanes,
		AutoLaneAdded:         autoAdded,
		ExperimentalLane:      lanes > 1,
		GameLaneEligible:      mode == ModeGame && lanes > 1,
		MeetsTarget:           meets,
	}, nil
}

func effectiveCapacity(obs Observation) float64 {
	if obs.LinkSpeedMode == LinkSpeedManual {
		return obs.ManualLinkSpeedMbps
	}
	return obs.CapacityMbps
}

// autoGameLaneCount converts observed raw-path risk into a lane count without a
// lookup table. It is a scheduling heuristic, not an independence guarantee:
// independently authenticated associations can still share the same physical
// congestion/loss cause. The formula therefore only controls how many copies
// are raced; MeetsTarget remains the per-lane FEC prediction.
func autoGameLaneCount(obs Observation) int {
	floor := obs.GameRequestedLanes
	limit := obs.GameMaxLanes
	if floor >= limit || obs.Loss <= 0 {
		return floor
	}

	// Burstiness increases the effective miss risk continuously instead of via
	// hard loss thresholds. sqrt keeps a long bad run from immediately pinning
	// every mildly lossy path to four lanes.
	risk := obs.Loss * math.Sqrt(obs.MeanBurst)
	if risk < 1e-9 {
		return floor
	}
	if risk > 0.95 {
		risk = 0.95
	}
	residual := 1 - obs.GameRaceTarget
	need := floor
	if residual > 0 && residual < 1 {
		n := int(math.Ceil(math.Log(residual) / math.Log(risk)))
		if n > need {
			need = n
		}
	}
	if need > limit {
		need = limit
	}
	if need < floor {
		need = floor
	}
	return need
}

func validateObservation(obs Observation) error {
	if obs.LinkSpeedMode != LinkSpeedAuto && obs.LinkSpeedMode != LinkSpeedManual {
		return errors.New("linkpolicy: link speed mode must be auto or manual")
	}
	if math.IsNaN(obs.CapacityMbps) || math.IsInf(obs.CapacityMbps, 0) || obs.CapacityMbps < 0 {
		return errors.New("linkpolicy: automatic capacity must be finite and non-negative")
	}
	if obs.LinkSpeedMode == LinkSpeedAuto && obs.CapacityMbps <= 0 {
		return errors.New("linkpolicy: automatic capacity must be positive in auto mode")
	}
	if obs.LinkSpeedMode == LinkSpeedManual && (obs.ManualLinkSpeedMbps <= 0 || math.IsNaN(obs.ManualLinkSpeedMbps) || math.IsInf(obs.ManualLinkSpeedMbps, 0)) {
		return errors.New("linkpolicy: manual link speed must be finite and positive")
	}
	if obs.Loss < 0 || obs.Loss >= 1 || math.IsNaN(obs.Loss) {
		return errors.New("linkpolicy: loss must be in [0,1)")
	}
	if obs.MeanBurst < 1 || math.IsNaN(obs.MeanBurst) || math.IsInf(obs.MeanBurst, 0) {
		return errors.New("linkpolicy: mean burst must be finite and >= 1")
	}
	if obs.CarrierExpansion < 1 || math.IsNaN(obs.CarrierExpansion) || math.IsInf(obs.CarrierExpansion, 0) {
		return errors.New("linkpolicy: carrier expansion must be finite and >= 1")
	}
	if obs.PayloadBytes <= 0 || obs.FramingBytes < 0 {
		return errors.New("linkpolicy: invalid payload/framing size")
	}
	if obs.TargetDelivery <= 0 || obs.TargetDelivery > 1 {
		return errors.New("linkpolicy: target delivery must be in (0,1]")
	}
	if obs.MaxWireUtil <= 0 || obs.MaxWireUtil > 1 {
		return errors.New("linkpolicy: max wire utilization must be in (0,1]")
	}
	if obs.GameRequestedLanes < 1 || obs.GameRequestedLanes > MaxGameLanes {
		return fmt.Errorf("linkpolicy: requested game lanes must be in [1,%d]", MaxGameLanes)
	}
	if obs.GameMaxLanes < obs.GameRequestedLanes || obs.GameMaxLanes > MaxGameLanes {
		return fmt.Errorf("linkpolicy: max game lanes must be in [%d,%d]", obs.GameRequestedLanes, MaxGameLanes)
	}
	if obs.GameRaceTarget <= 0 || obs.GameRaceTarget >= 1 {
		return errors.New("linkpolicy: game race target must be in (0,1)")
	}
	return nil
}

func chooseParity(loss, meanBurst, target float64) (parity int, predicted float64, meets bool) {
	for _, r := range paritySteps {
		p := ExpectedDelivery(DataShards, r, loss, meanBurst)
		if p >= target {
			return r, p, true
		}
	}
	r := paritySteps[len(paritySteps)-1]
	return r, ExpectedDelivery(DataShards, r, loss, meanBurst), false
}

func bumpParity(parity int) int {
	for i, r := range paritySteps {
		if parity <= r {
			if i+1 < len(paritySteps) {
				return paritySteps[i+1]
			}
			return r
		}
	}
	return paritySteps[len(paritySteps)-1]
}

func profileName(parity int) string {
	if parity <= 0 {
		return "off"
	}
	return fmt.Sprintf("%d:%d", DataShards, parity)
}

func fecExpansion(parity int) float64 {
	if parity <= 0 {
		return 1
	}
	return float64(DataShards+parity) / float64(DataShards)
}

func framingExpansion(payloadBytes, framingBytes int) float64 {
	return float64(payloadBytes+framingBytes) / float64(payloadBytes)
}

// ExpectedDelivery models first-complete systematic delivery under the same
// two-state Gilbert-style loss shape used by the simulator: the bad state
// drops every transmission, its mean run is meanBurst, and the stationary bad
// probability is loss. FEC succeeds when no more than parity shards are lost.
// The returned value is the expected delivered fraction of the K systematic
// source datagrams, not the probability that every source in the block arrives.
func ExpectedDelivery(k, parity int, loss, meanBurst float64) float64 {
	if k <= 0 || parity < 0 || loss < 0 || loss >= 1 || meanBurst < 1 {
		return 0
	}
	if loss == 0 {
		return 1
	}
	if parity == 0 {
		return 1 - loss
	}

	// meanBurst==1 is iid loss: from either previous state, next packet is bad
	// with probability loss. For meanBurst>1, pBadGood fixes the mean bad run;
	// pGoodBad follows from the requested stationary bad probability.
	pBadGood := 1 - loss
	pGoodBad := loss
	if meanBurst > 1 {
		pBadGood = 1 / meanBurst
		pGoodBad = loss / (1-loss) * pBadGood
		if pGoodBad > 1 {
			pGoodBad = 1
		}
	}

	total := k + parity
	// dp[state][totalLosses][systematicLosses]
	good := make([][]float64, total+1)
	bad := make([][]float64, total+1)
	for i := 0; i <= total; i++ {
		good[i] = make([]float64, k+1)
		bad[i] = make([]float64, k+1)
	}
	good[0][0] = 1 - loss
	bad[1][1] = loss

	for pos := 1; pos < total; pos++ {
		ng := make([][]float64, total+1)
		nb := make([][]float64, total+1)
		for i := 0; i <= total; i++ {
			ng[i] = make([]float64, k+1)
			nb[i] = make([]float64, k+1)
		}
		isSystematic := pos < k
		for losses := 0; losses <= pos; losses++ {
			for sysLosses := 0; sysLosses <= k; sysLosses++ {
				if v := good[losses][sysLosses]; v != 0 {
					ng[losses][sysLosses] += v * (1 - pGoodBad)
					ns := sysLosses
					if isSystematic {
						ns++
					}
					nb[losses+1][ns] += v * pGoodBad
				}
				if v := bad[losses][sysLosses]; v != 0 {
					ng[losses][sysLosses] += v * pBadGood
					ns := sysLosses
					if isSystematic {
						ns++
					}
					nb[losses+1][ns] += v * (1 - pBadGood)
				}
			}
		}
		good, bad = ng, nb
	}

	expectedUnrecovered := 0.0
	for losses := parity + 1; losses <= total; losses++ {
		for sysLosses := 1; sysLosses <= k; sysLosses++ {
			expectedUnrecovered += float64(sysLosses) * (good[losses][sysLosses] + bad[losses][sysLosses])
		}
	}
	delivery := 1 - expectedUnrecovered/float64(k)
	if delivery < 0 {
		return 0
	}
	if delivery > 1 {
		return 1
	}
	return delivery
}
