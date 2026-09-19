package main

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestClientKeepaliveTrackerRemoteActivityCancelsProbe(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	keepalive := 30 * time.Second
	tracker := newClientKeepaliveTracker(base, keepalive)

	send, nonce, dead := tracker.poll(base.Add(keepalive), keepalive)
	if !send || dead || nonce == 0 {
		t.Fatalf("first idle probe send=%t nonce=%d dead=%t", send, nonce, dead)
	}

	activityAt := base.Add(keepalive + time.Second)
	tracker.observeRemoteActivity(activityAt, keepalive)
	if send, _, dead := tracker.poll(activityAt.Add(keepalive-time.Nanosecond), keepalive); send || dead {
		t.Fatalf("remote activity did not postpone idle probe send=%t dead=%t", send, dead)
	}
	if send, nonce, dead := tracker.poll(activityAt.Add(keepalive), keepalive); !send || dead || nonce == 0 {
		t.Fatalf("idle probe after refreshed interval send=%t nonce=%d dead=%t", send, nonce, dead)
	}
}

func TestClientDataLoopContinuousRemoteDataSuppressesKeepaliveAndStaysLive(t *testing.T) {
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
	keepalive := 20 * time.Millisecond
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, keepalive, stop)
	}()

	peer := addr(client)
	payload := []byte("authenticated-remote-business-data")
	end := time.Now().Add(5 * keepalive)
	for time.Now().Before(end) {
		if _, err := dtls.WriteToUDP(payload, peer); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			t.Fatalf("active remote traffic was retired: %v", err)
		case <-time.After(3 * time.Millisecond):
		}
	}

	// Traffic ran well beyond the historical 3*keepalive retirement window.
	// There should be no queued PING because every successfully decoded remote
	// datagram restarts the idle interval.
	buf := make([]byte, 65535)
	pingCount := 0
	drainUntil := time.Now().Add(5 * time.Millisecond)
	for time.Now().Before(drainUntil) {
		_ = dtls.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, _, err := dtls.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			t.Fatal(err)
		}
		frame, err := control.UnmarshalLink(buf[:n])
		if err != nil {
			continue
		}
		if _, ok := frame.(control.Ping); ok {
			pingCount++
		}
	}
	if pingCount != 0 {
		t.Fatalf("active remote traffic emitted %d keepalive PINGs", pingCount)
	}

	stop <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("client loop shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client loop did not stop")
	}
}
