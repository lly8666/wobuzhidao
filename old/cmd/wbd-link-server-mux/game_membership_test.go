package main

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestMembershipProbeClassifiesAsGameBackend(t *testing.T) {
	var sid gamelane.SessionID
	sid[0] = 1
	wire, err := gamelane.MarshalLaneProbe(sid, 2)
	if err != nil { t.Fatal(err) }
	backend, err := classifyServicePayload(wire)
	if err != nil { t.Fatal(err) }
	if backend != backendGame { t.Fatalf("backend=%q want=%q", backend, backendGame) }
}
