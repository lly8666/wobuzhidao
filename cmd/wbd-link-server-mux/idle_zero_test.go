package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestIdleTimeoutZeroDisablesSessionExpiry(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	dir := t.TempDir()

	s, err := newServer(config{
		listen:         "127.0.0.1:0",
		service:        echo.LocalAddr().String(),
		rawIPService:   echo.LocalAddr().String(),
		ticketDir:      dir,
		ticketTTL:      time.Minute,
		setupTimeout:   time.Second,
		idleTimeout:    0,
		maxSessions:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	ticket, source := newTicket(t, dir, time.Now())
	client := newTestClient(t, ticket, testLinkConfig(false))
	defer client.conn.Close()
	startupClient(t, client, s.Addr())
	waitPlaneLen(t, s, 1)

	// idle_timeout=0 is an explicit policy: idle lease expiry is disabled.
	// Advancing the expiry clock by a day must not reclaim the active session.
	s.expirePeers(time.Now().Add(24 * time.Hour))
	if got := s.plane.Len(); got != 1 {
		t.Fatalf("idle_timeout=0 expired session: live=%d want=1", got)
	}
	exchange(t, client, s.Addr(), rawIPFrame(t, source, "IDLE-ZERO-STILL-LIVE"))

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

func TestNegativeIdleTimeoutRejected(t *testing.T) {
	if _, err := newServer(config{idleTimeout: -time.Second}); err == nil {
		t.Fatal("negative idle timeout unexpectedly accepted")
	}
}
