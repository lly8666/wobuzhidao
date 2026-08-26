package linkpolicy

import (
	"errors"
	"fmt"
	"math"
)

const DataShards = 20

var paritySteps = [...]int{0, 4, 8, 12, 16, 20}

type Mode string

const (
	ModeBalanced     Mode = "balanced"
	ModeConservative Mode = "conservative"
	ModeGame         Mode = "game"
)

type Observation struct {
	// CapacityMbps is measured path service capacity, not application goodput.
	// Packet loss must be estimated independently rather than baked into this
	// number, otherwise a lossy fast path looks like a genuinely slow path.
	CapacityMbps float64
	Loss         float64
	MeanBurst    float64

	// CarrierExpansion is observed FakeTCP/raw transmission expansion after
	// retransmissions. 1 means one carrier byte per logical WBD shard byte.
	CarrierExpansion float64

	PayloadBytes int
	FramingBytes int

	TargetDelivery float64
	MaxWireUtil    float64

	// Game mode may request two experimental lanes only on a low-loss,
	// low-burst path. LaneCount=2 is a request for a later lane data-plane; the
	// current release policy remains one lane.
	GameMinRawDelivery float64
	GameMaxMeanBurst   float64
}

type Recommendation struct {
	Mode Mode

	DataShards   int
	ParityShards int
	FECProfile   string

	InnerMbps         float64
	PredictedDelivery float64
	WireExpansion     float64

	LaneCount        int
	GameLaneEligible bool
	ExperimentalLane bool

	MeetsTarget bool
}

func DefaultObservation(capacityMbps, loss, meanBurst float64) Observation {
	return Observation{
		CapacityMbps:       capacityMbps,
		Loss:               loss,
		MeanBurst:          meanBurst,
		CarrierExpansion:   1,
		PayloadBytes:       1200,
		FramingBytes:       56,
		TargetDelivery:     0.995,
		MaxWireUtil:        0.92,
		GameMinRawDelivery: 0.97,
		GameMaxMeanBurst:   1.5,
	}
}

func Recommend(obs Observation, mode Mode) (Recommendation, error) {
	if err := validateObservation(obs); err != nil {
		return Recommendation{}, err
	}
	if mode != ModeBalanced && mode != ModeConservative && mode != ModeGame {
		return Recommendation{}, fmt.Errorf("linkpolicy: unsupported mode %q", mode)
	}

	parity, predicted, meets := chooseParity(obs.Loss, obs.MeanBurst, obs.TargetDelivery)
	if mode == ModeConservative {
		parity = bumpParity(parity)
		predicted = ExpectedDelivery(DataShards, parity, obs.Loss, obs.MeanBurst)
		meets = predicted >= obs.TargetDelivery
	}

	lanes := 1
	gameEligible := false
	if mode == ModeGame {
		rawDelivery := 1 - obs.Loss
		gameEligible = rawDelivery >= obs.GameMinRawDelivery && obs.MeanBurst <= obs.GameMaxMeanBurst
		if gameEligible {
			lanes = 2
		}
	}

	wireExpansion := framingExpansion(obs.PayloadBytes, obs.FramingBytes) * fecExpansion(parity) * obs.CarrierExpansion
	inner := obs.CapacityMbps * obs.MaxWireUtil / wireExpansion

	return Recommendation{
		Mode:              mode,
		DataShards:        DataShards,
		ParityShards:      parity,
		FECProfile:        profileName(parity),
		InnerMbps:         inner,
		PredictedDelivery: predicted,
		WireExpansion:     wireExpansion,
		LaneCount:         lanes,
		GameLaneEligible:  gameEligible,
		ExperimentalLane:  lanes > 1,
		MeetsTarget:       meets,
	}, nil
}

func validateObservation(obs Observation) error {
	if obs.CapacityMbps <= 0 || math.IsNaN(obs.CapacityMbps) || math.IsInf(obs.CapacityMbps, 0) {
		return errors.New("linkpolicy: capacity must be finite and positive")
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
	if obs.GameMinRawDelivery <= 0 || obs.GameMinRawDelivery > 1 {
		return errors.New("linkpolicy: game raw-delivery gate must be in (0,1]")
	}
	if obs.GameMaxMeanBurst < 1 {
		return errors.New("linkpolicy: game burst gate must be >= 1")
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
