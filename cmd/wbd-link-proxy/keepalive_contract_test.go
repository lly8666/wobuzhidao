package main

import (
	"os"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestDefaultKeepaliveRemainsFifteenSeconds(t *testing.T) {
	if defaultKeepalive != 15*time.Second {
		t.Fatalf("default keepalive=%s want=15s", defaultKeepalive)
	}
}

func TestDefaultKeepaliveEmitsRealPingAtFifteenSeconds(t *testing.T) {
	client := udp4(t)
	dtls := udp4(t)
	defer client.Close()
	defer dtls.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff,
		Scheduler: control.FECSchedulerNone,
		MTU: 1400,
		LaneCount: 1,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, defaultKeepalive, stop)
	}()

	if err := dtls.SetReadDeadline(started.Add(18 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	n, peer, err := dtls.ReadFromUDP(buf)
	if err != nil {
		stop <- os.Interrupt
		t.Fatalf("no default keepalive PING inside 18s margin: %v", err)
	}
	elapsed := time.Since(started)
	if elapsed < 14*time.Second {
		stop <- os.Interrupt
		t.Fatalf("default keepalive PING fired too early after %s", elapsed)
	}
	frame, err := control.UnmarshalLink(buf[:n])
	if err != nil {
		stop <- os.Interrupt
		t.Fatalf("decode default keepalive frame: %v", err)
	}
	ping, ok := frame.(control.Ping)
	if !ok || ping.Nonce == 0 {
		stop <- os.Interrupt
		t.Fatalf("first idle lifecycle frame=%T %+v want non-zero LINK PING", frame, frame)
	}
	pong, err := control.MarshalLink(control.Pong{Nonce: ping.Nonce})
	if err != nil {
		stop <- os.Interrupt
		t.Fatal(err)
	}
	if _, err := dtls.WriteToUDP(pong, peer); err != nil {
		stop <- os.Interrupt
		t.Fatal(err)
	}
	stop <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("client loop shutdown after keepalive receipt: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client loop did not stop after keepalive timing probe")
	}
	t.Logf("WBD_KEEPALIVE_15S_WIRE_PASS elapsed=%s nonce=%d", elapsed.Round(time.Millisecond), ping.Nonce)
}
