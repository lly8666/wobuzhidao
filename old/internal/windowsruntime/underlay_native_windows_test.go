//go:build windows

package windowsruntime

import (
	"net/netip"
	"testing"
)

func TestWindowsDefaultUnderlayDiscovererIsNative(t *testing.T) {
	if _, ok := defaultUnderlayDiscoverer().(NativeWindowsUnderlayDiscoverer); !ok {
		t.Fatalf("default discoverer=%T want NativeWindowsUnderlayDiscoverer", defaultUnderlayDiscoverer())
	}
}

func TestNativeRouteSelectionMatchesPowerShellPolicy(t *testing.T) {
	mk := func(prefix string, metric uint64, ifIndex uint32) nativeRouteCandidate {
		return nativeRouteCandidate{prefix: netip.MustParsePrefix(prefix), metric: metric, ifIndex: ifIndex}
	}
	got, err := selectNativeRoute([]nativeRouteCandidate{
		mk("0.0.0.0/0", 5, 9),
		mk("203.0.113.0/24", 40, 12),
		mk("203.0.113.0/24", 20, 11),
		mk("203.0.113.0/24", 20, 10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.prefix.Bits() != 24 || got.metric != 20 || got.ifIndex != 10 {
		t.Fatalf("selected=%+v", got)
	}
}

func TestNpcapDeviceUsesAdapterGuid(t *testing.T) {
	got, err := npcapDeviceForAdapter("{11111111-2222-3333-4444-555555555555}")
	if err != nil {
		t.Fatal(err)
	}
	want := `\Device\NPF_{11111111-2222-3333-4444-555555555555}`
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}
