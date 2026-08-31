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
	activeTunnelPeersMu.Lock()
	activeTunnelPeers = make(map[string]map[*peerSession]struct{})
	activeTunnelPeersMu.Unlock()
}

func TestClaimTunnelTransportAllowsFourAndRejectsFifth(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	for i := 0; i < logicaltunnel.MaxProductPublicTransportLanes; i++ {
		peer := &peerSession{key: fmt.Sprintf("lane-%d", i+1)}
		if err := claimTunnelTransport(peer, binding); err != nil {
			t.Fatalf("lane %d claim failed: %v", i+1, err)
		}
	}
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 4 {
		t.Fatalf("active lane count=%d want=4", got)
	}
	fifth := &peerSession{key: "lane-5"}
	if err := claimTunnelTransport(fifth, binding); !errors.Is(err, errTransportLaneLimit) {
		t.Fatalf("fifth lane was not rejected by bounded lane set: %v", err)
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
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 {
		t.Fatalf("idempotent claim changed lane count: got=%d want=1", got)
	}
}

func TestForgetPeerTunnelFreesOnlyThatLane(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	peers := make([]*peerSession, 0, 4)
	for i := 0; i < 4; i++ {
		peer := &peerSession{key: fmt.Sprintf("lane-%d", i+1)}
		if err := claimTunnelTransport(peer, binding); err != nil {
			t.Fatal(err)
		}
		peerTunnelBindings.Store(peer, binding)
		peers = append(peers, peer)
	}
	forgetPeerTunnel(peers[1])
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 3 {
		t.Fatalf("release removed wrong number of lanes: got=%d want=3", got)
	}
	candidate := &peerSession{key: "candidate"}
	if err := claimTunnelTransport(candidate, binding); err != nil {
		t.Fatalf("freed lane slot did not accept candidate: %v", err)
	}
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 4 {
		t.Fatalf("candidate did not restore desired four-lane set: got=%d", got)
	}
}

func TestMakeBeforeBreakReplacementKeepsLogicalTunnelLease(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	manager, err := logicaltunnel.ParseManager("10.66.0.0/29", []string{"0.0.0.0/0"})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := logicaltunnel.ParseInstallationID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	firstLease, err := manager.Acquire("test-account", installation)
	if err != nil {
		t.Fatal(err)
	}
	firstBinding := realityfront.TicketBinding{
		Account:        firstLease.Account,
		InstallationID: firstLease.InstallationID,
		Config:         firstLease.Config,
	}
	oldLane := &peerSession{key: "old-lane"}
	if err := claimTunnelTransport(oldLane, firstBinding); err != nil {
		t.Fatalf("old lane claim failed: %v", err)
	}
	peerTunnelBindings.Store(oldLane, firstBinding)

	replacementLease, err := manager.Acquire("test-account", installation)
	if err != nil {
		t.Fatal(err)
	}
	if replacementLease.Config.TunnelID != firstLease.Config.TunnelID {
		t.Fatalf("replacement changed TunnelID: first=%s replacement=%s", firstLease.Config.TunnelID, replacementLease.Config.TunnelID)
	}
	if replacementLease.Config.Address4 != firstLease.Config.Address4 {
		t.Fatalf("replacement changed lease: first=%s replacement=%s", firstLease.Config.Address4, replacementLease.Config.Address4)
	}
	replacementBinding := realityfront.TicketBinding{
		Account:        replacementLease.Account,
		InstallationID: replacementLease.InstallationID,
		Config:         replacementLease.Config,
	}
	candidate := &peerSession{key: "candidate-lane"}
	if err := claimTunnelTransport(candidate, replacementBinding); err != nil {
		t.Fatalf("make-before-break candidate overlap was rejected: %v", err)
	}
	peerTunnelBindings.Store(candidate, replacementBinding)
	if got := activeTunnelTransportCount(firstLease.Config.TunnelID); got != 2 {
		t.Fatalf("A -> A+B overlap count=%d want=2", got)
	}

	forgetPeerTunnel(oldLane)
	if got := activeTunnelTransportCount(firstLease.Config.TunnelID); got != 1 {
		t.Fatalf("A+B -> B drain count=%d want=1", got)
	}
	bound, ok := peerTunnelBinding(candidate)
	if !ok {
		t.Fatal("candidate lost Logical Tunnel binding")
	}
	if bound.Config.TunnelID != firstBinding.Config.TunnelID || bound.Config.Address4 != firstBinding.Config.Address4 {
		t.Fatalf("candidate did not retain logical identity/lease: first=%+v candidate=%+v", firstBinding.Config, bound.Config)
	}
}
