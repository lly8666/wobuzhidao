package main

import (
	"errors"
	"fmt"
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

func TestClaimTunnelTransportAllowsFourAndRejectsFifth(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding()
	peers := make([]*peerSession, 0, logicaltunnel.MaxProductPublicTransportLanes)
	for i := 1; i <= logicaltunnel.MaxProductPublicTransportLanes; i++ {
		peer := &peerSession{key: fmt.Sprintf("transport-%d", i)}
		if err := claimTunnelTransport(peer, binding); err != nil { t.Fatalf("lane %d claim failed: %v", i, err) }
		peers = append(peers, peer)
		if got := activeTunnelTransportCount(binding.Config.TunnelID); got != i { t.Fatalf("after lane %d active=%d", i, got) }
	}
	fifth := &peerSession{key: "transport-5"}
	if err := claimTunnelTransport(fifth, binding); !errors.Is(err, errTransportLaneLimit) {
		t.Fatalf("fifth lane was not rejected: %v", err)
	}
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != logicaltunnel.MaxProductPublicTransportLanes { t.Fatalf("rejected fifth changed active=%d", got) }
}

func TestClaimTunnelTransportIsIdempotentForSamePeer(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding(); peer := &peerSession{key: "same"}
	if err := claimTunnelTransport(peer, binding); err != nil { t.Fatal(err) }
	if err := claimTunnelTransport(peer, binding); err != nil { t.Fatalf("same peer repeat claim failed: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("idempotent claim active=%d want=1", got) }
}

func TestForgetPeerTunnelFreesOneLaneSlot(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding()
	peers := make([]*peerSession, 0, 4)
	for i := 0; i < 4; i++ {
		p := &peerSession{key: fmt.Sprintf("p-%d", i)}
		if err := claimTunnelTransport(p, binding); err != nil { t.Fatal(err) }
		peerTunnelBindings.Store(p, binding); peers = append(peers, p)
	}
	candidate := &peerSession{key: "candidate"}
	if err := claimTunnelTransport(candidate, binding); !errors.Is(err, errTransportLaneLimit) { t.Fatalf("fifth candidate should be rejected: %v", err) }
	forgetPeerTunnel(peers[0])
	if err := claimTunnelTransport(candidate, binding); err != nil { t.Fatalf("freed lane slot rejected candidate: %v", err) }
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 4 { t.Fatalf("active=%d want=4", got) }
}

func TestMakeBeforeBreakReplacementAllowsOldAndCandidateOverlap(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	manager, err := logicaltunnel.ParseManager("10.66.0.0/29", []string{"0.0.0.0/0"}); if err != nil { t.Fatal(err) }
	installation, err := logicaltunnel.ParseInstallationID("00112233445566778899aabbccddeeff"); if err != nil { t.Fatal(err) }
	firstLease, err := manager.Acquire("test-account", installation); if err != nil { t.Fatal(err) }
	firstBinding := realityfront.TicketBinding{Account:firstLease.Account, InstallationID:firstLease.InstallationID, Config:firstLease.Config}
	oldTransport := &peerSession{key:"old-transport"}
	if err := claimTunnelTransport(oldTransport, firstBinding); err != nil { t.Fatal(err) }
	peerTunnelBindings.Store(oldTransport, firstBinding)

	replacementLease, err := manager.Acquire("test-account", installation); if err != nil { t.Fatal(err) }
	if replacementLease.Config.TunnelID != firstLease.Config.TunnelID || replacementLease.Config.Address4 != firstLease.Config.Address4 { t.Fatal("replacement changed Logical Tunnel identity or lease") }
	replacementBinding := realityfront.TicketBinding{Account:replacementLease.Account, InstallationID:replacementLease.InstallationID, Config:replacementLease.Config}
	candidate := &peerSession{key:"candidate"}
	if err := claimTunnelTransport(candidate, replacementBinding); err != nil { t.Fatalf("make-before-break candidate overlap rejected: %v", err) }
	peerTunnelBindings.Store(candidate, replacementBinding)
	if got := activeTunnelTransportCount(firstLease.Config.TunnelID); got != 2 { t.Fatalf("A+B overlap active=%d want=2", got) }
	forgetPeerTunnel(oldTransport)
	if got := activeTunnelTransportCount(firstLease.Config.TunnelID); got != 1 { t.Fatalf("after A retire active=%d want=1", got) }
	bound, ok := peerTunnelBinding(candidate); if !ok { t.Fatal("candidate lost binding") }
	if bound.Config.TunnelID != firstBinding.Config.TunnelID || bound.Config.Address4 != firstBinding.Config.Address4 { t.Fatal("candidate did not retain tunnel identity/lease") }
}

func TestCandidateFailureLeavesOldTransportClaimed(t *testing.T) {
	resetTunnelTransportTestState(); t.Cleanup(resetTunnelTransportTestState)
	binding := testTunnelBinding(); old := &peerSession{key:"old"}
	if err := claimTunnelTransport(old, binding); err != nil { t.Fatal(err) }
	peerTunnelBindings.Store(old, binding)
	// A failed candidate never claims a slot / never calls forgetPeerTunnel(old).
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 { t.Fatalf("candidate failure disturbed old lane: active=%d", got) }
	if _, ok := peerTunnelBinding(old); !ok { t.Fatal("candidate failure removed old binding") }
}
