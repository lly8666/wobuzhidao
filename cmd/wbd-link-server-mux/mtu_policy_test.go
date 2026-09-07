package main

import "testing"

func TestLinkPolicyForInnerMTU(t *testing.T) {
	policy, err := linkPolicyForInnerMTU(defaultInnerMTU)
	if err != nil {
		t.Fatalf("default policy: %v", err)
	}
	if policy.MinMTU != 1400 || policy.MaxMTU != 1400 {
		t.Fatalf("default inner MTU %d policy = %d..%d, want LINK 1400", defaultInnerMTU, policy.MinMTU, policy.MaxMTU)
	}

	policy, err = linkPolicyForInnerMTU(1280)
	if err != nil {
		t.Fatalf("1280 policy: %v", err)
	}
	if policy.MinMTU != 1320 || policy.MaxMTU != 1320 {
		t.Fatalf("1280 inner MTU policy = %d..%d, want LINK 1320", policy.MinMTU, policy.MaxMTU)
	}

	policy, err = linkPolicyForInnerMTU(1300)
	if err != nil {
		t.Fatalf("1300 policy: %v", err)
	}
	if policy.MinMTU != 1340 || policy.MaxMTU != 1340 {
		t.Fatalf("1300 inner MTU policy = %d..%d, want LINK 1340", policy.MinMTU, policy.MaxMTU)
	}
}

func TestLinkPolicyForInnerMTURejectsOutOfRange(t *testing.T) {
	for _, mtu := range []int{575, 1461, 1501} {
		if _, err := linkPolicyForInnerMTU(mtu); err == nil {
			t.Fatalf("mtu=%d unexpectedly accepted", mtu)
		}
	}
}
