package benchmark

import (
	"encoding/json"
	"testing"
)

func TestStandardReportSchemaAndOraclePin(t *testing.T) {
	r, err := StandardReport()
	if err != nil {
		t.Fatal(err)
	}
	if r.SchemaVersion != ReportSchemaVersion || len(r.Profiles) < 5 || len(r.Results) == 0 {
		t.Fatalf("bad report: %+v", r)
	}
	if r.QUICOracle.Tag != "v0.61.0" || r.QUICOracle.CommitShort != "579ee19" || r.QUICOracle.MinimumGo != "1.25.0" {
		t.Fatalf("bad QUIC pin: %+v", r.QUICOracle)
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatal(err)
	}
}
