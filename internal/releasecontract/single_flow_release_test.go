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

func TestWindowsProductConnectDoesNotRunLegacyRealityBootstrap(t *testing.T) {
	body := readRepoFile(t, "internal/windowsruntime/controller.go")
	if strings.Contains(body, "reality-bootstrap") || strings.Contains(body, "BuildBootstrap(") {
		t.Fatal("Windows product controller must not create a second public Reality bootstrap connection")
	}
	requireContains(t, body, "BuildFakeTCPCommand", "Windows controller")
	requireContains(t, body, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", "Windows controller")
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
	requireContains(t, body, `source_sha=$env:WBD_SOURCE_SHA`, "Windows portable workflow")
	requireContains(t, body, `"$payload\SOURCE_SHA"`, "Windows embedded payload")
	requireContains(t, body, `${{ runner.temp }}\SOURCE_SHA`, "Windows artifact sidecar")
	if strings.Contains(body, "source_sha=$env:GITHUB_SHA") {
		t.Fatal("Windows artifact source identity must not use pull_request merge GITHUB_SHA")
	}
}
