package logicaltunnel

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

func mustInstallation(t *testing.T, s string) InstallationID {
	t.Helper()
	id, err := ParseInstallationID(s)
	if err != nil { t.Fatal(err) }
	return id
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := ParseManager("10.66.0.0/29", []string{"0.0.0.0/0"})
	if err != nil { t.Fatal(err) }
	return m
}

func TestProductTransportLanePolicyAllowsOneThroughFour(t *testing.T) {
	if MinProductPublicTransportLanes != 1 || MaxProductPublicTransportLanes != 4 {
		t.Fatalf("ADR-0012 transport bounds changed: min=%d max=%d", MinProductPublicTransportLanes, MaxProductPublicTransportLanes)
	}
	for _, n := range []int{1, 2, 3, 4} {
		if err := ValidateProductTransportLaneCount(n); err != nil {
			t.Fatalf("valid public transport count %d rejected: %v", n, err)
		}
	}
	for _, n := range []int{-1, 0, 5, 8} {
		if err := ValidateProductTransportLaneCount(n); !errors.Is(err, ErrTransportLanes) {
			t.Fatalf("invalid public transport count %d unexpectedly accepted: %v", n, err)
		}
	}
}

func TestSameAccountInstallationsReceiveDistinctLeases(t *testing.T) {
	m := newTestManager(t)
	a, err := m.Acquire("shared-account", mustInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil { t.Fatal(err) }
	b, err := m.Acquire("shared-account", mustInstallation(t, "ffeeddccbbaa99887766554433221100"))
	if err != nil { t.Fatal(err) }
	if a.Config.TunnelID == b.Config.TunnelID { t.Fatal("distinct installations reused TunnelID") }
	if a.Config.Address4 == b.Config.Address4 { t.Fatalf("distinct installations reused lease %s", a.Config.Address4) }
	if a.Config.Address4 != "10.66.0.1/32" || b.Config.Address4 != "10.66.0.2/32" {
		t.Fatalf("unexpected deterministic leases: A=%s B=%s", a.Config.Address4, b.Config.Address4)
	}
}

func TestSameInstallationReacquiresSameActiveTunnel(t *testing.T) {
	m := newTestManager(t)
	installation := mustInstallation(t, "00112233445566778899aabbccddeeff")
	first, err := m.Acquire("shared-account", installation)
	if err != nil { t.Fatal(err) }
	second, err := m.Acquire("shared-account", installation)
	if err != nil { t.Fatal(err) }
	if first.Config.TunnelID != second.Config.TunnelID || first.Config.Address4 != second.Config.Address4 {
		t.Fatalf("active logical tunnel was not stable: first=%+v second=%+v", first.Config, second.Config)
	}
}

func TestReleaseMakesLowestLeaseReusableDeterministically(t *testing.T) {
	m := newTestManager(t)
	first, err := m.Acquire("shared-account", mustInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil { t.Fatal(err) }
	if err := m.Release(first.Config.TunnelID); err != nil { t.Fatal(err) }
	replacement, err := m.Acquire("shared-account", mustInstallation(t, "11112222333344445555666677778888"))
	if err != nil { t.Fatal(err) }
	if replacement.Config.Address4 != first.Config.Address4 {
		t.Fatalf("released address was not deterministically reused: first=%s replacement=%s", first.Config.Address4, replacement.Config.Address4)
	}
	if replacement.Config.TunnelID == first.Config.TunnelID { t.Fatal("new logical tunnel reused released TunnelID") }
}

func TestValidateIPv4SourceRejectsOtherTunnelLease(t *testing.T) {
	m := newTestManager(t)
	a, _ := m.Acquire("shared-account", mustInstallation(t, "00112233445566778899aabbccddeeff"))
	b, _ := m.Acquire("shared-account", mustInstallation(t, "ffeeddccbbaa99887766554433221100"))
	aIP, _ := a.Config.LeaseIPv4()
	bIP, _ := b.Config.LeaseIPv4()
	packet := ipv4Packet(t, bIP, netip.MustParseAddr("1.1.1.1"))
	if err := ValidateIPv4Source(packet, aIP); !errors.Is(err, ErrSourceSpoof) { t.Fatalf("spoofed B source accepted for A lease: %v", err) }
	if err := ValidateIPv4Source(packet, bIP); err != nil { t.Fatalf("owner source rejected: %v", err) }
}

func ipv4Packet(t *testing.T, src, dst netip.Addr) []byte {
	t.Helper()
	if !src.Is4() || !dst.Is4() { t.Fatal("test requires IPv4") }
	packet := make([]byte, 20)
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	s := src.As4(); d := dst.As4()
	copy(packet[12:16], s[:]); copy(packet[16:20], d[:])
	return packet
}
