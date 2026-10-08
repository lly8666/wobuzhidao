//go:build windows

package windowsclient

import (
	"github.com/lly8666/wobuzhidao/internal/splitroute"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLargeSplitSnapshotPowerShellRender(t *testing.T) {
	bypass, err := splitroute.Direct("bypass-lan-cn", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildNetworkPlan(Config{AdapterAlias: "WBD", TunnelMTU: 1249, Lease4: netip.MustParsePrefix("10.66.0.2/32"), Server4: netip.MustParseAddr("198.51.100.10"), Physical: PhysicalPath{InterfaceIndex: 12, NextHop4: netip.MustParseAddr("192.168.1.1")}, StatePath: filepath.Join(t.TempDir(), "state.json"), Bypass4: bypass})
	if err != nil {
		t.Fatal(err)
	}
	p.CapturePrefixFile4 = filepath.Join(t.TempDir(), "capture.txt")
	var b strings.Builder
	for _, r := range p.CaptureRoutes {
		b.WriteString(r.Prefix.String() + "\n")
	}
	if err = os.WriteFile(p.CapturePrefixFile4, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	args, err := p.PowerShellArgs("Render", filepath.Join("..", "..", "scripts", "windows_client_network.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("powershell.exe", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("render: %v %s", err, out)
	}
	var captured []netip.Prefix
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[1] == "CAPTURE" {
			prefix, err := netip.ParsePrefix(f[2])
			if err != nil {
				t.Fatal(err)
			}
			captured = append(captured, prefix)
		}
	}
	if len(captured) != len(p.CaptureRoutes) {
		t.Fatalf("render capture count=%d wanted=%d", len(captured), len(p.CaptureRoutes))
	}
	for _, s := range []string{"192.168.1.2", "223.5.5.5", "198.51.100.10"} {
		if splitroute.Contains(captured, netip.MustParseAddr(s)) {
			t.Fatal("captured direct", s)
		}
	}
	if !splitroute.Contains(captured, netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("foreign missing")
	}
	t.Logf("POWERSHELL_RENDER_PASS prefixes=%d physical=NOT_RUN", len(captured))
}

func TestWindowsSplitApplyCleanupAndRollbackOwnership(t *testing.T) {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join("..", "..", "tools", "test_windows_splitroute.ps1")).CombinedOutput()
	if err != nil {
		t.Fatalf("Apply/Cleanup simulation: %v %s", err, out)
	}
	for _, scenario := range []string{"success", "route-failure", "dns-failure", "dns-off", "alias-failure"} {
		marker := "WINDOWS_SPLIT_OWNERSHIP_MOCK_PASS scenario=" + scenario + " "
		if strings.Count(string(out), marker) != 1 {
			t.Fatalf("missing or duplicate %s ownership receipt: %s", scenario, out)
		}
	}
	if strings.Count(string(out), "WINDOWS_SPLIT_OWNERSHIP_MOCK_PASS") != 5 {
		t.Fatalf("unexpected ownership scenario receipts: %s", out)
	}
	t.Log(string(out))
}
