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

func TestBreakBeforeMakeReplacementKeepsLogicalTunnelLease(t *testing.T) {
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
	first := &peerSession{key: "first-epoch"}
	if err := claimTunnelTransport(first, firstBinding); err != nil {
		t.Fatalf("first transport claim failed: %v", err)
	}
	peerTunnelBindings.Store(first, firstBinding)

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
	replacement := &peerSession{key: "replacement-epoch"}
	if err := claimTunnelTransport(replacement, replacementBinding); !errors.Is(err, errConcurrentTunnelTransport) {
		t.Fatalf("replacement overlapped old public transport: %v", err)
	}

	forgetPeerTunnel(first)
	if err := claimTunnelTransport(replacement, replacementBinding); err != nil {
		t.Fatalf("replacement could not claim after old transport teardown: %v", err)
	}
	peerTunnelBindings.Store(replacement, replacementBinding)

	bound, ok := peerTunnelBinding(replacement)
	if !ok {
		t.Fatal("replacement lost Logical Tunnel binding")
	}
	if bound.Config.TunnelID != firstBinding.Config.TunnelID || bound.Config.Address4 != firstBinding.Config.Address4 {
		t.Fatalf("replacement did not retain logical identity/lease: first=%+v replacement=%+v", firstBinding.Config, bound.Config)
	}
}
