//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var runtimeLogFile *os.File

func init() {
	path := strings.TrimSpace(os.Getenv("WBD_RUNTIME_LOG"))
	if path == "" {
		return
	}
	f, err := openRuntimeLog(path)
	if err != nil {
		return
	}
	runtimeLogFile = f
	os.Stdout = f
	os.Stderr = f
	fmt.Fprintf(f, "WBD_RUNTIME_LOG_READY time=%s path=%s\n", time.Now().Format(time.RFC3339Nano), path)
}

func openRuntimeLog(path string) (*os.File, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("runtime log path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}
