package main

import (
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestUnclassifiedPendingApplicationDatagramUsesNonFatalSentinel(t *testing.T) {
	if _, err := classifyServicePayload([]byte("not-a-wbd-application-frame")); !errors.Is(err, errUnclassifiedApplicationDatagram) {
		t.Fatalf("classify error=%v want pending sentinel", err)
	}
}

func TestMembershipProbeStillClassifiesGame(t *testing.T) {
	var sid gamelane.SessionID
	sid[0] = 1
	probe, err := gamelane.MarshalLaneProbe(sid, 1)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := classifyServicePayload(probe)
	if err != nil {
		t.Fatal(err)
	}
	if backend != backendGame {
		t.Fatalf("backend=%q want=%q", backend, backendGame)
	}
}

func TestBackendMetadataRefreshWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := &peerSession{}
	if !ps.backendMetaRefreshDue(now) {
		t.Fatal("metadata with no prior send must be due")
	}
	ps.noteMetaSent(now)
	if ps.backendMetaRefreshDue(now.Add(backendMetaRefreshInterval - time.Millisecond)) {
		t.Fatal("metadata refreshed too early")
	}
	if !ps.backendMetaRefreshDue(now.Add(backendMetaRefreshInterval)) {
		t.Fatal("metadata must refresh at interval boundary")
	}
}
