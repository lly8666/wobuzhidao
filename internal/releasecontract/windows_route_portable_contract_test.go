package releasecontract

import (
	"strings"
	"testing"
)

func TestWindowsRouteApplyDoesNotReownTunInterfaceMTU(t *testing.T) {
	script := readRepoFile(t, "scripts/windows_tun_route.ps1")
	requireContains(t, script, "Schema = 'wbd-windows-route-state/v4'", "Windows route-state schema")
	requireContains(t, script, "WBD_WINDOWS_TUN_ROUTE_STATE_READY", "Windows route-state readiness marker")
	requireContains(t, script, "already owned and verified by wbd-tun", "single MTU owner contract")
	requireNotContains(t, script, "$ipif4 = Get-NetIPInterface", "route apply duplicate IPv4 interface query")
	requireNotContains(t, script, "$ipif6 = Get-NetIPInterface", "route apply duplicate IPv6 interface query")
	requireNotContains(t, script, "-NlMtuBytes $MTU", "route apply duplicate MTU mutation")
	requireNotContains(t, script, "-InterfaceMetric 5", "route apply interface metric mutation")

	stateAt := strings.Index(script, "WBD_WINDOWS_TUN_ROUTE_STATE_READY")
	findRouteAt := strings.Index(script, "$found = @(Find-NetRoute -RemoteIPAddress $item.IP)")
	if stateAt < 0 || findRouteAt < 0 || stateAt >= findRouteAt {
		t.Fatalf("route state must be persisted before physical route discovery: state=%d find=%d", stateAt, findRouteAt)
	}
}

func TestWindowsSplitPolicyStaysInsideTun(t *testing.T) {
	routing := readRepoFile(t, "internal/windowsruntime/routing_policy.go")
	plan := readRepoFile(t, "internal/windowsruntime/plan.go")
	rebind := readRepoFile(t, "internal/windowsruntime/route_rebind.go")
	tunMain := readRepoFile(t, "cmd/wbd-tun/main.go")

	requireContains(t, routing, `return routingPlan{Mode: "Full", CaptureLAN: true}`, "policy-agnostic host routing")
	requireNotContains(t, plan, "-PrefixFile4", "CN capture routes in runtime plan")
	requireNotContains(t, plan, "-DirectPrefixFile4", "CN direct routes in runtime plan")
	requireNotContains(t, rebind, "-DirectPrefixFile4", "CN direct routes during physical-route rebind")
	for _, want := range []string{"-proxy-lan=", "-proxy-china=", "-proxy-other=", "-direct-ifindex", "-route-state", "-cn4"} {
		requireContains(t, plan, want, "TUN split policy argument")
	}
	requireContains(t, tunMain, "tunsplit.NewClassifier", "in-TUN split classifier")
	requireContains(t, tunMain, "tunsplit.NewDirectEngine", "userspace direct stack")
	requireContains(t, tunMain, "WBD_TUN_SPLIT_READY", "split readiness marker")
}

func TestWindowsPortableShipsVerifiedCNBundle(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")
	for _, want := range []string{
		"Seed verified portable CN routing bundle",
		"internal\\ipset\\seed\\cn4.txt",
		"go run .\\cmd\\wbd-ipset -action install",
		"go run .\\cmd\\wbd-ipset -action verify",
		"'cn4.txt','cn6.txt','cn-manifest.json'",
		"WBD_WINDOWS_CN_BUNDLE_PASS source=repository-frozen",
	} {
		requireContains(t, workflow, want, "Windows portable CN routing bundle")
	}
	requireNotContains(t, workflow, "ftp.apnic.net/stats/apnic/delegated-apnic-latest", "Windows portable CN live build dependency")
}
