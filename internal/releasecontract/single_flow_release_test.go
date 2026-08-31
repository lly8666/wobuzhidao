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

func TestADR0012MultipathArchitectureFreeze(t *testing.T) {
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 4", "Logical Tunnel transport policy")
	requireContains(t, policy, "MinProductPublicTransportLanes = 1", "Logical Tunnel transport policy")
	requireContains(t, policy, "ValidateProductTransportLaneCount", "Logical Tunnel transport policy")

	adr := readRepoFile(t, "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md")
	requireContains(t, adr, "1..4 replaceable Transport Lanes", "ADR-0012")
	requireContains(t, adr, "single-flow is a per-lane invariant", "ADR-0012")
	requireContains(t, adr, "Replacement is make-before-break", "ADR-0012")
	requireContains(t, adr, "Game Lane semantics are the general multipath/replacement layer", "ADR-0012")

	withdrawn := readRepoFile(t, "docs/architecture/ADR-0013-global-single-public-flow-release-freeze.md")
	requireContains(t, withdrawn, "WITHDRAWN / SUPERSEDED BY REAFFIRMED ADR-0012", "ADR-0013")

	constitution := readRepoFile(t, "PROJECT_CONSTITUTION.md")
	requireContains(t, constitution, "1..4 independent WBD Transport Lanes", "project constitution")
	requireContains(t, constitution, "single-flow invariant applies to each lane", "project constitution")
	requireContains(t, constitution, "Make-before-break", "project constitution")
	if strings.Contains(constitution, "MaxProductPublicTransportLanes` is fixed at `1`") {
		t.Fatal("project constitution still freezes global transport count to one")
	}

	architecture := readRepoFile(t, "ARCHITECTURE.md")
	requireContains(t, architecture, "Per-lane single-flow, tunnel-level multipath", "architecture")
	requireContains(t, architecture, "1..4 active Transport Lanes", "architecture")
	requireContains(t, architecture, "A -> A+B -> B", "architecture")
	if strings.Contains(architecture, "Global single-public-flow invariant") {
		t.Fatal("architecture still selects withdrawn global single-public-flow policy")
	}
}

func TestLinuxProductRunPathOwnsSharedPublicRawMux(t *testing.T) {
	body := readRepoFile(t, "scripts/linux_server_manager.sh")
	start := strings.Index(body, "run_server() {")
	end := strings.Index(body, "\nuninstall_files() {")
	if start < 0 || end <= start {
		t.Fatal("cannot isolate linux run_server product path")
	}
	run := body[start:end]
	requireContains(t, run, "public_raw=", "linux run_server")
	requireContains(t, run, "max_tunnel_lanes=4", "linux run_server")
	requireContains(t, run, `"$PREFIX/bin/wbd-faketcp-mux" server`, "linux run_server")
	if strings.Contains(run, "wbd-reality-front") {
		t.Fatal("linux product run path must not start a parallel wbd-reality-front listener")
	}
}

func TestWindowsProductUsesPerLaneSameAssociationBootstrap(t *testing.T) {
	body := readRepoFile(t, "internal/windowsruntime/controller.go")
	if strings.Contains(body, "reality-bootstrap") || strings.Contains(body, "BuildBootstrap(") {
		t.Fatal("Windows product controller must not create a separate ordinary-TCP Reality bootstrap connection")
	}
	requireContains(t, body, "BuildFakeTCPCommand", "Windows controller")
	requireContains(t, body, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", "Windows controller")
	requireContains(t, body, "RuntimeDisconnected", "Windows controller")
}

func TestLinkServerAcceptsBoundedLaneSetForSameTunnel(t *testing.T) {
	body := readRepoFile(t, "cmd/wbd-link-server-mux/logical_tunnel.go")
	requireContains(t, body, "activeTunnelPeers", "LINK Logical Tunnel lane set")
	requireContains(t, body, "MaxProductPublicTransportLanes", "LINK Logical Tunnel lane set")
	requireContains(t, body, "errTransportLaneLimit", "LINK Logical Tunnel lane limit")
	requireContains(t, body, "releaseTunnelTransport", "LINK Logical Tunnel teardown")
	if strings.Contains(body, "LoadOrStore(key, ps)") {
		t.Fatal("LINK server still uses withdrawn one-peer-per-TunnelID claim")
	}
}

func TestLinuxBundleCarriesSubstantiveSourceSHAAndPerLaneOperatorContract(t *testing.T) {
	builder := readRepoFile(t, "scripts/build_linux_server_bundle.sh")
	requireContains(t, builder, `BUILD_SOURCE_SHA=${WBD_SOURCE_SHA:-${GITHUB_SHA:-unknown}}`, "Linux bundle builder")
	requireContains(t, builder, `> "$root/SOURCE_SHA"`, "Linux bundle builder")
	requireContains(t, builder, "one public raw wbd-faketcp-mux listener", "Linux bundle README")
	requireContains(t, builder, "1..4 independent Transport Lanes", "Linux bundle README")
	requireContains(t, builder, "Each lane owns one FakeTCP SYN/4-tuple/sequence lineage", "Linux bundle README")
	requireContains(t, builder, "never starts it as a public listener", "Linux bundle README")

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
