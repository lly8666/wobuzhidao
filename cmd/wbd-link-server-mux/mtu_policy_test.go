package main

import "testing"

func TestLinkPolicyForInnerMTU(t *testing.T) {
	policy, err := linkPolicyForInnerMTU(defaultInnerMTU)
	if err != nil {
		t.Fatalf("default policy: %v", err)
	}
	if policy.MinMTU != defaultInnerMTU || policy.MaxMTU != defaultInnerMTU {
		t.Fatalf("default MTU policy = %d..%d, want %d", policy.MinMTU, policy.MaxMTU, defaultInnerMTU)
	}

	policy, err = linkPolicyForInnerMTU(1280)
	if err != nil {
		t.Fatalf("1280 policy: %v", err)
	}
	if policy.MinMTU != 1280 || policy.MaxMTU != 1280 {
		t.Fatalf("1280 MTU policy = %d..%d", policy.MinMTU, policy.MaxMTU)
	}
}

func TestLinkPolicyForInnerMTURejectsOutOfRange(t *testing.T) {
	for _, mtu := range []int{575, 1501} {
		if _, err := linkPolicyForInnerMTU(mtu); err == nil {
			t.Fatalf("mtu=%d unexpectedly accepted", mtu)
		}
	}
}
