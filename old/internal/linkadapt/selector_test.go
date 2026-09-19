package linkadapt

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestBetweenUsesUniqueLossMarksNotRetryAttempts(t *testing.T) {
	before := Snapshot{At: time.Unix(1, 0), Stats: faketcp.SenderStats{
		Enqueued: 100, EnqueuedBytes: 120000, LossMarked: 5, LossMarkedBytes: 6000,
		FastRetransmits: 5, RTOTransmits: 1, RetransmitBytes: 7200,
	}}
	after := Snapshot{At: time.Unix(21, 0), Stats: faketcp.SenderStats{
		Enqueued: 1100, EnqueuedBytes: 1320000, LossMarked: 105, LossMarkedBytes: 126000,
		FastRetransmits: 90, RTOTransmits: 41, RetransmitBytes: 157200,
	}}
	s, err := Between(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if s.Segments != 1000 || s.OriginalBytes != 1200000 || s.LossMarked != 100 || s.RetransmitAttempts != 125 {
		t.Fatalf("sample=%#v", s)
	}
	if math.Abs(s.LossRate()-0.10) > 1e-12 {
		t.Fatalf("loss=%f", s.LossRate())
	}
	if math.Abs(s.ARQFactor()-1.125) > 1e-12 {
		t.Fatalf("arq=%f", s.ARQFactor())
	}
}

func TestBetweenRejectsCounterReset(t *testing.T) {
	_, err := Between(
		Snapshot{At: time.Unix(1, 0), Stats: faketcp.SenderStats{Enqueued: 10}},
		Snapshot{At: time.Unix(2, 0), Stats: faketcp.SenderStats{Enqueued: 9}},
	)
	if !errors.Is(err, ErrCounterReset) {
		t.Fatalf("err=%v", err)
	}
}

func TestFixedRecommendationsUseConservativeWilsonUpper(t *testing.T) {
	cases := []struct {
		lost uint64
		want Profile
	}{
		{0, ProfileOff},
		{10, Profile20x4},
		{30, Profile20x8},
		{70, Profile20x12},
		{120, Profile20x16},
		{180, Profile20x20},
	}
	for _, tc := range cases {
		s := Sample{Segments: 1000, LossMarked: tc.lost}
		// Zero-loss needs a little more evidence before it is safe to select off.
		if tc.lost == 0 {
			s.Segments = 1024
		}
		got, err := RecommendFixed(s, 512)
		if err != nil {
			t.Fatalf("lost=%d err=%v", tc.lost, err)
		}
		if got.Profile != tc.want {
			t.Fatalf("lost=%d estimate=%.4f upper=%.4f got=%s want=%s", tc.lost, got.Estimate, got.WilsonHigh95, got.Profile.Name, tc.want.Name)
		}
	}
}

func TestInsufficientSampleAndProbeCost(t *testing.T) {
	s := Sample{Segments: 300}
	if _, err := RecommendFixed(s, 1024); !errors.Is(err, ErrInsufficientSamples) {
		t.Fatalf("err=%v", err)
	}
	if got := ProbeDeficit(s, 1024); got != 724 {
		t.Fatalf("deficit=%d", got)
	}
	bps := ProbeBitrate(1024, 128, 20*time.Second)
	if math.Abs(bps-52428.8) > 0.01 {
		t.Fatalf("probe bitrate=%f", bps)
	}
}

func TestLowLoadGate(t *testing.T) {
	s := Sample{Duration: 20 * time.Second, OriginalBytes: 1200000}
	if !s.LowLoad(200e6, 0.05) {
		t.Fatalf("%.3f Mbps should be low load", s.OriginalBitrate()/1e6)
	}
	busy := Sample{Duration: 20 * time.Second, OriginalBytes: 40_000_000}
	if busy.LowLoad(200e6, 0.05) {
		t.Fatal("16 Mbps should exceed a 10 Mbps low-load gate")
	}
}

func TestMaxInnerBitrateAccountsForFECAndShadowARQ(t *testing.T) {
	// 20:20 = 2x and 20% loss implies at least 1/(1-.2)=1.25x shadow
	// retransmission. At an 80% utilization target on a 200 Mbps path this
	// leaves 200*.8/(2*1.25) = 64 Mbps before header/ACK reserves.
	s := Sample{OriginalBytes: 1_200_000, RetransmitBytes: 120_000}
	bps, err := MaxInnerBitrate(Profile20x20, s, 0.20, RateBudget{
		CapacityBps: 200e6, TargetUtilization: 0.80, PayloadBytes: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(bps-64e6) > 1 {
		t.Fatalf("cap=%f want 64000000", bps)
	}

	// FEC off under the same loss/confidence leaves twice the payload budget.
	off, err := MaxInnerBitrate(ProfileOff, s, 0.20, RateBudget{
		CapacityBps: 200e6, TargetUtilization: 0.80, PayloadBytes: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(off-128e6) > 1 {
		t.Fatalf("off cap=%f want 128000000", off)
	}
}
