//go:build lifecycleacceptance

package acceptancefault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConsumeRulesAreBoundedAndLaneScoped(t *testing.T) {
	dir := t.TempDir()
	controlPath := filepath.Join(dir, "control.json")
	eventPath := filepath.Join(dir, "events.jsonl")
	t.Setenv("WBD_LIFECYCLE_ACCEPTANCE_CONTROL", controlPath)
	t.Setenv("WBD_LIFECYCLE_ACCEPTANCE_EVENTS", eventPath)

	write := func(epoch uint64, rules []rule) {
		t.Helper()
		data, err := json.Marshal(control{Epoch: epoch, Rules: rules})
		if err != nil { t.Fatal(err) }
		if err := os.WriteFile(controlPath, data, 0o644); err != nil { t.Fatal(err) }
	}
	write(1, []rule{{Kind: "health", Count: 2}, {Kind: "admission", Lane: 3, Count: 1}})
	if !Consume("health", 1) || !Consume("health", 4) || Consume("health", 1) {
		t.Fatal("wildcard health count not bounded")
	}
	if Consume("admission", 2) || !Consume("admission", 3) || Consume("admission", 3) {
		t.Fatal("lane-scoped admission rule mismatch")
	}
	write(2, []rule{{Kind: "detach", Lane: 1, Count: 1}})
	if !Consume("detach", 1) || Consume("detach", 1) {
		t.Fatal("epoch did not reset bounded rule")
	}
	if data, err := os.ReadFile(eventPath); err != nil || len(data) == 0 {
		t.Fatalf("missing acceptance event evidence: bytes=%d err=%v", len(data), err)
	}
}
