package main

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/gamepath"
)

func TestLinkPolicyForInnerMTUUsesGameCompatibleRange(t *testing.T) {
	minInner, maxInner, err := gamepath.InnerMTUBounds()
	if err != nil {
		t.Fatal(err)
	}
	wantMin, err := gamepath.LinkPlaintextMTU(minInner)
	if err != nil {
		t.Fatal(err)
	}
	wantMax, err := gamepath.LinkPlaintextMTU(maxInner)
	if err != nil {
		t.Fatal(err)
	}

	for _, serverDefault := range []int{1280, 1300, defaultInnerMTU, maxInner} {
		policy, err := linkPolicyForInnerMTU(serverDefault)
		if err != nil {
			t.Fatalf("server default inner MTU %d: %v", serverDefault, err)
		}
		if policy.MinMTU != uint16(wantMin) || policy.MaxMTU != uint16(wantMax) {
			t.Fatalf("server default inner MTU %d policy=%d..%d want shared Game LINK range=%d..%d", serverDefault, policy.MinMTU, policy.MaxMTU, wantMin, wantMax)
		}
	}
}

func TestLinkPolicyForInnerMTURejectsOutOfRange(t *testing.T) {
	for _, mtu := range []int{575, 1461, 1501} {
		if _, err := linkPolicyForInnerMTU(mtu); err == nil {
			t.Fatalf("mtu=%d unexpectedly accepted", mtu)
		}
	}
}
