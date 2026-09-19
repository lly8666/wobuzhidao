package windowsruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsUnderlayMonitorExcludesWBDOwnedPinnedRoute(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "windows_faketcp_underlay.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"[switch]$MonitorPhysicalPath",
		"connected physical-path monitoring requires the WBD route state file",
		"$state.UnderlayRoutes",
		"$state.AdapterInterfaceIndex",
		"$owned.ContainsKey($key)",
		"Test-IPv4InPrefix $Remote $prefix",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Windows physical-path discovery lost %q guard", want)
		}
	}
}
