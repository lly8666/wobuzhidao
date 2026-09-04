package releasecontract

import (
	"strings"
	"testing"
)

func TestWindowsPortableGameLaneRuntimeContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")

	if got := strings.Count(workflow, "cmd/wbd-game-lane-client/**"); got < 2 {
		t.Fatalf("Windows portable workflow must trigger for Game lane client changes on PR and push; count=%d", got)
	}
	for _, want := range []string{
		`go build -trimpath -ldflags "-s -w" -o build\windows-runtime\wbd-game-lane-client.exe .\cmd\wbd-game-lane-client`,
		`wbd-game-lane-client build failed`,
	} {
		requireContains(t, workflow, want, "Windows portable Game lane child build")
	}
	if got := strings.Count(workflow, "wbd-game-lane-client.exe"); got != 3 {
		t.Fatalf("Windows portable workflow must mention Game lane client exactly for build, manifest gate and extraction gate; occurrences=%d want=3", got)
	}
}

func TestWindowsGameLaneClientCrossCompiles(t *testing.T) {
	crossCompileWindowsCommand(t, "./cmd/wbd-game-lane-client", "wbd-game-lane-client.exe")
}
