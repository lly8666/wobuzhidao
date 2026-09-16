package main

import (
	"context"
	"testing"
	"time"
)

func TestIdleTimeoutZeroDisablesSessionExpiry(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	dir := t.TempDir()
	s, err := newServer(config{
		listen: "127.0.0.1:0", service: echo.LocalAddr().String(), rawIPService: echo.LocalAddr().String(), ticketDir: dir,
		ticketTTL: time.Minute, setupTimeout: time.Second, idleTimeout: 0, maxSessions: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.cfg.idleTimeout != 0 {
		t.Fatalf("idle timeout = %v, want disabled (0)", s.cfg.idleTimeout)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Run(ctx) }()

	ticket, _ := newTicket(t, dir, time.Now())
	c := newTestClient(t, ticket, testLinkConfig(false))
	defer c.conn.Close()
	startupClient(t, c, s.Addr())
	waitPlaneLen(t, s, 1)

	// Advance the lease clock far beyond every normal idle timeout. A zero
	// timeout is an explicit policy switch: expiry sweeps must remain no-ops.
	// Repeat the sweep to make the contract independent of backend scheduling
	// and prove that disabled expiry does not accumulate elapsed-time state.
	now := time.Now()
	s.expirePeers(now.Add(24 * time.Hour))
	waitPlaneLen(t, s, 1)
	s.expirePeers(now.Add(48 * time.Hour))
	waitPlaneLen(t, s, 1)
}

func TestNegativeIdleTimeoutRejected(t *testing.T) {
	echo, stopEcho := startEcho(t)
	defer stopEcho()
	_, err := newServer(config{
		listen: "127.0.0.1:0", service: echo.LocalAddr().String(), ticketDir: t.TempDir(),
		ticketTTL: time.Minute, setupTimeout: time.Second, idleTimeout: -time.Second, maxSessions: 1,
	})
	if err == nil {
		t.Fatal("negative idle timeout unexpectedly accepted")
	}
}
