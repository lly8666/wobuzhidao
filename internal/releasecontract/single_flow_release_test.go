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

func TestADR0012MultipathAuthority(t *testing.T) {
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MinProductPublicTransportLanes = 1", "Logical Tunnel transport policy")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 4", "Logical Tunnel transport policy")
	requireContains(t, policy, "Normal mode targets one lane; Game/weak-network mode may use 2..4 lanes", "Logical Tunnel transport policy")

	adr12 := readRepoFile(t, "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md")
	for _, want := range []string{
		"ACCEPTED / CURRENT LIFECYCLE AND MULTIPATH AUTHORITY",
		"single-flow is a per-Transport-Lane invariant",
		"1..4 independent complete WBD Transport Lanes",
		"A fifth simultaneously active product Transport Lane is rejected",
		"make-before-break",
	} { requireContains(t, adr12, want, "ADR-0012") }

	for _, path := range []string{
		"docs/architecture/ADR-0013-global-single-public-flow-release-freeze.md",
		"docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md",
		"docs/architecture/ADR-0015-human-authorized-global-single-public-flow.md",
	} {
		body := readRepoFile(t, path)
		requireContains(t, body, "WITHDRAWN", path)
	}

	constitution := readRepoFile(t, "PROJECT_CONSTITUTION.md")
	for _, want := range []string{
		"single-flow is PER TRANSPORT LANE",
		"Logical Tunnel may own 1..4 independent complete WBD Transport Lanes",
		"Game Lane is a product multipath mechanism",
		"A -> A+B -> B",
		"no ordinary kernel-TCP HOL",
	} { requireContains(t, constitution, want, "project constitution") }

	architecture := readRepoFile(t, "ARCHITECTURE.md")
	for _, want := range []string{
		"1..4 independent replaceable Transport Lanes",
		"one raw FakeTCP SYN lineage",
		"no FIN/RST/reconnect/new WBD payload SYN inside that lane",
		"packet/datagram payload without ordinary kernel-TCP HOL",
	} { requireContains(t, architecture, want, "architecture") }
}

func TestWindowsShippingProfileAndLifecycleUsePerLaneSameFlowMultipath(t *testing.T) {
	plan := readRepoFile(t, "internal/windowsruntime/plan.go")
	requireContains(t, plan, "ValidateProductTransportLaneCount(p.Lanes)", "Windows profile validation")
	requireContains(t, plan, "Game/weak-network policy may request 2..4", "Windows profile lane policy")

	controller := readRepoFile(t, "internal/windowsruntime/controller.go")
	if strings.Contains(controller, "run:reality-bootstrap") || strings.Contains(controller, "c.runner.Run(bootstrap)") {
		t.Fatal("Windows product controller must not create separate ordinary-TCP Reality bootstrap")
	}
	requireContains(t, controller, "BuildLaneBootstrap", "Windows controller same-flow bootstrap")

	lifecycle := readRepoFile(t, "internal/windowsruntime/controller_lifecycle.go")
	requireContains(t, lifecycle, "Wake recreates the configured 1..4 Transport Lanes", "Windows wake lifecycle")
	requireContains(t, lifecycle, "same-logical-ID make-before-break", "Windows replacement lifecycle")
	requireContains(t, lifecycle, "candidate lane %d Game promotion", "Windows candidate promotion")
	requireContains(t, lifecycle, "PromoteSameIDReplacement", "Windows generation fence")
}

func TestLinkServerCapsFourConcurrentPublicTransportsPerTunnel(t *testing.T) {
	body := readRepoFile(t, "cmd/wbd-link-server-mux/logical_tunnel.go")
	requireContains(t, body, "activeTunnelPeers", "LINK Logical Tunnel transport set")
	requireContains(t, body, "MaxProductPublicTransportLanes", "LINK Logical Tunnel transport limit")
	requireContains(t, body, "errTransportLaneLimit", "LINK Logical Tunnel fifth-transport limit")
	policy := readRepoFile(t, "internal/logicaltunnel/logicaltunnel.go")
	requireContains(t, policy, "MaxProductPublicTransportLanes = 4", "LINK transport ceiling")
}

func TestSameAssociationRealityLikeBootstrapRemainsProductPath(t *testing.T) {
	plan := readRepoFile(t, "internal/windowsruntime/plan.go")
	for _, want := range []string{
		"--reality-server-name",
		"--reality-route-key",
		"--reality-username",
		"--reality-password",
		"--reality-ticket-out",
	} { requireContains(t, plan, want, "Windows same-flow FakeTCP bootstrap") }

	adr11 := readRepoFile(t, "docs/architecture/ADR-0011-single-public-flow-reality-bootstrap.md")
	requireContains(t, adr11, "no FIN/RST/new WBD payload SYN", "ADR-0011")
}

func TestArtifactsCarryMatchingSubstantiveSourceSHAEvidence(t *testing.T) {
	linux := readRepoFile(t, ".github/workflows/linux-server-release.yml")
	requireContains(t, linux, `WBD_SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}`, "Linux release workflow")

	windows := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")
	requireContains(t, windows, `WBD_SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}`, "Windows portable workflow")
	requireContains(t, windows, `-source-sha $env:WBD_SOURCE_SHA`, "Windows portable stage")
	requireContains(t, windows, `name: wbd-windows-portable-${{ env.WBD_SOURCE_SHA }}`, "Windows portable artifact identity")
	if strings.Contains(windows, "source_sha=$env:GITHUB_SHA") || strings.Contains(windows, "-source-sha $env:GITHUB_SHA") {
		t.Fatal("Windows artifact source identity must not use pull_request merge GITHUB_SHA")
	}
}
