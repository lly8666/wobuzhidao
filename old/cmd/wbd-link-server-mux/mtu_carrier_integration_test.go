package main

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func linkConfigForInner(t *testing.T, innerMTU int, fixed bool) control.LinkConfig {
	t.Helper()
	linkMTU, err := gamepath.LinkPlaintextMTU(innerMTU)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed {
		return control.LinkConfig{FECMode: control.FECOff, Scheduler: control.FECSchedulerNone, LaneCount: 1, MTU: uint16(linkMTU)}
	}
	return control.LinkConfig{
		FECMode: control.FECFixed, Scheduler: control.FECSchedulerTailRS,
		DataShards: 20, ParityShards: 20, FlushMillis: 8,
		LaneCount: 1, MTU: uint16(linkMTU),
	}
}

func TestCarrierBudgetValidatorMultipleMTUsAndLayers(t *testing.T) {
	for _, connectionMTU := range []int{1280, 1400, 1500, 2000} {
		for _, fixed := range []bool{false, true} {
			budget, err := pathmtu.Derive(connectionMTU, pathmtu.Features{FEC: fixed, Game: true})
			if err != nil {
				t.Fatalf("connection=%d fixed=%v derive: %v", connectionMTU, fixed, err)
			}
			validator, err := linkConfigCarrierValidator(connectionMTU)
			if err != nil {
				t.Fatalf("connection=%d validator: %v", connectionMTU, err)
			}
			linkFromInner, err := gamepath.LinkPlaintextMTU(budget.InnerMTU)
			if err != nil {
				t.Fatal(err)
			}
			if linkFromInner != budget.LinkPlaintextMTU {
				t.Fatalf("connection=%d fixed=%v inner->link=%d budget link=%d", connectionMTU, fixed, linkFromInner, budget.LinkPlaintextMTU)
			}
			if budget.MaxDTLSDatagram() > budget.CarrierPayloadMTU || budget.MaxFECWireDatagram() > budget.DTLSPlaintextMTU || budget.MaxGameDatagram() > budget.LinkPlaintextMTU {
				t.Fatalf("connection=%d fixed=%v layer budget overflow: %+v", connectionMTU, fixed, budget)
			}
			for _, delta := range []int{-1, 0, 1} {
				inner := budget.InnerMTU + delta
				cfg := linkConfigForInner(t, inner, fixed)
				err := validator(cfg)
				if delta <= 0 && err != nil {
					t.Fatalf("connection=%d fixed=%v inner=%d delta=%d rejected: %v", connectionMTU, fixed, inner, delta, err)
				}
				if delta > 0 && err == nil {
					t.Fatalf("connection=%d fixed=%v inner=%d above carrier ceiling accepted", connectionMTU, fixed, inner)
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

func TestCarrierPolicyRejectsBeforeLinkAccept(t *testing.T) {
	old := serverConnectionMTU
	serverConnectionMTU = 1500
	defer func() { serverConnectionMTU = old }()
	policy, err := linkPolicyForInnerMTU(defaultInnerMTU)
	if err != nil {
		t.Fatal(err)
	}
	server, err := control.NewLinkServerSession(1, 1, nil, policy)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := control.MarshalLink(control.LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: linkConfigForInner(t, 1333, true)})
	if err != nil {
		t.Fatal(err)
	}
	replyWire, err := server.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := control.UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reply.(control.LinkAccept); ok {
		t.Fatal("over-ceiling LINK_INIT received LINK_ACCEPT")
	}
	if e, ok := reply.(control.Error); !ok || e.Code != control.ErrorPolicy {
		t.Fatalf("reply=%T %#v want ErrorPolicy", reply, reply)
	}
	if server.Stats().Configured || server.State() != control.StateFailed {
		t.Fatalf("rejected association configured=%v state=%v", server.Stats().Configured, server.State())
	}
}

func TestConcurrentClientsDifferentMTUsAndActionsStayIndependent(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	dir := t.TempDir()
	now := time.Now()
	inners := []int{576, 900, 1200, 1332}
	clients := make([]*testClient, len(inners))
	tickets := make([]realityfront.Ticket, len(inners))
	addrs := make([]netip.Addr, len(inners))
	for i := range inners {
		tickets[i], addrs[i] = newTicket(t, dir, now)
	}
	badTicket, _ := newTicket(t, dir, now)

	s, err := newServer(config{
		listen: "127.0.0.1:0", service: echo.LocalAddr().String(), rawIPService: echo.LocalAddr().String(), ticketDir: dir,
		ticketTTL: time.Minute, setupTimeout: 3 * time.Second, maxSessions: len(inners) + 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	for i := range inners {
		clients[i] = newTestClient(t, tickets[i], linkConfigForInner(t, inners[i], true))
		defer clients[i].conn.Close()
	}
	bad := newTestClient(t, badTicket, linkConfigForInner(t, 1333, true))
	defer bad.conn.Close()

	var wg sync.WaitGroup
	wg.Add(len(clients))
	for i := range clients {
		i := i
		go func() { defer wg.Done(); startupClient(t, clients[i], s.Addr()) }()
	}
	wg.Wait()
	waitPlaneLen(t, s, len(clients))
	startupClientExpectRejected(t, bad, s.Addr())
	waitPlaneLen(t, s, len(clients))

	for i := range clients {
		accepted, ok := clients[i].startup.(interface{ Accept() (control.LinkAccept, bool) }).Accept()
		if !ok {
			t.Fatalf("client %d missing accepted config", i)
		}
		want := linkConfigForInner(t, inners[i], true).MTU
		if accepted.Config.MTU != want {
			t.Fatalf("client %d accepted MTU=%d want=%d", i, accepted.Config.MTU, want)
		}
	}

	wg.Add(4)
	go func() { defer wg.Done(); exchange(t, clients[0], s.Addr(), rawIPFrame(t, addrs[0], "MTU-576")) }()
	go func() {
		defer wg.Done()
		if p, ok := lifecycleRoundTrip(t, clients[1], s.Addr(), control.Ping{Nonce: 0x900}).(control.Pong); !ok || p.Nonce != 0x900 {
			t.Errorf("client 1 ping=%#v", p)
		}
	}()
	go func() { defer wg.Done(); exchange(t, clients[2], s.Addr(), rawIPFrame(t, addrs[2], "MTU-1200")) }()
	go func() {
		defer wg.Done()
		if p, ok := lifecycleRoundTrip(t, clients[3], s.Addr(), control.Ping{Nonce: 0x1332}).(control.Pong); !ok || p.Nonce != 0x1332 {
			t.Errorf("client 3 ping=%#v", p)
		}
	}()
	wg.Wait()

	wg.Add(4)
	go func() { defer wg.Done(); _ = lifecycleRoundTrip(t, clients[0], s.Addr(), control.Close{Reason: control.CloseNormal, Detail: "close-0"}) }()
	go func() { defer wg.Done(); exchange(t, clients[1], s.Addr(), rawIPFrame(t, addrs[1], "MTU-900-LIVE")) }()
	go func() { defer wg.Done(); _ = lifecycleRoundTrip(t, clients[2], s.Addr(), control.Close{Reason: control.CloseNormal, Detail: "close-2"}) }()
	go func() {
		defer wg.Done()
		if p, ok := lifecycleRoundTrip(t, clients[3], s.Addr(), control.Ping{Nonce: 0xfeed}).(control.Pong); !ok || p.Nonce != 0xfeed {
			t.Errorf("client 3 second ping=%#v", p)
		}
	}()
	wg.Wait()
	waitPlaneLen(t, s, 2)
	exchange(t, clients[3], s.Addr(), rawIPFrame(t, addrs[3], "MTU-1332-AFTER-CLOSE"))

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
