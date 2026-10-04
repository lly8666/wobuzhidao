package benchmark

import "github.com/lly8666/wobuzhidao/internal/rbc"

const ReportSchemaVersion = "wbd-benchmark-report/v1"

type OraclePlan struct {
	Implementation string `json:"implementation"`
	Tag            string `json:"tag"`
	CommitShort    string `json:"commit_short"`
	MinimumGo      string `json:"minimum_go"`
	Status         string `json:"status"`
}

type Report struct {
	SchemaVersion string     `json:"schema_version"`
	Profiles      []Profile  `json:"profiles"`
	Results       []Result   `json:"results"`
	QUICOracle    OraclePlan `json:"quic_oracle"`
}

// StandardReport is deterministic and contains no wall-clock socket samples.
// Real kernel socket observations are qualified separately by socket_test.go.
func StandardReport() (Report, error) {
	profiles := StandardProfiles()
	strategies := []struct {
		s Strategy
		m rbc.MultiplierQ4
	}{
		{StrategyNativeTCP, rbc.Multiplier10},
		{StrategyNativeUDP, rbc.Multiplier10},
		{StrategyWBDReinjection, rbc.Multiplier15},
		{StrategyWBDTailDeadline, rbc.Multiplier15},
		{StrategyWBDDuplicate, rbc.Multiplier15},
		{StrategyWBDXOR, rbc.Multiplier15},
		{StrategyWBDReinjection, rbc.Multiplier20},
		{StrategyWBDTailDeadline, rbc.Multiplier20},
		{StrategyWBDDuplicate, rbc.Multiplier20},
		{StrategyWBDXOR, rbc.Multiplier20},
	}
	out := Report{
		SchemaVersion: ReportSchemaVersion,
		Profiles:      profiles,
		QUICOracle: OraclePlan{
			Implementation: "github.com/quic-go/quic-go",
			Tag:            "v0.61.0",
			CommitShort:    "579ee19",
			MinimumGo:      "1.25.0",
			Status:         "pinned-plan-not-yet-integrated",
		},
	}
	for _, p := range profiles {
		for _, sm := range strategies {
			r, err := Run(p, sm.s, sm.m)
			if err != nil {
				return Report{}, err
			}
			out.Results = append(out.Results, r)
		}
	}
	return out, nil
}
