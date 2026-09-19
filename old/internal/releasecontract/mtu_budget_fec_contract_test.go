package releasecontract

import "testing"

// The physical FEC matrix and the MTU budget probe must agree on every fixed
// RS profile accepted by the product. pathmtu only needs to know whether FEC is
// enabled because every fixed profile uses the same wire header budget.
func TestMTUBudgetProbeAcceptsAllFixedFECProfiles(t *testing.T) {
	probe := readRepoFile(t, ".github/scripts/mtu_budget.go")
	soak := readRepoFile(t, "scripts/windows_linux_wintun_lane_soak.ps1")

	for _, profile := range []string{"20:4", "20:8", "20:10", "20:12", "20:16", "20:20"} {
		requireContains(t, probe, `"`+profile+`"`, "MTU budget fixed-FEC profile set")
		requireContains(t, soak, `'`+profile+`'`, "physical FEC soak profile set")
	}
	requireContains(t, probe, "fecEnabled = true", "fixed FEC must consume the FEC MTU header budget")
	requireContains(t, probe, "pathmtu.Features{FEC: fecEnabled, Game: true}", "MTU budget must derive from the normalized FEC enabled flag")
}
