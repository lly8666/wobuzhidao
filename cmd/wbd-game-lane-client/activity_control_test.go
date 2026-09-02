package main

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestDormantPayloadStillAdvancesActivity(t *testing.T) {
	app := loopbackUDP(t)
	c := &client{app: app, lanes: make(map[uint8]*laneConn), overlap: make(map[uint8]*laneConn)}
	errCh := make(chan error, 1)
	go func() { errCh <- c.appLoop() }()

	peer, err := net.DialUDP("udp4", nil, app.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if _, err := peer.Write([]byte("wake-me")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for atomic.LoadUint64(&c.dormantDrop) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	activity := c.activity.snapshot()
	if activity.Sequence != 1 || activity.LastPayloadActivityUnixNano <= 0 {
		t.Fatalf("activity=%+v", activity)
	}
	if got := atomic.LoadUint64(&c.logicalTX); got != 0 {
		t.Fatalf("dormant payload unexpectedly transmitted logical_tx=%d", got)
	}

	_ = app.Close()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("appLoop did not stop")
	}
}

func TestActivityControlReportsOnlyMarkedPayload(t *testing.T) {
	c := &client{}
	if reply, ok := c.handleControlRequest([]byte(`{"op":"activity"}`)).(gamelane.LaneActivityReply); !ok || !reply.OK || reply.Activity.Sequence != 0 || reply.Activity.LastPayloadActivityUnixNano != 0 {
		t.Fatalf("initial reply=%+v ok=%v", reply, ok)
	}
	when := time.Unix(1700000000, 123)
	c.activity.mark(when)
	reply, ok := c.handleControlRequest([]byte(`{"op":"activity"}`)).(gamelane.LaneActivityReply)
	if !ok || !reply.OK {
		t.Fatalf("reply=%+v ok=%v", reply, ok)
	}
	if reply.Activity.Sequence != 1 || reply.Activity.LastPayloadActivityUnixNano != when.UnixNano() {
		t.Fatalf("activity=%+v", reply.Activity)
	}
}
