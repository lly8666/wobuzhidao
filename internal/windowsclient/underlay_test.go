package windowsclient

import (
	"net/netip"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func testPhysicalUnderlay() PhysicalUnderlay {
	return PhysicalUnderlay{
		Peer4:          netip.MustParseAddr("198.51.100.10"),
		Source4:        netip.MustParseAddr("192.0.2.20"),
		InterfaceIndex: 12,
		PacketDevice:   "\\Device\\NPF_{11111111-2222-3333-4444-555555555555}",
		SourceMAC:      [6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55},
		NextHop4:       netip.MustParseAddr("192.0.2.1"),
		NextHopMAC:     [6]byte{0x02, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
	}
}

func TestPhysicalUnderlayFeedsSamePathToNpcapAndNetworkPlan(t *testing.T) {
	u := testPhysicalUnderlay()
	if err := u.Validate(); err != nil {
		t.Fatal(err)
	}
	npcap, err := u.NpcapConfig(
		41001,
		443,
		7,
		faketcp.PacketPersonaWindows11,
	)
	if err != nil {
		t.Fatal(err)
	}
	if npcap.Flow.LocalIP != [4]byte{192, 0, 2, 20} ||
		npcap.Flow.PeerIP != [4]byte{198, 51, 100, 10} ||
		npcap.Flow.LocalPort != 41001 ||
		npcap.Flow.PeerPort != 443 ||
		npcap.Generation != 7 {
		t.Fatalf("Npcap flow=%+v generation=%d", npcap.Flow, npcap.Generation)
	}

	path, err := u.PhysicalPath()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildNetworkPlan(Config{
		AdapterAlias: "WBD",
		Lease4:       netip.MustParsePrefix("10.66.0.7/32"),
		Server4:      u.Peer4,
		Physical:     path,
		StatePath:    "C:\\ProgramData\\WBD\\state.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.UnderlayRoute.Prefix.String() != "198.51.100.10/32" ||
		plan.UnderlayRoute.InterfaceIndex != u.InterfaceIndex ||
		plan.UnderlayRoute.NextHop != u.NextHop4 {
		t.Fatalf("underlay route=%+v", plan.UnderlayRoute)
	}
	for _, route := range plan.CaptureRoutes {
		if route.InterfaceIndex == u.InterfaceIndex {
			t.Fatalf("capture route unexpectedly owns physical interface: %+v", route)
		}
	}
}

func TestPhysicalRouteSelectionLongestPrefixMetricThenInterface(t *testing.T) {
	selected, err := selectPhysicalRoute([]physicalRouteCandidate{
		{
			prefix: netip.MustParsePrefix("0.0.0.0/0"),
			metric: 1,
			interfaceIndex: 3,
		},
		{
			prefix: netip.MustParsePrefix("198.51.100.0/24"),
			metric: 30,
			interfaceIndex: 12,
		},
		{
			prefix: netip.MustParsePrefix("198.51.100.0/24"),
			metric: 20,
			interfaceIndex: 11,
		},
		{
			prefix: netip.MustParsePrefix("198.51.100.0/24"),
			metric: 20,
			interfaceIndex: 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.prefix.Bits() != 24 ||
		selected.metric != 20 ||
		selected.interfaceIndex != 10 {
		t.Fatalf("selected=%+v", selected)
	}
}

func TestNpcapDeviceUsesWindowsAdapterGUID(t *testing.T) {
	for _, input := range []string{
		"{11111111-2222-3333-4444-555555555555}",
		"11111111-2222-3333-4444-555555555555",
		"\\Device\\Tcpip_{11111111-2222-3333-4444-555555555555}",
	} {
		got, err := npcapDeviceForAdapter(input)
		if err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		want := "\\Device\\NPF_{11111111-2222-3333-4444-555555555555}"
		if got != want {
			t.Fatalf("%q => %q want %q", input, got, want)
		}
	}
	if _, err := npcapDeviceForAdapter("Ethernet"); err == nil {
		t.Fatal("non-GUID adapter name unexpectedly accepted")
	}
}
