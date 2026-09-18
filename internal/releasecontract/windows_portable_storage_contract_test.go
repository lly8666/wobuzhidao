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
		for _, forbidden := range []string{
			`os.Getenv("ProgramData")`,
			`os.Getenv("LOCALAPPDATA")`,
			"os.UserCacheDir()",
			"os.TempDir()",
			"$env:ProgramData",
			"$env:LOCALAPPDATA",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s still references system storage expression %q", label, forbidden)
			}
		}
	}

	launcher := files["portable launcher"]
	requireContains(t, launcher, `filepath.Join(portableDir, "wbd.json")`, "portable launcher fixed profile")
	requireNotContains(t, launcher, `flag.String("profile"`, "portable launcher profile override")
	requireNotContains(t, launcher, `"-profile"`, "portable launcher child GUI arguments")

	gui := files["GUI runtime"]
	requireNotContains(t, gui, `flag.String("profile"`, "GUI profile override")
	requireNotContains(t, gui, "-profile", "GUI legacy profile override text")
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

func TestWindowsPortableCNBaselineIsRepositoryOwnedAndNonBlocking(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")
	requireContains(t, workflow, `internal\ipset\seed\cn4.txt`, "Windows portable repository CN seed")
	requireContains(t, workflow, `source=repository-frozen`, "Windows portable frozen CN source marker")
	requireNotContains(t, workflow, "ftp.apnic.net/stats/apnic/delegated-apnic-latest", "Windows portable CN live build dependency")

	gui := readRepoFile(t, "cmd/wbd-windows-gui/main_windows.go")
	requireContains(t, gui, "profile.AutoUpdateCN = false", "portable GUI non-blocking CN policy")
	requireContains(t, gui, "WBD_GUI_CONNECTED state=connected routes_ready=1", "portable GUI final routing readiness marker")

	controller := readRepoFile(t, "internal/windowsruntime/controller.go")
	requireContains(t, controller, "ipset.EnsureEmbeddedCNBaseline(profile.CNSetDir)", "Windows runtime embedded CN fallback")

	seed := readRepoFile(t, "internal/ipset/seed/cn4.txt")
	requireContains(t, seed, "1.0.1.0/24", "repository CN baseline representative route")
}
