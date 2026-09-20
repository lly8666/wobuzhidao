package logicaltunnel

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

func mustInstallation(t *testing.T, s string) InstallationID {
	t.Helper()
	id, err := ParseInstallationID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newIdentityTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := ParseManager("10.66.0.0/29", []string{"10.20.0.0/16", "0.0.0.0/0"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTunnelIDRawBytesMatchAdmissionWireIdentity(t *testing.T) {
	raw := []byte("0123456789abcdef")
	id, err := TunnelIDFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := id.Bytes(); !bytes.Equal(got, raw) {
		t.Fatalf("raw tunnel id=%x want=%x", got, raw)
	}
	parsed, err := ParseTunnelID(id.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsed != id {
		t.Fatalf("hex round trip got=%s want=%s", parsed.String(), id.String())
	}
	if _, err := TunnelIDFromBytes(raw[:15]); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("short raw tunnel id err=%v", err)
	}
}

func TestSameInstallationReacquiresStableTunnelAndLease(t *testing.T) {
	m := newIdentityTestManager(t)
	installation := mustInstallation(t, "00112233445566778899aabbccddeeff")
	first, err := m.Acquire("solo", installation)
	if err != nil {
		t.Fatal(err)
	}
	first.Config.Routes4[0] = "mutated"
	second, err := m.Acquire(" solo ", installation)
	if err != nil {
		t.Fatal(err)
	}
	if first.Config.TunnelID != second.Config.TunnelID || first.Config.Address4 != second.Config.Address4 {
		t.Fatalf("stable logical tunnel changed first=%+v second=%+v", first.Config, second.Config)
	}
	if second.Config.Address4 != "10.66.0.1/32" {
		t.Fatalf("lease=%s want=10.66.0.1/32", second.Config.Address4)
	}
	if len(second.Config.Routes4) != 2 || second.Config.Routes4[0] != "0.0.0.0/0" || second.Config.Routes4[1] != "10.20.0.0/16" {
		t.Fatalf("routes were not canonical/owned: %v", second.Config.Routes4)
	}
}

func TestIdentityIsolationUsesAccountAndInstallation(t *testing.T) {
	m := newIdentityTestManager(t)
	aID := mustInstallation(t, "00112233445566778899aabbccddeeff")
	bID := mustInstallation(t, "ffeeddccbbaa99887766554433221100")

	a, err := m.Acquire("solo", aID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Acquire("solo", bID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Acquire("other", aID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Config.TunnelID == b.Config.TunnelID || a.Config.TunnelID == c.Config.TunnelID || b.Config.TunnelID == c.Config.TunnelID {
		t.Fatal("isolated identities reused TunnelID")
	}
	if a.Config.Address4 == b.Config.Address4 || a.Config.Address4 == c.Config.Address4 || b.Config.Address4 == c.Config.Address4 {
		t.Fatalf("isolated identities reused lease A=%s B=%s C=%s", a.Config.Address4, b.Config.Address4, c.Config.Address4)
	}
}

func TestConcurrentReacquireDoesNotForkOneInstallation(t *testing.T) {
	m := newIdentityTestManager(t)
	installation := mustInstallation(t, "1234567890abcdef1234567890abcdef")
	const n = 32
	out := make(chan Lease, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := m.Acquire("solo", installation)
			out <- lease
			errs <- err
		}()
	}
	wg.Wait()
	close(out)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first Lease
	for lease := range out {
		if first.Account == "" {
			first = lease
			continue
		}
		if lease.Config.TunnelID != first.Config.TunnelID || lease.Config.Address4 != first.Config.Address4 {
			t.Fatalf("concurrent reacquire forked logical tunnel first=%+v got=%+v", first.Config, lease.Config)
		}
	}
}

func TestReleaseFreesLeaseButNeverReusesTunnelIdentity(t *testing.T) {
	m := newIdentityTestManager(t)
	first, err := m.Acquire("solo", mustInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Release(first.Config.TunnelID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lookup(first.Config.TunnelID); !errors.Is(err, ErrUnknownTunnel) {
		t.Fatalf("released tunnel lookup err=%v", err)
	}
	replacement, err := m.Acquire("solo", mustInstallation(t, "11112222333344445555666677778888"))
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Config.Address4 != first.Config.Address4 {
		t.Fatalf("released address not reused first=%s replacement=%s", first.Config.Address4, replacement.Config.Address4)
	}
	if replacement.Config.TunnelID == first.Config.TunnelID {
		t.Fatal("new logical tunnel reused released TunnelID")
	}
}

func TestPoolAndIdentityValidationFailClosed(t *testing.T) {
	if _, err := ParseManager("10.66.0.0/31", nil); !errors.Is(err, ErrInvalidPool) {
		t.Fatalf("/31 pool err=%v", err)
	}
	if _, err := ParseManager("10.66.0.0/29", []string{"::/0"}); !errors.Is(err, ErrInvalidPool) {
		t.Fatalf("IPv6 route err=%v", err)
	}
	if _, err := ParseInstallationID("abcd"); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("short installation err=%v", err)
	}
	m := newIdentityTestManager(t)
	if _, err := m.Acquire("   ", mustInstallation(t, "00112233445566778899aabbccddeeff")); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("empty account err=%v", err)
	}
}
