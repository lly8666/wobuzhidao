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

func TestLinuxBundleCarriesSourceSHAAndSingleFlowOperatorContract(t *testing.T) {
	body := readRepoFile(t, "scripts/build_linux_server_bundle.sh")
	requireContains(t, body, `> "$root/SOURCE_SHA"`, "Linux bundle builder")
	requireContains(t, body, "one raw wbd-faketcp-mux public listener", "Linux bundle README")
	requireContains(t, body, "no parallel kernel TCP Reality front", "Linux bundle README")
	requireContains(t, body, "diagnostic/reference only", "Linux bundle README")
}

func TestWindowsPortableCarriesMatchingSourceSHAEvidence(t *testing.T) {
	body := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")
	requireContains(t, body, `source_sha=$env:GITHUB_SHA`, "Windows portable workflow")
	requireContains(t, body, `"$payload\SOURCE_SHA"`, "Windows embedded payload")
	requireContains(t, body, `${{ runner.temp }}\SOURCE_SHA`, "Windows artifact sidecar")
}
