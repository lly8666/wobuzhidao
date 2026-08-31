package main

import (
	"errors"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func testTunnelBinding() realityfront.TicketBinding {
	return realityfront.TicketBinding{
		Account:        "test-account",
		InstallationID: logicaltunnel.InstallationID("00112233445566778899aabbccddeeff"),
		Config: logicaltunnel.TunnelConfig{
			TunnelID: logicaltunnel.TunnelID("11223344556677889900aabbccddeeff"),
			Address4: "10.66.0.1/32",
			Routes4:  []string{"0.0.0.0/0"},
		},
	}
}

func resetTunnelTransportTestState() {
	peerTunnelBindings.Range(func(key, _ any) bool {
		peerTunnelBindings.Delete(key)
		return true
	})
	activeTunnelPeers.Range(func(key, _ any) bool {
		activeTunnelPeers.Delete(key)
		return true
	})
}

func TestClaimTunnelTransportRejectsSecondPeerForSameTunnel(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	first := &peerSession{key: "first"}
	second := &peerSession{key: "second"}
	if err := claimTunnelTransport(first, binding); err != nil {
		t.Fatalf("first transport claim failed: %v", err)
	}
	if err := claimTunnelTransport(second, binding); !errors.Is(err, errConcurrentTunnelTransport) {
		t.Fatalf("second concurrent transport was not rejected: %v", err)
	}
}

func TestClaimTunnelTransportIsIdempotentForSamePeer(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	peer := &peerSession{key: "same"}
	if err := claimTunnelTransport(peer, binding); err != nil {
		t.Fatal(err)
	}
	if err := claimTunnelTransport(peer, binding); err != nil {
		t.Fatalf("same peer could not repeat its claim: %v", err)
	}
}

func TestForgetPeerTunnelReleasesSingleTransportClaim(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	first := &peerSession{key: "first"}
	second := &peerSession{key: "second"}
	if err := claimTunnelTransport(first, binding); err != nil {
		t.Fatal(err)
	}
	peerTunnelBindings.Store(first, binding)
	forgetPeerTunnel(first)
	if err := claimTunnelTransport(second, binding); err != nil {
		t.Fatalf("replacement transport remained blocked after old peer teardown: %v", err)
	}
}
