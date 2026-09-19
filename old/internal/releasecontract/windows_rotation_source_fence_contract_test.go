package releasecontract

import (
	"strings"
	"testing"
)

func TestWindowsTunnelLeaseSourceFenceContract(t *testing.T) {
	plan := readRepoFile(t, "internal/windowsruntime/plan.go")
	requireContains(t, plan, `"-expected-source-ipv4", tunnelPrefix.Addr().String()`, "authenticated Logical Tunnel lease passed to wbd-tun")

	tunMain := readRepoFile(t, "cmd/wbd-tun/main.go")
	for _, want := range []string{`expected-source-ipv4`, `WBD_TUN_SOURCE_IPV4_FENCE`, `WBD_TUN_SOURCE_IPV4_DROP`, `if source != e.expected`} {
		requireContains(t, tunMain, want, "client-side lease source fail-closed fence")
	}
	filter := strings.Index(tunMain, "if source != e.expected")
	writeFn := strings.Index(tunMain, "func (e *sourceIPv4Endpoint) WritePacket")
	if writeFn < 0 {
		t.Fatal("source IPv4 endpoint WritePacket function missing")
	}
	returnPacket := strings.LastIndex(tunMain[:writeFn], "return n, nil")
	if filter < 0 || returnPacket <= filter {
		t.Fatalf("source lease filter must run before a TUN packet is returned to the bridge: filter=%d return=%d", filter, returnPacket)
	}
}

func TestWindowsLaneRotationRangeContract(t *testing.T) {
	profile := readRepoFile(t, "internal/windowsgui/config.go")
	for _, want := range []string{`lane_rotation_min_seconds`, `lane_rotation_max_seconds`, `DefaultLaneRotationMinSeconds`, `DefaultLaneRotationMaxSeconds`} {
		requireContains(t, profile, want, "Windows profile lane rotation bounds")
	}
	idle := readRepoFile(t, "internal/windowsruntime/controller_idle.go")
	for _, want := range []string{`chooseLaneAgeDeadlineWithin`, `currentLaneRotationBounds`, `ages.reconcileWithin`, `c.ReplaceLane(laneID)`} {
		requireContains(t, idle, want, "lane age rotation remains per-lane make-before-break")
	}
	vm := readRepoFile(t, ".github/workflows/windows-runtime-vm.yml")
	requireContains(t, vm, `go test ./internal/windowsruntime ./internal/windowsgui ./cmd/wbd-tun`, "hosted Windows VM qualification")
}
