package fec

import (
	"errors"
	"math"
)

var ErrInvalidPlannerInput = errors.New("fec: invalid planner input")

// BlockFailureProbability returns the ideal iid-loss probability that a
// systematic (data+parity,data) MDS block cannot be reconstructed because more
// than parity shards are lost. It is a planning bound, not a burst-loss model.
func BlockFailureProbability(data, parity int, loss float64) (float64, error) {
	if data <= 0 || parity < 0 || loss < 0 || loss >= 1 {
		return 0, ErrInvalidPlannerInput
	}
	if loss == 0 {
		return 0, nil
	}
	n := data + parity
	q := 1 - loss
	var tail float64
	for lost := parity + 1; lost <= n; lost++ {
		logC, _ := math.Lgamma(float64(n + 1))
		logL, _ := math.Lgamma(float64(lost + 1))
		logR, _ := math.Lgamma(float64(n - lost + 1))
		logP := logC - logL - logR + float64(lost)*math.Log(loss) + float64(n-lost)*math.Log(q)
		tail += math.Exp(logP)
	}
	if tail > 1 {
		return 1, nil
	}
	return tail, nil
}

// MinParityForTarget returns the smallest parity count at or below maxParity
// whose ideal iid block-failure probability is no greater than target.
func MinParityForTarget(data, maxParity int, loss, target float64) (int, float64, error) {
	if data <= 0 || maxParity < 0 || loss < 0 || loss >= 1 || target <= 0 || target >= 1 {
		return 0, 0, ErrInvalidPlannerInput
	}
	for parity := 0; parity <= maxParity; parity++ {
		p, err := BlockFailureProbability(data, parity, loss)
		if err != nil {
			return 0, 0, err
		}
		if p <= target {
			return parity, p, nil
		}
	}
	p, err := BlockFailureProbability(data, maxParity, loss)
	if err != nil {
		return 0, 0, err
	}
	return -1, p, nil
}

// RepairDebtLowerBound returns the repair/source ratio alpha for which the mean
// rate of successfully received repair equations equals the mean rate at which
// iid source loss creates missing-equation debt:
//
//     alpha * (1-loss) = loss
//     alpha = loss / (1-loss)
//
// A real controller must stay above this lower bound and also satisfy a tail
// target, capacity headroom, burst-loss margin and estimator hysteresis.
func RepairDebtLowerBound(loss float64) (float64, error) {
	if loss < 0 || loss >= 1 {
		return 0, ErrInvalidPlannerInput
	}
	return loss / (1 - loss), nil
}

// ApproxMeanNextRepairWait estimates the residual time from an arbitrary point
// in an evenly-spaced repair schedule to the next repair opportunity. It is a
// scheduling comparison helper only; it is not a decoder-latency guarantee.
func ApproxMeanNextRepairWait(sourceSpacingSeconds, repairRatio float64) (float64, error) {
	if sourceSpacingSeconds < 0 || repairRatio <= 0 {
		return 0, ErrInvalidPlannerInput
	}
	return sourceSpacingSeconds / (2 * repairRatio), nil
}
