//go:build windows

package windowsdiag

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

type readinessLogRow struct {
	Event   string `json:"event"`
	Command string `json:"command"`
	Text    string `json:"text"`
}

// WaitReady observes the same redacted JSONL stream used for support logs.
// Matching both command name and marker prevents the DTLS READY marker from
// being satisfied by FakeTCP's earlier "READY role=client" line.
//
// The diagnostic runner intentionally remains small, but it must not hide the
// first failure behind a later readiness timeout. Npcap/DTLS/LINK children can
// exit before emitting their marker; poll the child handle in parallel with the
// log and reap an exited child exactly once. Setting Process=nil makes the
// controller's subsequent cleanup idempotent instead of attempting a second
// TerminateProcess on an already-dead Windows process.
func (p *loggingProcess) WaitReady(marker string, timeout time.Duration) error {
	if p == nil || p.log == nil || p.log.file == nil {
		return errors.New("logging process has no readiness stream")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" { return nil }
	if timeout <= 0 { timeout = 25 * time.Second }
	deadline := time.Now().Add(timeout)
	for {
		seen, err := readinessMarkerSeen(p.log.file.Name(), p.name, marker)
		if err != nil { return err }
		if seen { return nil }
		exited, err := loggingProcessExited(p)
		if err != nil { return err }
		if exited {
			waitErr := p.cmd.Wait()
			if p.stdout != nil { p.stdout.Flush() }
			if p.stderr != nil { p.stderr.Flush() }
			// cmd.Wait has consumed the child handle. Stop must now be a no-op.
			p.cmd.Process = nil
			if waitErr != nil {
				return fmt.Errorf("%s exited before readiness marker %q: %w", p.name, marker, waitErr)
			}
			return fmt.Errorf("%s exited before readiness marker %q", p.name, marker)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout waiting for marker %q", marker)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func loggingProcessExited(p *loggingProcess) (bool, error) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return true, nil
	}
	// SYNCHRONIZE is sufficient for WaitForSingleObject and avoids asking for
	// terminate/query privileges. The process was created by WBD, so OpenProcess
	// should normally succeed even after it has transitioned to signaled state.
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(p.cmd.Process.Pid))
	if err != nil {
		// Do not turn an inability to open a live process into a false child-exit
		// diagnosis. The ordinary readiness timeout remains the safe fallback.
		return false, nil
	}
	defer syscall.CloseHandle(h)
	state, err := syscall.WaitForSingleObject(h, 0)
	if err != nil { return false, err }
	switch state {
	case syscall.WAIT_OBJECT_0:
		return true, nil
	case syscall.WAIT_TIMEOUT:
		return false, nil
	default:
		return false, fmt.Errorf("WaitForSingleObject returned %#x", state)
	}
}

func readinessMarkerSeen(path, command, marker string) (bool, error) {
	f, err := os.Open(path)
	if err != nil { return false, err }
	defer f.Close()
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 1<<20)
	for s.Scan() {
		var row readinessLogRow
		if err := json.Unmarshal(s.Bytes(), &row); err != nil { continue }
		if row.Event == "child_log" && row.Command == command && strings.Contains(row.Text, marker) {
			return true, nil
		}
	}
	if err := s.Err(); err != nil { return false, err }
	return false, nil
}
