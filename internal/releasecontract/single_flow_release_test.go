package releasecontract

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok { t.Fatal("runtime.Caller failed") }
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(path)))
	if err != nil { t.Fatalf("read %s: %v", path, err) }
	return string(b)
}

func requireContains(t *testing.T, body, want, label string) {
	t.Helper()
	if !strings.Contains(body, want) { t.Fatalf("%s missing release-contract marker %q", label, want) }
}

func requireNotContains(t *testing.T, body, forbidden, label string) {
	t.Helper()
	if strings.Contains(body, forbidden) { t.Fatalf("%s contains forbidden release-contract marker %q", label, forbidden) }
}

func TestPerLaneSingleFlowLogicalTunnelMultipathAuthority(t *testing.T) {
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MinProductPublicTransportLanes = 1", "Logical Tunnel transport policy")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 4", "Logical Tunnel transport policy")
	requireContains(t, policy, "product permits 1..4 active public transport lanes", "Logical Tunnel transport policy")

	adr11 := readRepoFile(t, "docs/architecture/ADR-0011-single-public-flow-reality-bootstrap.md")
	requireContains(t, adr11, "each independent Transport Lane/epoch", "ADR-0011")
	requireContains(t, adr11, "no FIN/RST/new WBD payload SYN", "ADR-0011")

	adr12 := readRepoFile(t, "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md")
	for _, want := range []string{
		"CURRENT LIFECYCLE AND MULTIPATH AUTHORITY",
		"per-Transport-Lane invariant",
		"not a global one-flow-per-Logical-Tunnel invariant",
		"MaxProductPublicTransportLanes = 4",
		"Game / weak-network mode",
		"A -> A+B -> B",
		"shared TUN + one host NAT",
	} { requireContains(t, adr12, want, "ADR-0012") }

	adr14 := readRepoFile(t, "docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md")
	requireContains(t, adr14, "WITHDRAWN / INVALIDATED", "ADR-0014")
	requireContains(t, adr14, "incorrectly expanded", "ADR-0014")
	requireNotContains(t, adr14, "Status: **ACCEPTED / PRODUCT-OWNER FINAL FREEZE", "ADR-0014")

	constitution := readRepoFile(t, "PROJECT_CONSTITUTION.md")
	for _, want := range []string{
		"PER TRANSPORT LANE",
		"1..4 independent complete WBD Transport Lanes",
		"Game Lane is a product multipath mechanism",
		"not research-only",
		"A -> A+B -> B",
		"one shared WBD TUN",
	} { requireContains(t, constitution, want, "project constitution") }
	requireNotContains(t, constitution, "A simultaneous second WBD public transport for the same Logical Tunnel is forbidden", "project constitution")

	architecture := readRepoFile(t, "ARCHITECTURE.md")
	for _, want := range []string{
		"each independent Transport Lane",
		"2..4 lanes",
		"first valid arrival is delivered once",
		"A ACTIVE",
		"one shared WBD TUN",
	} { requireContains(t, architecture, want, "architecture") }
}

func TestLinuxProductRunPathSupportsGameRaceSharedTUNAboveOnePublicRawMux(t *testing.T) {
	body := readRepoFile(t, "scripts/linux_server_manager.sh")
	start := strings.Index(body, "run_server() {")
	end := strings.Index(body, "\nuninstall_files() {")
	if start < 0 || end <= start { t.Fatal("cannot isolate linux run_server product path") }
	run := body[start:end]
	for _, want := range []string{
		"public_raw=",
		"max_tunnel_lanes=4",
		`"$PREFIX/bin/wbd-faketcp-mux" server`,
		`"$PREFIX/bin/wbd-game-lane-server"`,
		`"$PREFIX/bin/wbd-ip-gateway-shared"`,
		`-service "$WBD_SHARED_TUN_LISTEN"`,
		`-service "$WBD_GAME_LISTEN"`,
		`-raw-ip-service "$WBD_SHARED_TUN_LISTEN"`,
		`--tunnel-pool "$WBD_TUNNEL_POOL"`,
	} { requireContains(t, run, want, "linux run_server") }
	requireNotContains(t, run, "wbd-platform-proxy-server\" -listen", "linux raw-L3 product run")
	requireNotContains(t, run, "wbd-reality-front server", "linux run_server")

	sharedFW := readRepoFile(t, "scripts/linux_shared_tun_firewall.sh")
	requireContains(t, sharedFW, "one root-namespace shared TUN plus one host NAT", "shared-TUN firewall")
	requireContains(t, sharedFW, "never flushes host rulesets", "shared-TUN firewall")
}

func TestWindowsProductUsesMultiLaneSameAssociationBootstrap(t *testing.T) {
	controller := readRepoFile(t, "internal/windowsruntime/controller.go")
	if strings.Contains(controller, "run:reality-bootstrap") || strings.Contains(controller, "c.runner.Run(bootstrap)") {
		t.Fatal("Windows product controller must not create separate ordinary-TCP Reality bootstrap")
	}
	requireContains(t, controller, "BuildLaneBootstrap", "Windows controller")
	requireContains(t, controller, "BuildMultiLanePlan", "Windows controller")
	requireContains(t, controller, "StartMultiLane", "Windows controller")
	plan := readRepoFile(t, "internal/windowsruntime/multilane.go")
	requireContains(t, plan, "LaneBootstrap", "Windows multi-lane plan")
	requireContains(t, plan, "wbd-game-lane-client.exe", "Windows Game/race product path")
}

func TestLinkServerAllowsProductLaneSetAndRejectsFifth(t *testing.T) {
	body := readRepoFile(t, "cmd/wbd-link-server-mux/logical_tunnel.go")
	requireContains(t, body, "activeTunnelPeers", "LINK Logical Tunnel transport set")
	requireContains(t, body, "MaxProductPublicTransportLanes", "LINK Logical Tunnel transport limit")
	requireContains(t, body, "errTransportLaneLimit", "LINK Logical Tunnel fifth-lane limit")
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 4", "LINK transport ceiling")
}

func TestProductMakeBeforeBreakLifecycleIsExecutableControlCode(t *testing.T) {
	body := readRepoFile(t, "internal/logicaltunnel/lifecycle.go")
	for _, want := range []string{
		"BeginReplacement",
		"CandidateHealthy",
		"CandidateFailed",
		"BeginDrain",
		"Retire",
		"Generation",
	} { requireContains(t, body, want, "Logical Tunnel lane lifecycle") }
}

func TestLinuxBundleCarriesSubstantiveSourceSHAAndMultipathContract(t *testing.T) {
	builder := readRepoFile(t, "scripts/build_linux_server_bundle.sh")
	for _, want := range []string{
		`BUILD_SOURCE_SHA=${WBD_SOURCE_SHA:-${GITHUB_SHA:-unknown}}`,
		`> "$root/SOURCE_SHA"`,
		"one public raw wbd-faketcp-mux listener",
		"1..4 independent Transport Lanes",
		"one root-namespace shared WBD TUN",
		"max_tunnel_lanes=4",
		"game_product=1",
		"shared_tun=1",
		"host_nat=1",
	} { requireContains(t, builder, want, "Linux bundle builder") }
	requireNotContains(t, builder, "exactly one active public WBD transport", "Linux bundle README")
	requireNotContains(t, builder, "Game Lane server remains a research/reference binary", "Linux bundle README")
	workflow := readRepoFile(t, ".github/workflows/linux-server-release.yml")
	requireContains(t, workflow, `WBD_SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}`, "Linux release workflow")
}

func TestWindowsPortableCarriesMatchingSubstantiveSourceSHAEvidence(t *testing.T) {
	body := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")
	requireContains(t, body, `WBD_SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}`, "Windows portable workflow")
	requireContains(t, body, `-source-sha $env:WBD_SOURCE_SHA`, "Windows portable stage")
	requireContains(t, body, `name: wbd-windows-portable-${{ env.WBD_SOURCE_SHA }}`, "Windows portable artifact identity")
	if strings.Contains(body, "source_sha=$env:GITHUB_SHA") || strings.Contains(body, "-source-sha $env:GITHUB_SHA") {
		t.Fatal("Windows artifact source identity must not use pull_request merge GITHUB_SHA")
	}
}
