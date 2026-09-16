package releasecontract

import (
	"strings"
	"testing"
)

// idle_timeout=0 is an explicit no-expiry policy. Keep the release gate tied
// to expiry semantics rather than backend scheduling speed so slow ARM runners
// cannot turn a correct lease policy into an unrelated UDP echo timeout.
func TestIdleZeroDeterministicReleaseContract(t *testing.T) {
	test := readRepoFile(t, "cmd/wbd-link-server-mux/idle_timeout_disabled_test.go")

	requireContains(t, test, `idleTimeout: 0`, "zero idle timeout must remain the disabled-expiry policy")
	requireContains(t, test, `s.expirePeers(now.Add(24 * time.Hour))`, "zero idle timeout must survive a 24-hour expiry sweep")
	requireContains(t, test, `s.expirePeers(now.Add(48 * time.Hour))`, "zero idle timeout must survive repeated expiry sweeps")
	requireContains(t, test, `waitPlaneLen(t, s, 1)`, "zero idle timeout sweep must retain the live session")

	if strings.Contains(test, `exchange(t, c, s.Addr()`) {
		t.Fatal("idle-zero expiry contract must not depend on backend echo scheduling")
	}
}
