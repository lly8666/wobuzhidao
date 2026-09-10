package main

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

func TestExpireDropsAuthenticatedMetadataThatNeverBindsSession(t *testing.T) {
	idle := 75 * time.Millisecond
	s := &server{
		idle: idle,
		sessions: make(map[gamelane.SessionID]*gameSession),
		peerSession: make(map[string]gamelane.SessionID),
		peerMeta: make(map[string]rawipbackend.TunnelMeta),
	}
	peer := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 32001}
	meta := rawipbackend.TunnelMeta{
		TunnelID: logicaltunnel.TunnelID("00112233445566778899aabbccddeeff"),
		Address4: netip.MustParseAddr("10.66.0.9"),
	}
	t0 := time.Unix(1_700_000_000, 0)
	if err := s.registerPeerMeta(peer, meta, t0); err != nil { t.Fatal(err) }
	if got := len(s.peerMeta); got != 1 { t.Fatalf("peerMeta=%d want=1 after registration", got) }

	// Metadata is allowed to precede the Game Probe/payload, so it must survive
	// inside the normal idle window.
	s.expire(t0.Add(idle - time.Millisecond))
	if got := len(s.peerMeta); got != 1 { t.Fatalf("peerMeta=%d inside idle window want=1", got) }

	// If the authenticated transport disappears before ever binding a Game
	// session, its pre-bind metadata must not be retained forever.
	s.expire(t0.Add(idle + time.Millisecond))
	if got := len(s.peerMeta); got != 0 { t.Fatalf("peerMeta=%d after idle expiry want=0", got) }
}
