package windowsruntime

import (
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func payloadIdleControlResponder(t *testing.T) (string, func(), <-chan gamelane.LaneSetCommand) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var sequence atomic.Uint64
	var lastUnixNano atomic.Int64
	sets := make(chan gamelane.LaneSetCommand, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			op, err := gamelane.ParseLaneControlOp(buf[:n])
			if err != nil {
				continue
			}
			switch op {
			case gamelane.LaneControlActivity:
				seq := sequence.Load()
				last := int64(0)
				if seq > 0 {
					last = lastUnixNano.Load()
				}
				wire, _ := json.Marshal(gamelane.LaneActivityReply{OK: true, Activity: gamelane.PayloadActivity{Sequence: seq, LastPayloadActivityUnixNano: last}})
				_, _ = conn.WriteToUDP(wire, peer)
			case gamelane.LaneControlSet:
				cmd, err := gamelane.ParseLaneSetCommand(buf[:n])
				if err != nil {
					continue
				}
				sets <- cmd
				wire, _ := json.Marshal(gamelane.LaneControlReply{OK: true, Active: uniqueGameLaneIDsFromTargets(cmd.Lanes)})
				_, _ = conn.WriteToUDP(wire, peer)
			}
		}
	}()

	markPayload := func() {
		lastUnixNano.Store(time.Now().UnixNano())
		sequence.Add(1)
	}
	return conn.LocalAddr().String(), markPayload, sets
}

func waitControllerState(t *testing.T, c *Controller, want RuntimeState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.State() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("runtime state=%s want=%s", c.State(), want)
}

func TestPayloadIdleMonitorDormantsWithoutPayloadAndWakesOnSequenceAdvance(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	control, markPayload, sets := payloadIdleControlResponder(t)
	setControllerGameControl(c, control)
	c.startPayloadIdleMonitor(50 * time.Millisecond)

	waitControllerState(t, c, RuntimeDormant, 2*time.Second)
	select {
	case cmd := <-sets:
		if len(cmd.Lanes) != 0 {
			t.Fatalf("idle DORMANT Game barrier lanes=%v", cmd.Lanes)
		}
	case <-time.After(time.Second):
		t.Fatal("idle DORMANT did not publish empty Game target set")
	}

	// Activity/control queries above did not refresh idle. Only a real payload
	// sequence advance is allowed to wake the existing Logical Tunnel.
	markPayload()
	waitControllerState(t, c, RuntimeConnected, 2*time.Second)
	select {
	case cmd := <-sets:
		if len(cmd.Lanes) == 0 || cmd.Lanes[0].ID != 1 {
			t.Fatalf("wake first READY Game lanes=%v", cmd.Lanes)
		}
	case <-time.After(time.Second):
		t.Fatal("payload wake did not publish first READY lane")
	}

	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectStartsPayloadIdleMonitorOnlyWhenConfigured(t *testing.T) {
	for _, tc := range []struct {
		name string
		seconds int
		wantMonitor bool
	}{
		{name: "disabled-default", seconds: 0, wantMonitor: false},
		{name: "enabled", seconds: 1, wantMonitor: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &recordingRunner{}
			c := testController(r)
			p := testProfile()
			p.TunnelIPv4 = ""
			p.IdleTimeoutSeconds = tc.seconds
			if err := c.Connect(p); err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			running := c.idleStop != nil
			c.mu.Unlock()
			if running != tc.wantMonitor {
				t.Fatalf("idle monitor running=%v want=%v", running, tc.wantMonitor)
			}
			if err := c.Disconnect(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfileRejectsNegativeIdleTimeout(t *testing.T) {
	p := testProfile()
	p.IdleTimeoutSeconds = -1
	if err := p.Validate(); err == nil {
		t.Fatal("negative idle timeout unexpectedly accepted")
	}
}

func TestPayloadIdleObservationUsesSequenceAsAuthority(t *testing.T) {
	base := time.Unix(1700000000, 0)
	o := newPayloadIdleObservation(base)
	if changed, advanced := o.observe(gamelane.PayloadActivity{Sequence: 7, LastPayloadActivityUnixNano: 1}, base.Add(time.Second)); !changed || advanced {
		t.Fatalf("initial observation changed=%v advanced=%v", changed, advanced)
	}
	if changed, _ := o.observe(gamelane.PayloadActivity{Sequence: 7, LastPayloadActivityUnixNano: 999999}, base.Add(2*time.Second)); changed {
		t.Fatal("child timestamp changed idle policy without a payload sequence advance")
	}
	if changed, advanced := o.observe(gamelane.PayloadActivity{Sequence: 8, LastPayloadActivityUnixNano: 2}, base.Add(3*time.Second)); !changed || !advanced {
		t.Fatalf("payload sequence advance changed=%v advanced=%v", changed, advanced)
	}
}
