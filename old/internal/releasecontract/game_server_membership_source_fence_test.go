package releasecontract

import (
	"strings"
	"testing"
)

func TestLinuxServerReleaseTracksGameMembershipSource(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/linux-server-release.yml")
	if got := strings.Count(workflow, `cmd/wbd-game-lane-server/**`); got < 2 {
		t.Fatalf("linux server release must rebuild for Game membership changes on PR and push: occurrences=%d", got)
	}

	build := readRepoFile(t, "scripts/build_linux_server_bundle.sh")
	for _, want := range []string{
		`./cmd/wbd-game-lane-server`,
		`wbd-game-lane-server:./cmd/wbd-game-lane-server`,
		`SOURCE_SHA`,
	} {
		requireContains(t, build, want, "exact-source Linux Game server bundle")
	}
}

func TestExactSourcePhysicalPairRebuildTrigger(t *testing.T) {
	// This release-contract package is intentionally in the Windows portable
	// workflow path set and under Linux server's internal/** path. Any substantive
	// source-fence contract change therefore causes both platform artifacts to be
	// rebuilt from the same commit, which is required before physical qualification.
	windows := readRepoFile(t, ".github/workflows/windows-portable-bundle.yml")
	requireContains(t, windows, `internal/releasecontract/**`, "Windows exact-source bundle trigger")
	linux := readRepoFile(t, ".github/workflows/linux-server-release.yml")
	requireContains(t, linux, `internal/**`, "Linux exact-source bundle trigger")
}
