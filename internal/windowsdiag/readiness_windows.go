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
)

type readinessLogRow struct {
	Event   string `json:"event"`
	Command string `json:"command"`
	Text    string `json:"text"`
}

// WaitReady observes the same redacted JSONL stream used for support logs.
// Matching both command name and marker prevents the DTLS READY marker from
// being satisfied by FakeTCP's earlier "READY role=client" line.
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
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout waiting for marker %q", marker)
		}
		time.Sleep(20 * time.Millisecond)
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
