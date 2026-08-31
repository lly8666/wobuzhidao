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
	activeTunnelPeersMu.Lock()
	activeTunnelPeers = make(map[string]map[*peerSession]struct{})
	activeTunnelPeersMu.Unlock()
}

func TestClaimTunnelTransportAllowsOneAndRejectsSecond(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	first := &peerSession{key: "transport-1"}
	if err := claimTunnelTransport(first, binding); err != nil {
		t.Fatalf("first public transport claim failed: %v", err)
	}
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 {
		t.Fatalf("active public transport count=%d want=1", got)
	}
	second := &peerSession{key: "transport-2"}
	if err := claimTunnelTransport(second, binding); !errors.Is(err, errTransportLaneLimit) {
		t.Fatalf("second simultaneous public transport was not rejected: %v", err)
	}
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 {
		t.Fatalf("rejected transport changed active count: got=%d want=1", got)
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
		t.Fatalf("idempotent claim changed transport count: got=%d want=1", got)
	}
}

func TestForgetPeerTunnelFreesSingleTransportSlot(t *testing.T) {
	resetTunnelTransportTestState()
	t.Cleanup(resetTunnelTransportTestState)

	binding := testTunnelBinding()
	old := &peerSession{key: "old"}
	if err := claimTunnelTransport(old, binding); err != nil {
		t.Fatal(err)
	}
	peerTunnelBindings.Store(old, binding)

	candidate := &peerSession{key: "candidate-before-retire"}
	if err := claimTunnelTransport(candidate, binding); !errors.Is(err, errTransportLaneLimit) {
		t.Fatalf("candidate overlapped active public transport: %v", err)
	}

	forgetPeerTunnel(old)
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 0 {
		t.Fatalf("retiring old transport left active count=%d want=0", got)
	}
	candidate = &peerSession{key: "candidate-after-retire"}
	if err := claimTunnelTransport(candidate, binding); err != nil {
		t.Fatalf("retired public transport slot did not accept replacement: %v", err)
	}
	if got := activeTunnelTransportCount(binding.Config.TunnelID); got != 1 {
		t.Fatalf("replacement transport count=%d want=1", got)
	}
}

func TestReplacementKeepsLogicalTunnelLeaseWithoutPublicOverlap(t *testing.T) {
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
	oldTransport := &peerSession{key: "old-transport"}
	if err := claimTunnelTransport(oldTransport, firstBinding); err != nil {
		t.Fatalf("old public transport claim failed: %v", err)
	}
	peerTunnelBindings.Store(oldTransport, firstBinding)

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
	candidate := &peerSession{key: "candidate"}
	if err := claimTunnelTransport(candidate, replacementBinding); !errors.Is(err, errTransportLaneLimit) {
		t.Fatalf("candidate should be rejected until old public transport is retired: %v", err)
	}

	forgetPeerTunnel(oldTransport)
	if err := claimTunnelTransport(candidate, replacementBinding); err != nil {
		t.Fatalf("replacement could not claim after old transport retirement: %v", err)
	}
	peerTunnelBindings.Store(candidate, replacementBinding)
	if got := activeTunnelTransportCount(firstLease.Config.TunnelID); got != 1 {
		t.Fatalf("replacement active count=%d want=1", got)
	}
	bound, ok := peerTunnelBinding(candidate)
	if !ok {
		t.Fatal("replacement lost Logical Tunnel binding")
	}
	if bound.Config.TunnelID != firstBinding.Config.TunnelID || bound.Config.Address4 != firstBinding.Config.Address4 {
		t.Fatalf("replacement did not retain logical identity/lease: first=%+v replacement=%+v", firstBinding.Config, bound.Config)
	}
}
