package main

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

type testClient struct {
	conn    *net.UDPConn
	startup interface {
		Established() bool
		RetryWire() ([]byte, error)
		HandleWire([]byte) ([]byte, error)
	}
	path *linkdata.Path
}

func testLinkConfig(fixed bool) control.LinkConfig {
	if !fixed {
		return control.LinkConfig{
			FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
			MTU: 1400, LaneCount: 1,
		}
	}
	return control.LinkConfig{
		FECMode: control.FECFixed, Scheduler: control.FECSchedulerTailRS,
		DataShards: 20, ParityShards: 20, FlushMillis: 8,
		MTU: 1400, LaneCount: 1,
	}
}

func newTicket(t *testing.T, dir string, now time.Time) realityfront.Ticket {
	t.Helper()
	var ticket realityfront.Ticket
	if _, err := rand.Read(ticket[:]); err != nil {
		t.Fatal(err)
	}
	if err := realityfront.RecordTicketForAccount(dir, ticket, "solo", now); err != nil {
		t.Fatal(err)
	}
	return ticket
}

func newTestClient(t *testing.T, ticket realityfront.Ticket, cfg control.LinkConfig) *testClient {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	var bind [control.DemoWitnessLen]byte
	copy(bind[:], ticket[:])
	startup, err := control.NewDemoTicketLinkClientSession(control.LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg}, bind)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	path, err := linkdata.New(cfg, maxBlocks)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return &testClient{conn: conn, startup: startup, path: path}
}

func startupClient(t *testing.T, c *testClient, dst *net.UDPAddr) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	for !c.startup.Established() {
		wire, err := c.startup.RetryWire()
		if err != nil {
			t.Fatal(err)
		}
		if len(wire) != 0 {
			if _, err := c.conn.WriteToUDP(wire, dst); err != nil {
				t.Fatal(err)
			}
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, from, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() && time.Now().Before(deadline) {
				continue
			}
			t.Fatalf("startup read: %v", err)
		}
		if !from.IP.Equal(dst.IP) || from.Port != dst.Port {
			continue
		}
		next, err := c.startup.HandleWire(buf[:n])
		if err != nil {
			t.Fatal(err)
		}
		if len(next) != 0 {
			if _, err := c.conn.WriteToUDP(next, dst); err != nil {
				t.Fatal(err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("startup timeout")
		}
	}
}

func isControlWire(packet []byte) bool {
	return len(packet) >= control.HeaderLen &&
		string(packet[:4]) == string(control.Magic[:]) &&
		packet[4] == control.FrameVersion1
}

// exchange tolerates delayed/retried startup control and stale application
// echoes already queued on this client's socket. A cross-session routing bug
// still fails: the expected payload can never arrive at the correct client and
// the exchange times out.
func exchange(t *testing.T, c *testClient, dst *net.UDPAddr, payload []byte) {
	t.Helper()
	wire, err := c.path.Encode(payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wire {
		if _, err := c.conn.WriteToUDP(w, dst); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 65535)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
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
		if isControlWire(buf[:n]) {
			next, err := c.startup.HandleWire(buf[:n])
			if err != nil {
				// Lifecycle replies and duplicate startup frames after Established
				// are not application data and must not poison this assertion.
				continue
			}
			if len(next) != 0 {
				_, _ = c.conn.WriteToUDP(next, dst)
			}
			continue
		}
		packets, err := c.path.Decode(buf[:n])
		if err != nil {
			continue
		}
		for _, p := range packets {
			if string(p) == string(payload) {
				return
			}
		}
	}
	t.Fatalf("echo timeout payload=%q", payload)
}

func lifecycleRoundTrip(t *testing.T, c *testClient, dst *net.UDPAddr, frame any) any {
	t.Helper()
	wire, err := control.MarshalLink(frame)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.conn.WriteToUDP(wire, dst); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	buf := make([]byte, 65535)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, from, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			t.Fatal(err)
		}
		if !from.IP.Equal(dst.IP) || from.Port != dst.Port || !isControlWire(buf[:n]) {
			continue
		}
		got, err := control.UnmarshalLink(buf[:n])
		if err != nil {
			continue
		}
		switch frame.(type) {
		case control.Ping:
			if _, ok := got.(control.Pong); ok {
				return got
			}
		case control.Close:
			if _, ok := got.(control.Close); ok {
				return got
			}
		}
	}
	t.Fatalf("lifecycle reply timeout for %T", frame)
	return nil
}

func waitPlaneLen(t *testing.T, s *server, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.plane.Len() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("live sessions=%d want=%d", s.plane.Len(), want)
}

func startEcho(t *testing.T) (*net.UDPConn, context.CancelFunc) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		buf := make([]byte, 65535)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			_, _ = conn.WriteToUDP(buf[:n], from)
		}
	}()
	return conn, func() { cancel(); _ = conn.Close() }
}

func TestSharedAccountTwoClientLinkMuxOffAndFixed(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		name := "off"
		if fixed {
			name = "fixed20x20"
		}
		t.Run(name, func(t *testing.T) {
			echo, stopEcho := startEcho(t)
			defer stopEcho()
			dir := t.TempDir()
			now := time.Now()
			ta := newTicket(t, dir, now)
			tb := newTicket(t, dir, now)
			if ta == tb {
				t.Fatal("tickets unexpectedly equal")
			}

			s, err := newServer(config{
				listen: "127.0.0.1:0", service: echo.LocalAddr().String(), ticketDir: dir,
				ticketTTL: time.Minute, setupTimeout: 3 * time.Second, maxSessions: 4,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			errCh := make(chan error, 1)
			go func() { errCh <- s.Run(ctx) }()

			cfg := testLinkConfig(fixed)
			a := newTestClient(t, ta, cfg)
			b := newTestClient(t, tb, cfg)
			defer a.conn.Close()
			defer b.conn.Close()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); startupClient(t, a, s.Addr()) }()
			go func() { defer wg.Done(); startupClient(t, b, s.Addr()) }()
			wg.Wait()
			// The final startup response is written before the server installs
			// the corresponding DataPlane path. Client establishment therefore
			// precedes server activation by a few scheduler ticks; wait for the
			// observable server state instead of racing an immediate Len read.
			waitPlaneLen(t, s, 2)
			if _, err := realityfront.TicketAccount(dir, ta); err == nil {
				t.Fatal("ticket A was not consumed")
			}
			if _, err := realityfront.TicketAccount(dir, tb); err == nil {
				t.Fatal("ticket B was not consumed")
			}

			wg.Add(2)
			go func() { defer wg.Done(); exchange(t, a, s.Addr(), []byte("DEVICE-A-UNIQUE")) }()
			go func() { defer wg.Done(); exchange(t, b, s.Addr(), []byte("DEVICE-B-UNIQUE")) }()
			wg.Wait()

			if pong, ok := lifecycleRoundTrip(t, b, s.Addr(), control.Ping{Nonce: 0x1234}).(control.Pong); !ok || pong.Nonce != 0x1234 {
				t.Fatalf("bad PONG: %#v", pong)
			}
			closed, ok := lifecycleRoundTrip(t, a, s.Addr(), control.Close{Reason: control.CloseNormal, Detail: "test client done"}).(control.Close)
			if !ok || closed.Reason != control.CloseNormal {
				t.Fatalf("bad CLOSE echo: %#v", closed)
			}
			waitPlaneLen(t, s, 1)
			exchange(t, b, s.Addr(), []byte("DEVICE-B-STILL-LIVE"))

			cancel()
			select {
			case err := <-errCh:
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
					t.Fatalf("server run: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("server did not stop")
			}
		})
	}
}

func TestIdleLeaseReclaimsSessionSlot(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	dir := t.TempDir()
	s, err := newServer(config{
		listen: "127.0.0.1:0", service: echo.LocalAddr().String(), ticketDir: dir,
		ticketTTL: time.Minute, setupTimeout: time.Second, idleTimeout: 40 * time.Millisecond, maxSessions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	a := newTestClient(t, newTicket(t, dir, time.Now()), testLinkConfig(false))
	defer a.conn.Close()
	startupClient(t, a, s.Addr())
	waitPlaneLen(t, s, 1)
	if pong, ok := lifecycleRoundTrip(t, a, s.Addr(), control.Ping{Nonce: 7}).(control.Pong); !ok || pong.Nonce != 7 {
		t.Fatalf("bad lease PONG: %#v", pong)
	}
	// The PING refreshed the lease; once no further data/control arrives the
	// server must remove both the LiveID and peer slot.
	waitPlaneLen(t, s, 0)

	b := newTestClient(t, newTicket(t, dir, time.Now()), testLinkConfig(false))
	defer b.conn.Close()
	startupClient(t, b, s.Addr())
	waitPlaneLen(t, s, 1)
	exchange(t, b, s.Addr(), []byte("REUSED-IDLE-SLOT"))

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
