package windowsruntime

import (
	"strings"
	"testing"
)

func TestBuildPlanKeepaliveProfileContract(t *testing.T) {
	ticket := strings.Repeat("ab", 32)
	p := testProfile()
	plan, err := BuildPlan(p, testUnderlay(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if argValue(plan.Link.Args, "-keepalive") != "" {
		t.Fatalf("omitted keepalive must preserve link proxy default: %v", plan.Link.Args)
	}

	zero := 0
	p.KeepaliveSeconds = &zero
	plan, err = BuildPlan(p, testUnderlay(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if got := argValue(plan.Link.Args, "-keepalive"); got != "0s" {
		t.Fatalf("keepalive=0 arg=%q want 0s; args=%v", got, plan.Link.Args)
	}

	positive := 27
	p.KeepaliveSeconds = &positive
	plan, err = BuildPlan(p, testUnderlay(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if got := argValue(plan.Link.Args, "-keepalive"); got != "27s" {
		t.Fatalf("keepalive=27 arg=%q want 27s; args=%v", got, plan.Link.Args)
	}
}

func TestProfileRejectsNegativeKeepalive(t *testing.T) {
	negative := -1
	p := testProfile()
	p.KeepaliveSeconds = &negative
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "keepalive") {
		t.Fatalf("negative keepalive error=%v", err)
	}
}

func argValue(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
