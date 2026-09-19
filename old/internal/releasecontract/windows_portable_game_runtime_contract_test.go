package releasecontract

import (
	"strings"
	"testing"
)

func TestWindowsPortableGameLaneRuntimeContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")

	if got := strings.Count(workflow, "cmd/wbd-game-lane-client/**"); got < 2 {
		t.Fatalf("Windows install-directory workflow must trigger for Game lane client changes on PR and push; count=%d", got)
	}
	for _, want := range []string{
		`go build -trimpath -ldflags "-s -w" -o build\windows-runtime\wbd-game-lane-client.exe .\cmd\wbd-game-lane-client`,
		`wbd-game-lane-client build failed`,
		`'wbd.exe','wbd-reality-front.exe','wbd-faketcp.exe','wbd_dtls_shim.exe','wbd-link-proxy.exe','wbd-game-lane-client.exe'`,
	} {
		requireContains(t, workflow, want, "Windows install-directory Game lane runtime")
	}
	if got := strings.Count(workflow, "wbd-game-lane-client.exe"); got != 2 {
		t.Fatalf("Windows install-directory workflow must mention Game lane client exactly for build and required-files gate; occurrences=%d want=2", got)
	}
	requireContains(t, workflow, `schema = 'wbd-windows-install-directory/v1'`, "Windows install-directory manifest")
	requireContains(t, workflow, `extraction = $false`, "Windows install-directory no-extraction contract")
}

func TestWindowsGameLaneClientCrossCompiles(t *testing.T) {
	crossCompileWindowsCommand(t, "./cmd/wbd-game-lane-client", "wbd-game-lane-client.exe")
}
