//go:build windows

package windowsdiag

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLoggingProcessWaitReadyReturnsEarlyChildExit(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "support-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	cmd := exec.Command("cmd.exe", "/d", "/c", "exit 23")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &loggingProcess{name: "faketcp", cmd: cmd, log: &logger{file: f}}
	started := time.Now()
	err = p.WaitReady("WBD_SINGLE_FLOW_BOOTSTRAP_READY", 5*time.Second)
	elapsed := time.Since(started)
	_ = cmd.Wait()
	if err == nil {
		t.Fatal("expected early child exit before readiness marker")
	}
	if !strings.Contains(err.Error(), "exited before readiness marker") || !strings.Contains(err.Error(), "exit_code=23") {
		t.Fatalf("unexpected early-exit diagnostic: %v", err)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("early child exit was hidden behind readiness timeout: elapsed=%s err=%v", elapsed, err)
	}
}
