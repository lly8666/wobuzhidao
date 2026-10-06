package qualificationdiag

import "testing"

func TestClientStageTimingRequiresExplicitOptInAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name, env, path string
		want, wantErr   bool
	}{
		{"ordinary", "", "", false, false},
		{"ordinary diagnostics", "", "diagnostic.jsonl", false, false},
		{"explicit off", "0", "diagnostic.jsonl", false, false},
		{"qualified", "1", "diagnostic.jsonl", true, false},
		{"no output", "1", "", false, true},
		{"invalid", "true", "diagnostic.jsonl", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WBD_QUALIFICATION_CLIENT_STAGE_TIMING", tc.env)
			got, err := ClientStageTimingEnabled(tc.path)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("enabled=%t err=%v want=%t error=%t", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
