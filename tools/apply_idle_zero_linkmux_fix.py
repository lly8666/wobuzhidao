from pathlib import Path

p = Path('cmd/wbd-link-server-mux/main.go')
s = p.read_text(encoding='utf-8')
old = '''func newServer(c config) (*server, error) {
\tif c.idleTimeout <= 0 {
\t\tc.idleTimeout = defaultIdleTimeout
\t}
'''
new = '''func newServer(c config) (*server, error) {
\tif c.idleTimeout < 0 {
\t\treturn nil, errors.New("-idle-timeout must be zero (disabled) or a positive duration")
\t}
'''
if old in s:
    s = s.replace(old, new, 1)
elif new not in s:
    raise SystemExit('newServer idle-timeout normalization contract not found')

old = '''\tps := &peerSession{peer: cloneUDPAddr(peer), key: key, created: now, lastActivity: now}
\tverify := func(bind [control.DemoWitnessLen]byte) error { return s.consumeLogicalTunnelTicket(ps, bind) }
\tstartup, err := control.NewDemoTicketReliableLinkServerSession(1, 1, s.linkPolicy, verify)
'''
new = '''\tps := &peerSession{peer: cloneUDPAddr(peer), key: key, created: now, lastActivity: now}
\tverify := func(bind [control.DemoWitnessLen]byte) error {
\t\terr := s.consumeLogicalTunnelTicket(ps, bind)
\t\tif err != nil {
\t\t\tfmt.Fprintf(os.Stderr, "WBD_LINK_MUX_BIND_REJECT peer=%s err=%v\\n", ps.peer, err)
\t\t}
\t\treturn err
\t}
\tstartup, err := control.NewDemoTicketReliableLinkServerSession(1, 1, s.linkPolicy, verify)
'''
if old in s:
    s = s.replace(old, new, 1)
elif new not in s:
    raise SystemExit('newPeer verifier contract not found')

old = '''\t\tif ps.idleFor(now) < s.cfg.idleTimeout {
\t\t\tcontinue
\t\t}
'''
new = '''\t\tif s.cfg.idleTimeout <= 0 || ps.idleFor(now) < s.cfg.idleTimeout {
\t\t\tcontinue
\t\t}
'''
if old in s:
    s = s.replace(old, new, 1)
elif new not in s:
    raise SystemExit('expirePeers idle contract not found')

p.write_text(s, encoding='utf-8')

Path('cmd/wbd-link-server-mux/idle_timeout_disabled_test.go').write_text(r'''package main

import (
    "context"
    "net"
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

    ticket, source := newTicket(t, dir, time.Now())
    c := newTestClient(t, ticket, testLinkConfig(false))
    defer c.conn.Close()
    startupClient(t, c, s.Addr())
    waitPlaneLen(t, s, 1)

    // Advance the lease clock far beyond every normal idle timeout. A zero
    // timeout is an explicit policy switch: idle expiry must remain disabled.
    s.expirePeers(time.Now().Add(24 * time.Hour))
    waitPlaneLen(t, s, 1)
    exchange(t, c, s.Addr(), rawIPFrame(t, source, "IDLE-ZERO-STILL-LIVE"))
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

var _ = net.IPv4len
''', encoding='utf-8')
