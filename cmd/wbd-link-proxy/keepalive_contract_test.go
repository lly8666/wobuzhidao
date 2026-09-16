package main

import (
	"testing"
	"time"
)

func TestDefaultKeepaliveRemainsFifteenSeconds(t *testing.T) {
	if defaultKeepalive != 15*time.Second {
		t.Fatalf("default keepalive=%s want=15s", defaultKeepalive)
	}
}
