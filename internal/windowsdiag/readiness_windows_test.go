//go:build windows

package windowsdiag

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWaitReadyFailsFastWhenChildExits(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "support-*.jsonl")
	if err != nil { t.Fatal(err) }
	defer f.Close()

	cmd := exec.Command("cmd.exe", "/c", "exit", "/b", "7")
	if err := cmd.Start(); err != nil { t.Fatal(err) }
	p := &loggingProcess{
		name: "faketcp",
		cmd:  cmd,
		log:  &logger{file: f},
	}

	start := time.Now()
	err = p.WaitReady("READY role=client", 5*time.Second)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected child exit before readiness to fail")
	}
	if !strings.Contains(err.Error(), "exited before readiness marker") {
		t.Fatalf("unexpected readiness error: %v", err)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("child exit was hidden behind readiness timeout: elapsed=%s err=%v", elapsed, err)
	}
	if p.cmd.Process != nil {
		t.Fatal("exited child must be reaped and marked consumed before cleanup")
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("cleanup of already-exited child must be idempotent: %v", err)
	}
}

func TestWaitReadyAcceptsExistingCommandScopedMarker(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "support-*.jsonl")
	if err != nil { t.Fatal(err) }
	defer f.Close()
	if _, err := f.WriteString("{\"event\":\"child_log\",\"command\":\"dtls\",\"text\":\"READY role=client version=DTLSv1.3\"}\n"); err != nil {
		t.Fatal(err)
	}
	p := &loggingProcess{name: "dtls", log: &logger{file: f}}
	if err := p.WaitReady("READY role=client", time.Second); err != nil {
		t.Fatalf("existing scoped readiness marker should pass: %v", err)
	}
}
