//go:build windows

package windowsdiag

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

type readinessLogRow struct {
	Event   string `json:"event"`
	Command string `json:"command"`
	Text    string `json:"text"`
}

// WaitReady observes the same redacted JSONL stream used for support logs.
// Matching both command name and marker prevents the DTLS READY marker from
// being satisfied by FakeTCP's earlier "READY role=client" line. Unlike the
// original diagnostics-only implementation, this also observes the child
// process itself so an early exit is returned immediately instead of being
// hidden behind the full marker timeout.
func (p *loggingProcess) WaitReady(marker string, timeout time.Duration) error {
	if p == nil || p.log == nil || p.log.file == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("logging process has no readiness stream or child process")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		seen, err := readinessMarkerSeen(p.log.file.Name(), p.name, marker)
		if err != nil {
			return err
		}
		if seen {
			return nil
		}
		exited, code, err := loggingProcessExitState(p)
		if err != nil {
			return fmt.Errorf("inspect child while waiting for marker %q: %w", marker, err)
		}
		if exited {
			// Reap the child here. loggingProcess.Stop is intentionally idempotent
			// when cmd is nil, so cleanup will not issue TerminateProcess against an
			// already-dead Windows process and create a misleading Access Denied.
			waitErr := p.cmd.Wait()
			if p.stdout != nil { p.stdout.Flush() }
			if p.stderr != nil { p.stderr.Flush() }
			p.cmd = nil
			if waitErr != nil {
				return fmt.Errorf("process exited before readiness marker %q exit_code=%d: %w", marker, code, waitErr)
			}
			return fmt.Errorf("process exited before readiness marker %q exit_code=%d", marker, code)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout waiting for marker %q", marker)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// loggingProcessExitState asks Windows for a fresh handle to the child and uses
// the process wait state rather than relying on exec.Cmd.ProcessState, which is
// populated only after Cmd.Wait. The diagnostics runner intentionally does not
// call Wait until Stop, so ProcessState alone cannot detect an early child exit.
func loggingProcessExitState(p *loggingProcess) (bool, uint32, error) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil || p.cmd.Process.Pid <= 0 {
		return false, 0, errors.New("child process is unavailable")
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.cmd.Process.Pid))
	if err != nil {
		return false, 0, err
	}
	defer windows.CloseHandle(h)
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false, 0, err
	}
	if status == uint32(windows.WAIT_TIMEOUT) {
		return false, 0, nil
	}
	if status != uint32(windows.WAIT_OBJECT_0) {
		return false, 0, fmt.Errorf("unexpected WaitForSingleObject status %#x", status)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true, 0, err
	}
	return true, code, nil
}

func readinessMarkerSeen(path, command, marker string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 1<<20)
	for s.Scan() {
		var row readinessLogRow
		if err := json.Unmarshal(s.Bytes(), &row); err != nil {
			continue
		}
		if row.Event == "child_log" && row.Command == command && strings.Contains(row.Text, marker) {
			return true, nil
		}
	}
	if err := s.Err(); err != nil {
		return false, err
	}
	return false, nil
}
