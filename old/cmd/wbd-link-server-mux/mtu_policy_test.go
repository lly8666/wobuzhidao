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

	for _, serverDefault := range []int{minInner, 1280, 1300, defaultInnerMTU, maxInner} {
		policy, err := linkPolicyForInnerMTU(serverDefault)
		if err != nil {
			t.Fatalf("server default inner MTU %d: %v", serverDefault, err)
		}
		if policy.MinMTU != uint16(wantMin) || policy.MaxMTU != uint16(wantMax) {
			t.Fatalf("server default inner MTU %d policy=%d..%d want shared Game LINK range=%d..%d", serverDefault, policy.MinMTU, policy.MaxMTU, wantMin, wantMax)
		}
	}
}

func TestLinkPolicyForInnerMTUProtocolBoundary(t *testing.T) {
	minInner, maxInner, err := gamepath.InnerMTUBounds()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mtu     int
		wantErr bool
	}{
		{mtu: minInner - 1, wantErr: true},
		{mtu: minInner},
		{mtu: maxInner},
		{mtu: maxInner + 1, wantErr: true},
	} {
		_, err := linkPolicyForInnerMTU(tc.mtu)
		if tc.wantErr && err == nil {
			t.Fatalf("mtu=%d unexpectedly accepted", tc.mtu)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("mtu=%d unexpectedly rejected: %v", tc.mtu, err)
		}
	}
}
