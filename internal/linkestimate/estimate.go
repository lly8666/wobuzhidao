package linkestimate

import (
	"errors"
	"math"
	"sort"
	"time"
)

type ProbePacket struct {
	Seq        uint64
	Bytes      int
	SentAt     time.Time
	ReceivedAt time.Time
}

type TrainEstimate struct {
	ServiceMbps float64
	Loss        float64
	MeanBurst   float64
	PairSamples int
	Received    int
	Expected    int
	Confidence  float64
}

// EstimateProbeTrain estimates path service rate independently from loss.
// Missing packets contribute to Loss/MeanBurst but never stretch a packet-pair
// gap. Only surviving consecutive sequence pairs are used for rate samples.
// This is important on high-loss paths: a 20% loss trace must not turn a
// 100-Mbit serialization path into an apparent 80-Mbit path merely because
// fewer payload bytes survived.
func EstimateProbeTrain(firstSeq uint64, expected int, packets []ProbePacket) (TrainEstimate, error) {
	if expected < 2 {
		return TrainEstimate{}, errors.New("linkestimate: probe train needs at least two packets")
	}
	if len(packets) == 0 {
		return TrainEstimate{Loss: 1, MeanBurst: float64(expected), Expected: expected}, nil
	}

	bySeq := make(map[uint64]ProbePacket, expected)
	end := firstSeq + uint64(expected)
	for _, p := range packets {
		if p.Seq < firstSeq || p.Seq >= end || p.Bytes <= 0 || p.SentAt.IsZero() || p.ReceivedAt.IsZero() {
			continue
		}
		old, ok := bySeq[p.Seq]
		if !ok || p.ReceivedAt.Before(old.ReceivedAt) {
			bySeq[p.Seq] = p
		}
	}

	missing := 0
	missingRuns := 0
	inMissing := false
	for i := 0; i < expected; i++ {
		_, ok := bySeq[firstSeq+uint64(i)]
		if !ok {
			missing++
			if !inMissing {
				missingRuns++
				inMissing = true
			}
		} else {
			inMissing = false
		}
	}
	meanBurst := 1.0
	if missing != 0 && missingRuns != 0 {
		meanBurst = float64(missing) / float64(missingRuns)
		if meanBurst < 1 {
			meanBurst = 1
		}
	}

	rates := make([]float64, 0, expected-1)
	for i := 0; i+1 < expected; i++ {
		a, okA := bySeq[firstSeq+uint64(i)]
		b, okB := bySeq[firstSeq+uint64(i+1)]
		if !okA || !okB || !b.ReceivedAt.After(a.ReceivedAt) || !b.SentAt.After(a.SentAt) {
			continue
		}
		recvRate := mbpsFor(b.Bytes, b.ReceivedAt.Sub(a.ReceivedAt))
		sendRate := mbpsFor(b.Bytes, b.SentAt.Sub(a.SentAt))
		if recvRate <= 0 || sendRate <= 0 {
			continue
		}
		// Arrival compression cannot create a trustworthy sample faster than
		// the sender actually injected the pair. This mirrors the BBR idea of
		// bounding ACK/delivery rate by send rate.
		rates = append(rates, math.Min(recvRate, sendRate))
	}

	service := 0.0
	if len(rates) != 0 {
		service = robustQuantile(rates, 0.75)
	}
	pairNeed := math.Max(8, float64(expected-1)*0.25)
	confidence := math.Min(1, float64(len(rates))/pairNeed)
	return TrainEstimate{
		ServiceMbps: service,
		Loss:        float64(missing) / float64(expected),
		MeanBurst:   meanBurst,
		PairSamples: len(rates),
		Received:    len(bySeq),
		Expected:    expected,
		Confidence:  confidence,
	}, nil
}

func mbpsFor(bytes int, elapsed time.Duration) float64 {
	if bytes <= 0 || elapsed <= 0 {
		return 0
	}
	return float64(bytes*8) / elapsed.Seconds() / 1e6
}

func robustQuantile(values []float64, q float64) float64 {
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	median := quantileSorted(v, 0.5)
	if median > 0 {
		// Drop extreme batching/coalescing spikes while preserving legitimate
		// high-rate samples. Cross traffic normally pushes rates downward, so a
		// moderately upper quantile is preferable to the minimum/mean.
		limit := median * 4
		j := 0
		for _, x := range v {
			if x <= limit {
				v[j] = x
				j++
			}
		}
		v = v[:j]
	}
	if len(v) == 0 {
		return 0
	}
	return quantileSorted(v, q)
}

func quantileSorted(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	if q <= 0 {
		return v[0]
	}
	if q >= 1 {
		return v[len(v)-1]
	}
	pos := q * float64(len(v)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return v[lo]
	}
	f := pos - float64(lo)
	return v[lo]*(1-f) + v[hi]*f
}

// DeliveryRateMbps returns the BBR-style rate sample data/max(send, ack).
// Using the longer interval prevents ACK compression from manufacturing a rate
// faster than the flight was sent. Loss can remove individual samples, so a
// rolling max filter should be used over several RTTs rather than treating one
// missing/late ACK as a capacity collapse.
func DeliveryRateMbps(deliveredBytes uint64, sendElapsed, ackElapsed time.Duration) float64 {
	if deliveredBytes == 0 || sendElapsed <= 0 || ackElapsed <= 0 {
		return 0
	}
	elapsed := sendElapsed
	if ackElapsed > elapsed {
		elapsed = ackElapsed
	}
	return float64(deliveredBytes*8) / elapsed.Seconds() / 1e6
}

type RatePoint struct {
	At         time.Time
	Mbps       float64
	AppLimited bool
}

type RateWindow struct {
	Horizon time.Duration
	points  []RatePoint
}

func NewRateWindow(horizon time.Duration) *RateWindow {
	if horizon <= 0 {
		horizon = 10 * time.Second
	}
	return &RateWindow{Horizon: horizon}
}

func (w *RateWindow) Add(p RatePoint) {
	if p.At.IsZero() || p.Mbps <= 0 || math.IsNaN(p.Mbps) || math.IsInf(p.Mbps, 0) {
		return
	}
	w.points = append(w.points, p)
	w.prune(p.At)
	if len(w.points) > 512 {
		copy(w.points, w.points[len(w.points)-512:])
		w.points = w.points[:512]
	}
}

// Estimate returns a max-filtered service-rate estimate. Non-app-limited
// samples are authoritative when present. If only app-limited traffic exists,
// the result is explicitly marked as a lower bound so the caller can schedule
// a short chirp rather than silently treating it as line capacity.
func (w *RateWindow) Estimate(now time.Time) (mbps float64, samples int, lowerBound bool) {
	w.prune(now)
	for _, p := range w.points {
		if p.AppLimited {
			continue
		}
		if p.Mbps > mbps {
			mbps = p.Mbps
		}
		samples++
	}
	if samples != 0 {
		return mbps, samples, false
	}
	for _, p := range w.points {
		if p.Mbps > mbps {
			mbps = p.Mbps
		}
		samples++
	}
	return mbps, samples, samples != 0
}

func (w *RateWindow) prune(now time.Time) {
	if len(w.points) == 0 || now.IsZero() {
		return
	}
	cut := now.Add(-w.Horizon)
	i := 0
	for i < len(w.points) && w.points[i].At.Before(cut) {
		i++
	}
	if i != 0 {
		copy(w.points, w.points[i:])
		w.points = w.points[:len(w.points)-i]
	}
}

// ChirpRates returns logarithmically spaced pacing rates. A live probe can run
// several very short trains over this ladder and use only surviving consecutive
// pairs, so even a high-loss path can estimate serialization rate without a
// full speed-test transfer.
func ChirpRates(minMbps, maxMbps float64, steps int) ([]float64, error) {
	if minMbps <= 0 || maxMbps <= minMbps || steps < 2 || math.IsNaN(minMbps) || math.IsNaN(maxMbps) {
		return nil, errors.New("linkestimate: invalid chirp range")
	}
	out := make([]float64, steps)
	ratio := math.Pow(maxMbps/minMbps, 1/float64(steps-1))
	out[0] = minMbps
	for i := 1; i < steps; i++ {
		out[i] = out[i-1] * ratio
	}
	out[len(out)-1] = maxMbps
	return out, nil
}
