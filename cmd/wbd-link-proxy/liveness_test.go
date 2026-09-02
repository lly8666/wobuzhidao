package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestClientRemoteRXDeadlineUsesThreeKeepaliveWindows(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	keepalive := 10 * time.Second
	if clientRemoteRXExpired(base, base.Add(3*keepalive-time.Nanosecond), keepalive) {
		t.Fatal("remote RX expired before three keepalive windows")
	}
	if !clientRemoteRXExpired(base, base.Add(3*keepalive), keepalive) {
		t.Fatal("remote RX did not expire at three keepalive windows")
	}
	refreshed := base.Add(2 * keepalive)
	if clientRemoteRXExpired(refreshed, base.Add(4*keepalive), keepalive) {
		t.Fatal("valid remote receive did not refresh liveness deadline")
	}
}

func TestClientDataLoopOutboundTrafficCannotMaskNoRemoteRX(t *testing.T) {
	client := udp4(t)
	dtls := udp4(t)
	app := udp4(t)
	defer client.Close()
	defer dtls.Close()
	defer app.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
		MTU: 1400, LaneCount: 1,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	keepalive := 10 * time.Millisecond
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, keepalive, stop)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, _ = app.WriteToUDP([]byte("outbound-does-not-prove-peer-liveness"), addr(client))
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "liveness timeout") {
				t.Fatalf("client loop error=%v", err)
			}
			return
		case <-time.After(5 * time.Millisecond):
		}
	}
	stop <- os.Interrupt
	t.Fatal("continuous outbound traffic masked remote-RX liveness failure")
}
