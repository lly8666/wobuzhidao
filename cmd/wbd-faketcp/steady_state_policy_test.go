package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestEndpointRecoveryFlagSelectsSenderOnly(t *testing.T) {
	cases := []struct {
		name string
		want faketcp.RecoveryMode
	}{
		{name: "legacy", want: faketcp.RecoveryLegacy},
		{name: "sack-rack", want: faketcp.RecoverySACKRACK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &endpoint{cfg: config{recovery: tc.name}}
			s := e.newSender(100, time.Second)
			if got := s.RecoveryMode(); got != tc.want {
				t.Fatalf("sender recovery=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestEndpointSteadyStateTransitionCannotForkReceiverPolicyByRecoveryFlag(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(src, []byte("e.receiver.EnableSteadyStateDelivery()")) {
		t.Fatal("ordinary endpoint no longer enters the shared receiver steady-state policy")
	}
	if bytes.Contains(src, []byte("SetSteadyStateRecoveryMode")) {
		t.Fatal("ordinary endpoint reintroduced sender-mode-dependent receiver forgiveness")
	}
}
