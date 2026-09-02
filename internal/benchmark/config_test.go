package benchmark

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testSweepSpec() SweepSpec {
	return SweepSpec{
		Name: "upper-bound",
		Networks: []NetworkConfig{{
			Name: "50ms-1pct", Seed: 7001, Samples: 20, PayloadBytes: 256,
			RTTMinMS: 50, RTTMaxMS: 50, ImpairBasisPoints: 100,
			ExtraHoldMS: 200, SoftDeadlineMS: 100, SourceSpacingMS: 10,
		}},
		LaneCounts:            []int{1, 2, 4},
		WBDModes:              []string{"normal", "auto"},
		ReplicationCopies:     []int{2, 3},
		Windows:               []int{1, 4},
		FECProfiles:           []FECConfig{FECOff(), UDPspeederDefaultFEC(), UDPspeederOneToOneFEC()},
		IncludeNative:         true,
		IncludeUDP2RawSpeeder: true,
	}
}

func TestExpandSweepGeneratesRunnableAndExperimentalCases(t *testing.T) {
	cases, err := ExpandSweep(testSweepSpec())
	if err != nil {
		t.Fatal(err)
	}
	// WBD: 3 lanes * 2 modes * 2 windows * 3 fec profiles = 36.
	// Replication upper bound: 2 copy counts * 2 windows = 4.
	// Native: TCP + UDP = 2. Oracle: two enabled FEC profiles = 2.
	if len(cases) != 44 {
		t.Fatalf("cases=%d want=44", len(cases))
	}
	var runnable, replicated, experimental, oracle11 int
	for _, c := range cases {
		switch c.Engine {
		case EngineWBDReal:
			if !c.Runnable || c.FEC.Enabled {
				t.Fatalf("bad runnable case: %#v", c)
			}
			runnable++
		case EngineWBDReplicated:
			if !c.Runnable || c.LaneCount < 2 || c.LaneCount > 3 || c.FEC.Enabled {
				t.Fatalf("bad replicated case: %#v", c)
			}
			replicated++
		case EngineWBDFECExperiment:
			if c.Runnable || !c.FEC.Enabled {
				t.Fatalf("bad FEC case: %#v", c)
			}
			experimental++
		case EngineUDP2RawSpeeder:
			if c.Oracle == nil {
				t.Fatalf("missing oracle config: %#v", c)
			}
			if c.FEC.DataShards == 20 && c.FEC.ParityShards == 20 {
				oracle11++
				if c.FEC.Multiplier() != 2.0 {
					t.Fatalf("1:1 oracle multiplier=%.3f", c.FEC.Multiplier())
				}
			}
		}
	}
	if runnable != 12 || replicated != 4 || experimental != 24 || oracle11 != 1 {
		t.Fatalf("runnable=%d replicated=%d experimental=%d oracle11=%d", runnable, replicated, experimental, oracle11)
	}
}

func TestConfigurableLaneCountsRunClean(t *testing.T) {
	n := NetworkConfig{Name: "clean", Seed: 9001, Samples: 8, PayloadBytes: 128, RTTMinMS: 10, RTTMaxMS: 10, SoftDeadlineMS: 100, SourceSpacingMS: 0}
	for _, lanes := range []int{1, 2, 3, 4} {
		c := ExperimentCase{Engine: EngineWBDReal, Runnable: true, Network: n, LaneCount: lanes, WBDMode: "normal", Window: 1, FEC: FECOff()}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		obs, err := RunExperimentCase(ctx, c)
		cancel()
		if err != nil {
			t.Fatalf("lanes=%d: %v", lanes, err)
		}
		if obs.Completed != n.Samples || obs.GapEvents != 0 || obs.ReinjectionBytes != 0 {
			t.Fatalf("lanes=%d obs=%#v", lanes, obs)
		}
	}
}

func TestFECExperimentCannotMasqueradeAsRealRun(t *testing.T) {
	c := ExperimentCase{Engine: EngineWBDFECExperiment, Network: testSweepSpec().Networks[0], LaneCount: 2, WBDMode: "auto", Window: 4, FEC: UDPspeederOneToOneFEC()}
	_, err := RunExperimentCase(context.Background(), c)
	if !errors.Is(err, ErrCaseNotRunnable) {
		t.Fatalf("err=%v", err)
	}
}

func TestFECRejectsMoreThanTwoX(t *testing.T) {
	s := testSweepSpec()
	s.FECProfiles = []FECConfig{{Name: "too-much", Enabled: true, DataShards: 2, ParityShards: 3, Mode: 0, TimeoutMS: 8}}
	if _, err := ExpandSweep(s); !errors.Is(err, ErrInvalidSweepSpec) {
		t.Fatalf("err=%v", err)
	}
}

func TestBurstLengthGroupsImpairmentsOnSameLane(t *testing.T) {
	n := NetworkConfig{Name: "burst", Seed: 42, Samples: 40, PayloadBytes: 128, RTTMinMS: 50, RTTMaxMS: 50, ImpairBasisPoints: 1000, ExtraHoldMS: 200, SoftDeadlineMS: 100, SourceSpacingMS: 10, BurstLength: 3}
	p, err := n.RealProfile(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	s, err := BuildRealFaultSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	if countImpaired(s.Impaired) != 4 {
		t.Fatalf("impaired=%d want=4", countImpaired(s.Impaired))
	}
	for i, impaired := range s.Impaired {
		if impaired && i%2 != 0 {
			t.Fatalf("burst impairment escaped lane 1 at logical index %d", i)
		}
	}
}
