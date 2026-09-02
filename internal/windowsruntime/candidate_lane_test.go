package windowsruntime

import (
	"errors"
	"testing"
)

func TestCandidatePublicBootstrapIsForbiddenBeforeSecondSYN(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	u:=testUnderlay();u.SourcePort=windowsDynamicPortMin+99
	if _,err:=BuildCandidateLaneBootstrap(p,u,1);!errors.Is(err,ErrOverlappingPublicFlow){t.Fatalf("candidate bootstrap err=%v",err)}
	if _,err:=BuildCandidateLaneBootstrapSlot(p,u,1,makeBeforeBreakCandidateSlot);!errors.Is(err,ErrOverlappingPublicFlow){t.Fatalf("candidate slot bootstrap err=%v",err)}
}

func TestCandidatePublicPlanIsForbidden(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	if _,err:=BuildCandidateLanePlan(p,LaneBootstrap{});!errors.Is(err,ErrOverlappingPublicFlow){t.Fatalf("candidate plan err=%v",err)}
	if _,err:=BuildCandidateLanePlanSlot(p,LaneBootstrap{},makeBeforeBreakCandidateSlot);!errors.Is(err,ErrOverlappingPublicFlow){t.Fatalf("candidate slot plan err=%v",err)}
}
