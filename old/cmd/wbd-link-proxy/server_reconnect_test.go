package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestClientDataLoopServerTransientCloseRequestsReconnect(t *testing.T) {
	client := udp4(t)
	dtls := udp4(t)
	defer client.Close()
	defer dtls.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
		MTU: 1400, LaneCount: 1,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, 200*time.Millisecond, stop)
	}()

	wire, err := control.MarshalLink(control.Close{
		Reason: control.CloseTransportTransient,
		Detail: "server requests fresh transport",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dtls.WriteToUDP(wire, client.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("server transient CLOSE did not terminate client LINK")
		}
		if !strings.Contains(err.Error(), "server closed WBD link") || !strings.Contains(err.Error(), "reconnect=true") {
			t.Fatalf("server transient CLOSE error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client LINK did not exit on server transient CLOSE")
	}
}

func TestServerReconnectCloseReasonsRemainWireCompatible(t *testing.T) {
	for _, reason := range []control.CloseReason{control.CloseIdleTimeout, control.CloseTransportTransient} {
		wire, err := control.MarshalLink(control.Close{Reason: reason, Detail: "reconnect"})
		if err != nil {
			t.Fatalf("marshal reason=%d: %v", reason, err)
		}
		frame, err := control.UnmarshalLink(wire)
		if err != nil {
			t.Fatalf("unmarshal reason=%d: %v", reason, err)
		}
		closeFrame, ok := frame.(control.Close)
		if !ok || closeFrame.Reason != reason || !control.ReconnectAllowed(closeFrame.Reason) {
			t.Fatalf("reason=%d frame=%#v reconnect=%v", reason, frame, ok && control.ReconnectAllowed(closeFrame.Reason))
		}
	}
}
