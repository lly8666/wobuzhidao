//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimestampedLogPathLivesUnderPortableLogsDirectory(t *testing.T) {
	portable := t.TempDir()
	path, err := timestampedLogPath(portable, "self-test-", ".jsonl")
	if err != nil { t.Fatal(err) }
	wantDir := filepath.Join(portable, "logs")
	if filepath.Dir(path) != wantDir { t.Fatalf("log dir=%q want=%q", filepath.Dir(path), wantDir) }
	if !strings.HasPrefix(filepath.Base(path), "self-test-") || !strings.HasSuffix(path, ".jsonl") {
		t.Fatalf("unexpected log path %q", path)
	}
	if info, err := os.Stat(wantDir); err != nil || !info.IsDir() {
		t.Fatalf("logs directory not created: info=%v err=%v", info, err)
	}
}
