//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRuntimeLogWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "runtime.log")
	f, err := openRuntimeLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("runtime-log-test\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "runtime-log-test") {
		t.Fatalf("runtime log=%q", raw)
	}
}
