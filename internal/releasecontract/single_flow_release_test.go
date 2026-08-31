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
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func requireContains(t *testing.T, body, want, label string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("%s missing release-contract marker %q", label, want)
	}
}

func requireNotContains(t *testing.T, body, forbidden, label string) {
	t.Helper()
	if strings.Contains(body, forbidden) {
		t.Fatalf("%s contains forbidden release-contract marker %q", label, forbidden)
	}
}

func TestADR0014GlobalSingleFlowArchitectureFreeze(t *testing.T) {
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 1", "Logical Tunnel transport policy")
	requireContains(t, policy, "MinProductPublicTransportLanes = 1", "Logical Tunnel transport policy")
	requireContains(t, policy, "product permits exactly one active public transport lane", "Logical Tunnel transport policy")

	adr := readRepoFile(t, "docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md")
	requireContains(t, adr, "PRODUCT-OWNER FINAL FREEZE", "ADR-0014")
	requireContains(t, adr, "exactly one public WBD 4-tuple", "ADR-0014")
	requireContains(t, adr, "no concurrent second WBD Transport Lane", "ADR-0014")
	requireContains(t, adr, "no FIN/RST/new WBD payload SYN", "ADR-0014")

	adr12 := readRepoFile(t, "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md")
	requireContains(t, adr12, "PARTIALLY SUPERSEDED BY ADR-0014", "ADR-0012")
	adr13 := readRepoFile(t, "docs/architecture/ADR-0013-global-single-public-flow-release-freeze.md")
	requireContains(t, adr13, "HISTORICAL / SUPERSEDED BY ADR-0014", "ADR-0013")

	constitution := readRepoFile(t, "PROJECT_CONSTITUTION.md")
	requireContains(t, constitution, "ADR-0014 is authoritative", "project constitution")
	requireContains(t, constitution, "exactly one active public WBD 4-tuple", "project constitution")
	requireContains(t, constitution, "A simultaneous second WBD public transport", "project constitution")
	requireNotContains(t, constitution, "1..4 independent WBD Transport Lanes", "project constitution")

	architecture := readRepoFile(t, "ARCHITECTURE.md")
	requireContains(t, architecture, "ADR-0014 is authoritative", "architecture")
	requireContains(t, architecture, "exactly one WBD-owned raw TCP-shaped FakeTCP association", "architecture")
	requireContains(t, architecture, "Product transport cardinality is exactly one while connected", "architecture")
	requireNotContains(t, architecture, "A connected Logical Tunnel may own **1..4 active Transport Lanes**", "architecture")
}

func TestLinuxProductRunPathOwnsOneSharedPublicRawMuxWithoutGameHop(t *testing.T) {
	body := readRepoFile(t, "scripts/linux_server_manager.sh")
	start := strings.Index(body, "run_server() {")
	end := strings.Index(body, "\nuninstall_files() {")
	if start < 0 || end <= start {
		t.Fatal("cannot isolate linux run_server product path")
	}
	run := body[start:end]
	requireContains(t, run, "public_raw=", "linux run_server")
	requireContains(t, run, "max_tunnel_lanes=1", "linux run_server")
	requireContains(t, run, `"$PREFIX/bin/wbd-faketcp-mux" server`, "linux run_server")
	requireContains(t, run, `"$PREFIX/bin/wbd-link-server-mux" -listen "$WBD_LINK_LISTEN" -service "$WBD_PLATFORM_LISTEN"`, "linux run_server")
	requireNotContains(t, run, `"$PREFIX/bin/wbd-game-lane-server"`, "linux run_server")
	requireNotContains(t, run, "wbd-reality-front", "linux run_server")
}

func TestWindowsProductUsesOneSameAssociationBootstrap(t *testing.T) {
	controller := readRepoFile(t, "internal/windowsruntime/controller.go")
	if strings.Contains(controller, "reality-bootstrap") || strings.Contains(controller, "BuildBootstrap(") {
		t.Fatal("Windows product controller must not create a separate ordinary-TCP Reality bootstrap connection")
	}
	requireContains(t, controller, "BuildFakeTCPCommand", "Windows controller")
	requireContains(t, controller, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", "Windows controller")
	requireNotContains(t, controller, "BuildMultiLanePlan", "Windows controller")

	plan := readRepoFile(t, "internal/windowsruntime/plan.go")
	requireContains(t, plan, "ValidateProductTransportLaneCount(p.Lanes)", "Windows profile")
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 1", "Windows transport ceiling")
}

func TestLinkServerRejectsSecondPublicTransportForSameTunnel(t *testing.T) {
	body := readRepoFile(t, "cmd/wbd-link-server-mux/logical_tunnel.go")
	requireContains(t, body, "activeTunnelPeers", "LINK Logical Tunnel transport set")
	requireContains(t, body, "MaxProductPublicTransportLanes", "LINK Logical Tunnel transport limit")
	requireContains(t, body, "errTransportLaneLimit", "LINK Logical Tunnel transport limit")
	requireContains(t, body, "releaseTunnelTransport", "LINK Logical Tunnel teardown")
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 1", "LINK transport ceiling")
}

func TestLinuxBundleCarriesSubstantiveSourceSHAAndGlobalSingleFlowContract(t *testing.T) {
	builder := readRepoFile(t, "scripts/build_linux_server_bundle.sh")
	requireContains(t, builder, `BUILD_SOURCE_SHA=${WBD_SOURCE_SHA:-${GITHUB_SHA:-unknown}}`, "Linux bundle builder")
	requireContains(t, builder, `> "$root/SOURCE_SHA"`, "Linux bundle builder")
	requireContains(t, builder, "one public raw wbd-faketcp-mux listener", "Linux bundle README")
	requireContains(t, builder, "exactly one active public WBD transport", "Linux bundle README")
	requireContains(t, builder, "one FakeTCP SYN/4-tuple/sequence lineage", "Linux bundle README")
	requireContains(t, builder, "never starts it as a public listener", "Linux bundle README")
	requireContains(t, builder, "max_tunnel_lanes=1", "Linux release evidence")
	requireNotContains(t, builder, "A Logical Tunnel may bind 1..4 independent Transport Lanes", "Linux bundle README")

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
