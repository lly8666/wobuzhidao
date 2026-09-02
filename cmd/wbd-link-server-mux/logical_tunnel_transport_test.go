package main

import (
	"errors"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func testTunnelBinding() realityfront.TicketBinding {
	return realityfront.TicketBinding{
		Account: "test-account",
		InstallationID: logicaltunnel.InstallationID("00112233445566778899aabbccddeeff"),
		Config: logicaltunnel.TunnelConfig{
			TunnelID: logicaltunnel.TunnelID("11223344556677889900aabbccddeeff"),
			Address4: "10.66.0.1/32",
			Routes4: []string{"0.0.0.0/0"},
		},
	}
}

func resetTunnelTransportTestState() {
	peerTunnelBindings.Range(func(key, _ any) bool { peerTunnelBindings.Delete(key); return true })
	activeTunnelPeersMu.Lock()
	activeTunnelPeers = make(map[string]map[*peerSession]struct{})
	activeTunnelPeersMu.Unlock()
}

func TestClaimTunnelTransportAcceptsFourAndRejectsFifthConcurrentPeer(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	if logicaltunnel.MaxProductPublicTransportLanes != 4 { t.Fatalf("product transport max=%d want=4", logicaltunnel.MaxProductPublicTransportLanes) }
	binding := testTunnelBinding()
	peers := make([]*peerSession, 0, 4)
	for i := 1; i <= 4; i++ {
		peer := &peerSession{key: "lane-" + string(rune('0'+i))}
		if err := claimTunnelTransport(peer, binding); err != nil { t.Fatalf("claim lane %d: %v", i, err) }
		peers = append(peers, peer)
		if got := activeTunnelTransportCount(binding.Config.TunnelID); got != i { t.Fatalf("after lane %d active=%d", i, got) }
	}
	fifth := &peerSession{key: "lane-5"}
	if err := claimTunnelTransport(fifth, binding); !errors.Is(err, errTransportLaneLimit) { t.Fatalf("fifth concurrent public transport was not rejected: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 4 { t.Fatalf("rejected fifth claim changed active=%d", got) }
	_ = peers
}

func TestClaimTunnelTransportIsIdempotentForSamePeer(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding(); peer := &peerSession{key: "same"}
	if err := claimTunnelTransport(peer, binding); err != nil { t.Fatal(err) }
	if err := claimTunnelTransport(peer, binding); err != nil { t.Fatalf("same peer repeat claim failed: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("idempotent claim active=%d want=1", got) }
}

func TestReleaseTunnelTransportAllowsMakeBeforeBreakReplacement(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding()
	old := &peerSession{key: "old"}
	if err := claimTunnelTransport(old, binding); err != nil { t.Fatal(err) }
	peerTunnelBindings.Store(old, binding)

	candidate := &peerSession{key: "replacement"}
	if err := claimTunnelTransport(candidate, binding); err != nil { t.Fatalf("overlapping replacement rejected: %v", err) }
	peerTunnelBindings.Store(candidate, binding)
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 2 { t.Fatalf("make-before-break overlap active=%d want=2", got) }

	forgetPeerTunnel(old)
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("after old transport teardown active=%d want=1", got) }
	bound, ok := peerTunnelBinding(candidate); if !ok { t.Fatal("replacement lost binding") }
	if bound.Config.TunnelID != binding.Config.TunnelID || bound.Config.Address4 != binding.Config.Address4 { t.Fatal("replacement changed tunnel identity/lease") }
}

func TestRejectedFifthTransportLeavesExistingTransportsClaimed(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding()
	peers := make([]*peerSession, 0, 4)
	for i := 1; i <= 4; i++ {
		peer := &peerSession{key: "existing-" + string(rune('0'+i))}
		if err := claimTunnelTransport(peer, binding); err != nil { t.Fatal(err) }
		peerTunnelBindings.Store(peer, binding)
		peers = append(peers, peer)
	}
	fifth := &peerSession{key:"fifth"}
	if err := claimTunnelTransport(fifth, binding); !errors.Is(err, errTransportLaneLimit) { t.Fatalf("fifth transport rejection=%v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 4 { t.Fatalf("fifth transport rejection disturbed active transports: active=%d", got) }
	for i, peer := range peers {
		if _, ok := peerTunnelBinding(peer); !ok { t.Fatalf("fifth transport rejection removed existing binding %d", i+1) }
	}
}
