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

func TestClaimTunnelTransportAcceptsOneAndRejectsSecondConcurrentPeer(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	if logicaltunnel.MaxProductPublicTransportLanes != 1 { t.Fatalf("product transport max=%d want=1", logicaltunnel.MaxProductPublicTransportLanes) }
	binding := testTunnelBinding()
	first := &peerSession{key: "first"}
	if err := claimTunnelTransport(first, binding); err != nil { t.Fatalf("first public transport claim failed: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("after first claim active=%d want=1", got) }
	second := &peerSession{key: "second"}
	if err := claimTunnelTransport(second, binding); !errors.Is(err, errTransportLaneLimit) { t.Fatalf("second concurrent public transport was not rejected: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("rejected second claim changed active=%d", got) }
}

func TestClaimTunnelTransportIsIdempotentForSamePeer(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding(); peer := &peerSession{key: "same"}
	if err := claimTunnelTransport(peer, binding); err != nil { t.Fatal(err) }
	if err := claimTunnelTransport(peer, binding); err != nil { t.Fatalf("same peer repeat claim failed: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("idempotent claim active=%d want=1", got) }
}

func TestReleaseTunnelTransportAllowsLaterBreakBeforeMakeReplacement(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding()
	old := &peerSession{key: "old"}
	if err := claimTunnelTransport(old, binding); err != nil { t.Fatal(err) }
	peerTunnelBindings.Store(old, binding)

	candidate := &peerSession{key: "replacement"}
	if err := claimTunnelTransport(candidate, binding); !errors.Is(err, errTransportLaneLimit) { t.Fatalf("overlapping replacement should be rejected: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("overlap rejection active=%d want=1", got) }

	forgetPeerTunnel(old)
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 0 { t.Fatalf("after old transport teardown active=%d want=0", got) }
	if err := claimTunnelTransport(candidate, binding); err != nil { t.Fatalf("break-before-make replacement rejected after teardown: %v", err) }
	peerTunnelBindings.Store(candidate, binding)
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("replacement active=%d want=1", got) }
	bound, ok := peerTunnelBinding(candidate); if !ok { t.Fatal("replacement lost binding") }
	if bound.Config.TunnelID != binding.Config.TunnelID || bound.Config.Address4 != binding.Config.Address4 { t.Fatal("replacement changed tunnel identity/lease") }
}

func TestRejectedSecondTransportLeavesOldTransportClaimed(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding(); old := &peerSession{key:"old"}
	if err := claimTunnelTransport(old, binding); err != nil { t.Fatal(err) }
	peerTunnelBindings.Store(old, binding)
	second := &peerSession{key:"second"}
	if err := claimTunnelTransport(second, binding); !errors.Is(err, errTransportLaneLimit) { t.Fatalf("second transport rejection=%v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("second transport rejection disturbed old transport: active=%d", got) }
	if _, ok := peerTunnelBinding(old); !ok { t.Fatal("second transport rejection removed old binding") }
}
