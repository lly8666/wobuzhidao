package releasecontract

import (
	"strings"
	"testing"
)

func TestWindowsPortableUserRuntimeNeverUsesSystemStorage(t *testing.T) {
	files := map[string]string{
		"portable launcher": readRepoFile(t, "cmd/wbd-windows-portable/main_windows.go"),
		"GUI runtime": readRepoFile(t, "cmd/wbd-windows-gui/main_windows.go"),
		"GUI profile": readRepoFile(t, "cmd/wbd-windows-gui/profile_ui_windows.go"),
		"route script": readRepoFile(t, "scripts/windows_tun_route.ps1"),
		"Npcap script": readRepoFile(t, "scripts/windows_npcap_prepare.ps1"),
		"diagnostics": readRepoFile(t, "internal/windowsdiag/diagnose_windows.go"),
		"legacy bundle": readRepoFile(t, "internal/windowsbundle/bundle.go"),
	}
	for label, body := range files {
		for _, forbidden := range []string{"ProgramData", "LOCALAPPDATA", "UserCacheDir()", "os.TempDir()"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s still references system storage marker %q", label, forbidden)
			}
		}
	}

	launcher := files["portable launcher"]
	requireContains(t, launcher, `filepath.Join(portableDir, "wbd.json")`, "portable launcher fixed profile")
	requireNotContains(t, launcher, `flag.String("profile"`, "portable launcher profile override")
	requireNotContains(t, launcher, `"-profile"`, "portable launcher child GUI arguments")

	gui := files["GUI runtime"]
	requireNotContains(t, gui, `flag.String("profile"`, "GUI profile override")
	requireContains(t, gui, "windowsgui.LoadRuntimeProfile(path, portableDir, portableDir)", "GUI portable state")

	profileUI := files["GUI profile"]
	requireContains(t, profileUI, `profileUI.configPath = filepath.Join(dir, "wbd.json")`, "GUI single portable profile")
	requireNotContains(t, profileUI, "profiles.json", "GUI legacy profile store")
	requireNotContains(t, profileUI, "active-profile.json", "GUI legacy active profile copy")
}

func TestWindowsPortableDefaultRoutingKeepsLANDirect(t *testing.T) {
	gui := readRepoFile(t, "cmd/wbd-windows-gui/profile_ui_windows.go")
	requireContains(t, gui, "a, b := false, true", "GUI default LAN-direct policy")
	requireContains(t, gui, "ProxyLAN: &a, ProxyChina: &b, ProxyOther: &b", "GUI default explicit routing policy")

	example := readRepoFile(t, "docs/testing/wbd.example.json")
	requireContains(t, example, `"proxy_lan": false`, "portable example LAN policy")
	requireContains(t, example, `"proxy_china": true`, "portable example China policy")
	requireContains(t, example, `"proxy_other": true`, "portable example Other policy")
	requireNotContains(t, example, `"route_mode"`, "portable example legacy routing")
}
