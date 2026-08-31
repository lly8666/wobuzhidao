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

func TestGlobalSinglePublicFlowArchitectureFreeze(t *testing.T) {
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 1", "Logical Tunnel transport policy")
	requireContains(t, policy, "ValidateProductTransportLaneCount", "Logical Tunnel transport policy")

	adr := readRepoFile(t, "docs/architecture/ADR-0013-global-single-public-flow-release-freeze.md")
	requireContains(t, adr, "public WBD transport count = 1", "ADR-0013")
	requireContains(t, adr, "Replacement is break-before-make", "ADR-0013")
	requireContains(t, adr, "Multipath/Game Lane is not a release product path", "ADR-0013")

	constitution := readRepoFile(t, "PROJECT_CONSTITUTION.md")
	requireContains(t, constitution, "exactly one usable public WBD transport for a connected Logical Tunnel", "project constitution")
	requireContains(t, constitution, "A connected Logical Tunnel has **exactly one** usable public WBD FakeTCP association", "project constitution")
	requireContains(t, constitution, "Make-before-break/public candidate overlap is forbidden", "project constitution")
	if strings.Contains(constitution, "1..4 independent complete WBD transport lanes") {
		t.Fatal("project constitution still advertises 1..4 public transport lanes")
	}

	architecture := readRepoFile(t, "ARCHITECTURE.md")
	requireContains(t, architecture, "Global single-public-flow invariant", "architecture")
	requireContains(t, architecture, "release product configuration does not expose 2..4 public lanes", "architecture")
	if strings.Contains(architecture, "Replacement is make-before-break") {
		t.Fatal("architecture still selects make-before-break public transport overlap")
	}
}

func TestLinuxProductRunPathOwnsOnePublicRawListener(t *testing.T) {
	body := readRepoFile(t, "scripts/linux_server_manager.sh")
	start := strings.Index(body, "run_server() {")
	end := strings.Index(body, "\nuninstall_files() {")
	if start < 0 || end <= start {
		t.Fatal("cannot isolate linux run_server product path")
	}
	run := body[start:end]
	requireContains(t, run, "public_single_flow=", "linux run_server")
	requireContains(t, run, `"$PREFIX/bin/wbd-faketcp-mux" server`, "linux run_server")
	if strings.Contains(run, "wbd-reality-front") {
		t.Fatal("linux product run path must not start a parallel wbd-reality-front listener")
	}
}

func TestWindowsProductConnectOwnsOnlyOnePublicFakeTCPChild(t *testing.T) {
	body := readRepoFile(t, "internal/windowsruntime/controller.go")
	if strings.Contains(body, "reality-bootstrap") || strings.Contains(body, "BuildBootstrap(") {
		t.Fatal("Windows product controller must not create a second public Reality bootstrap connection")
	}
	requireContains(t, body, "BuildFakeTCPCommand", "Windows controller")
	requireContains(t, body, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", "Windows controller")
	requireContains(t, body, "RuntimeDisconnected", "Windows controller")
	if strings.Contains(body, "gamelane") || strings.Contains(body, "desiredLane") || strings.Contains(body, "candidateLane") {
		t.Fatal("Windows product controller must not expose a multipath/candidate public-lane lifecycle")
	}
}

func TestLinkServerRejectsConcurrentTransportForSameTunnel(t *testing.T) {
	body := readRepoFile(t, "cmd/wbd-link-server-mux/logical_tunnel.go")
	requireContains(t, body, "activeTunnelPeers", "LINK Logical Tunnel binding")
	requireContains(t, body, "LoadOrStore", "LINK Logical Tunnel binding")
	requireContains(t, body, "errConcurrentTunnelTransport", "LINK Logical Tunnel binding")
	requireContains(t, body, "releaseTunnelTransport", "LINK Logical Tunnel teardown")
}

func TestLinuxBundleCarriesSubstantiveSourceSHAAndSingleFlowOperatorContract(t *testing.T) {
	builder := readRepoFile(t, "scripts/build_linux_server_bundle.sh")
	requireContains(t, builder, `BUILD_SOURCE_SHA=${WBD_SOURCE_SHA:-${GITHUB_SHA:-unknown}}`, "Linux bundle builder")
	requireContains(t, builder, `> "$root/SOURCE_SHA"`, "Linux bundle builder")
	requireContains(t, builder, "one raw wbd-faketcp-mux public listener", "Linux bundle README")
	requireContains(t, builder, "no parallel kernel TCP Reality front", "Linux bundle README")
	requireContains(t, builder, "diagnostic/reference only", "Linux bundle README")

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
