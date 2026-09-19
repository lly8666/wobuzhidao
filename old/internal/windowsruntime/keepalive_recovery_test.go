package windowsruntime

import (
	"errors"
	"testing"
)

func TestKeepaliveZeroUsesDormantOnlyForAuthoritativeLinkExit(t *testing.T) {
	plan := livenessLanePlan(1)
	zero := 0
	profile := testProfile()
	profile.KeepaliveSeconds = &zero

	for _, tc := range []struct {
		name string
		exit LaneProcessExit
		want bool
	}{
		{name: "link-clean", exit: LaneProcessExit{Name: plan.Link.Name}, want: true},
		{name: "link-error", exit: LaneProcessExit{Name: plan.Link.Name, Err: errors.New("server idle close")}, want: true},
		{name: "dtls-error", exit: LaneProcessExit{Name: plan.DTLS.Name, Err: errors.New("dtls failed")}, want: false},
		{name: "faketcp-error", exit: LaneProcessExit{Name: plan.FakeTCP.Name, Err: errors.New("faketcp failed")}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDormantOnKeepaliveZeroExit(profile, plan, tc.exit); got != tc.want {
				t.Fatalf("shouldDormant=%v want=%v exit=%+v", got, tc.want, tc.exit)
			}
		})
	}

	profile.KeepaliveSeconds = nil
	if shouldDormantOnKeepaliveZeroExit(profile, plan, LaneProcessExit{Name: plan.Link.Name}) {
		t.Fatal("omitted/default keepalive unexpectedly changed exit recovery policy")
	}
	positive := 15
	profile.KeepaliveSeconds = &positive
	if shouldDormantOnKeepaliveZeroExit(profile, plan, LaneProcessExit{Name: plan.Link.Name}) {
		t.Fatal("positive keepalive unexpectedly changed exit recovery policy")
	}
}
