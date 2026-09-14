package main

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

func linkConfigForInner(t *testing.T, innerMTU int, fixed bool) control.LinkConfig {
	t.Helper()
	linkMTU, err := gamepath.LinkPlaintextMTU(innerMTU)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed {
		return control.LinkConfig{
			FECMode:   control.FECOff,
			Scheduler: control.FECSchedulerNone,
			LaneCount: 1,
			MTU:       uint16(linkMTU),
		}
	}
	return control.LinkConfig{
		FECMode:      control.FECFixed,
		Scheduler:    control.FECSchedulerTailRS,
		DataShards:   20,
		ParityShards: 20,
		FlushMillis:  8,
		LaneCount:    1,
		MTU:          uint16(linkMTU),
	}
}

func TestCarrierBudgetValidatorMultipleMTUsAndLayers(t *testing.T) {
	for _, connectionMTU := range []int{1280, 1400, 1500, 2000} {
		for _, fixed := range []bool{false, true} {
			features := pathmtu.Features{FEC: fixed, Game: true}
			budget, err := pathmtu.Derive(connectionMTU, features)
			if err != nil {
				t.Fatalf("connection=%d fixed=%v derive: %v", connectionMTU, fixed, err)
			}
			validator, err := linkConfigCarrierValidator(connectionMTU)
			if err != nil {
				t.Fatalf("connection=%d validator: %v", connectionMTU, err)
			}

			linkFromInner, err := gamepath.LinkPlaintextMTU(budget.InnerMTU)
			if err != nil {
				t.Fatalf("connection=%d fixed=%v inner=%d: %v", connectionMTU, fixed, budget.InnerMTU, err)
			}
			if linkFromInner != budget.LinkPlaintextMTU {
				t.Fatalf("connection=%d fixed=%v inner->link=%d budget link=%d", connectionMTU, fixed, linkFromInner, budget.LinkPlaintextMTU)
			}
			if budget.MaxDTLSDatagram() > budget.CarrierPayloadMTU {
				t.Fatalf("connection=%d fixed=%v DTLS datagram=%d carrier payload=%d", connectionMTU, fixed, budget.MaxDTLSDatagram(), budget.CarrierPayloadMTU)
			}
			if budget.MaxFECWireDatagram() > budget.DTLSPlaintextMTU {
				t.Fatalf("connection=%d fixed=%v FEC wire=%d DTLS plaintext=%d", connectionMTU, fixed, budget.MaxFECWireDatagram(), budget.DTLSPlaintextMTU)
			}
			if budget.MaxGameDatagram() > budget.LinkPlaintextMTU {
				t.Fatalf("connection=%d fixed=%v Game datagram=%d LINK plaintext=%d", connectionMTU, fixed, budget.MaxGameDatagram(), budget.LinkPlaintextMTU)
			}

			for _, delta := range []int{-1, 0, 1} {
				linkMTU := budget.LinkPlaintextMTU + delta
				cfg := linkConfigForInner(t, linkMTU-gamepath.DatagramOverhead(), fixed)
				err := validator(cfg)
				if delta <= 0 && err != nil {
					t.Fatalf("connection=%d fixed=%v link=%d delta=%d rejected: %v", connectionMTU, fixed, linkMTU, delta, err)
				}
				if delta > 0 && err == nil {
					t.Fatalf("connection=%d fixed=%v link=%d above carrier ceiling accepted", connectionMTU, fixed, linkMTU)
				}
			}
		}
	}
}

func TestCarrierBudgetCanonical1500GameFEC20Boundary(t *testing.T) {
	validator, err := linkConfigCarrierValidator(1500)
	if err != nil {
		t.Fatal(err)
	}
	if err := validator(linkConfigForInner(t, 1332, true)); err != nil {
		t.Fatalf("inner MTU 1332 must fit connection MTU 1500 + Game + FEC20: %v", err)
	}
	if err := validator(linkConfigForInner(t, 1333, true)); err == nil {
		t.Fatal("inner MTU 1333 unexpectedly fits connection MTU 1500 + Game + FEC20")
	}
}

func TestConcurrentClientsDifferentMTUsAndActionsStayIndependent(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	dir := t.TempDir()
	now := time.Now()

	const clientCount = 4
	inners := [clientCount]int{576, 900, 1200, 1332}
	clients := make([]*testClient, clientCount)
	sources := make([]net.IP, 0, clientCount)
	_ = sources

	tickets := make([][control.DemoWitnessLen]byte, clientCount)
	addrs := make([]string, clientCount)
	for i := 0; i < clientCount; i++ {
		ticket, addr := newTicket(t, dir, now)
		tickets[i] = ticket
		addrs[i] = addr.String()
	}
	badTicket, _ := newTicket(t, dir, now)

	s, err := newServer(config{
		listen: "127.0.0.1:0", service: echo.LocalAddr().String(), rawIPService: echo.LocalAddr().String(), ticketDir: dir,
		ticketTTL: time.Minute, setupTimeout: 3 * time.Second, maxSessions: clientCount + 2, connectionMTU: 1500,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	for i := 0; i < clientCount; i++ {
		clients[i] = newTestClient(t, tickets[i], linkConfigForInner(t, inners[i], true))
		defer clients[i].conn.Close()
	}
	bad := newTestClient(t, badTicket, linkConfigForInner(t, 1333, true))
	defer bad.conn.Close()

	var wg sync.WaitGroup
	wg.Add(clientCount + 1)
	for i := 0; i < clientCount; i++ {
		i := i
		go func() {
			defer wg.Done()
			startupClient(t, clients[i], s.Addr())
		}()
	}
	go func() {
		defer wg.Done()
		startupClientExpectRejected(t, bad, s.Addr())
	}()
	wg.Wait()
	waitPlaneLen(t, s, clientCount)

	for i := 0; i < clientCount; i++ {
		accept, ok := clients[i].startup.(interface{ Accept() (control.LinkAccept, bool) }).Accept()
		if !ok {
			t.Fatalf("client %d has no accepted config", i)
		}
		want := linkConfigForInner(t, inners[i], true).MTU
		if accept.Config.MTU != want {
			t.Fatalf("client %d accepted LINK MTU=%d want=%d", i, accept.Config.MTU, want)
		}
	}

	wg.Add(clientCount)
	go func() {
		defer wg.Done()
		exchange(t, clients[0], s.Addr(), rawIPFrame(t, mustAddr(t, addrs[0]), "MTU-576-ACTION"))
	}()
	go func() {
		defer wg.Done()
		if pong, ok := lifecycleRoundTrip(t, clients[1], s.Addr(), control.Ping{Nonce: 0x900}).(control.Pong); !ok || pong.Nonce != 0x900 {
			t.Errorf("client 1 ping: %#v", pong)
		}
	}()
	go func() {
		defer wg.Done()
		exchange(t, clients[2], s.Addr(), rawIPFrame(t, mustAddr(t, addrs[2]), "MTU-1200-ACTION"))
	}()
	go func() {
		defer wg.Done()
		if pong, ok := lifecycleRoundTrip(t, clients[3], s.Addr(), control.Ping{Nonce: 0x1332}).(control.Pong); !ok || pong.Nonce != 0x1332 {
			t.Errorf("client 3 ping: %#v", pong)
		}
	}()
	wg.Wait()

	wg.Add(clientCount)
	go func() {
		defer wg.Done()
		_ = lifecycleRoundTrip(t, clients[0], s.Addr(), control.Close{Reason: control.CloseNormal, Detail: "concurrent close 0"})
	}()
	go func() {
		defer wg.Done()
		exchange(t, clients[1], s.Addr(), rawIPFrame(t, mustAddr(t, addrs[1]), "MTU-900-STILL-LIVE"))
	}()
	go func() {
		defer wg.Done()
		_ = lifecycleRoundTrip(t, clients[2], s.Addr(), control.Close{Reason: control.CloseNormal, Detail: "concurrent close 2"})
	}()
	go func() {
		defer wg.Done()
		if pong, ok := lifecycleRoundTrip(t, clients[3], s.Addr(), control.Ping{Nonce: 0xfeed}).(control.Pong); !ok || pong.Nonce != 0xfeed {
			t.Errorf("client 3 second ping: %#v", pong)
		}
	}()
	wg.Wait()
	waitPlaneLen(t, s, 2)
	exchange(t, clients[3], s.Addr(), rawIPFrame(t, mustAddr(t, addrs[3]), "MTU-1332-AFTER-PEER-CLOSE"))

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("server run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}

func startupClientExpectRejected(t *testing.T, c *testClient, dst *net.UDPAddr) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	for time.Now().Before(deadline) {
		wire, err := c.startup.RetryWire()
		if err != nil {
			return
		}
		if len(wire) != 0 {
			if _, err := c.conn.WriteToUDP(wire, dst); err != nil {
				t.Fatal(err)
			}
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, from, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			t.Fatal(err)
		}
		if !from.IP.Equal(dst.IP) || from.Port != dst.Port {
			continue
		}
		if _, err := c.startup.HandleWire(buf[:n]); err != nil {
			return
		}
		if c.startup.Established() {
			t.Fatal("over-ceiling client established")
		}
	}
	t.Fatal("over-ceiling client did not receive rejection")
}

func mustAddr(t *testing.T, raw string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
