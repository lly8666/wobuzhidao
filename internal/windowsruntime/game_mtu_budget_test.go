package windowsruntime

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestGameLinkPlaintextMTUFollowsConfiguredInnerMTU(t *testing.T) {
	for _, innerMTU := range []int{576, 1280, 1300, DefaultTunnelMTU} {
		got, err := gameLinkPlaintextMTU(innerMTU)
		if err != nil {
			t.Fatalf("inner MTU %d rejected: %v", innerMTU, err)
		}
		want := innerMTU + dataplane.HeaderLen + gamelane.HeaderSize
		if got != want {
			t.Fatalf("inner MTU %d -> LINK plaintext MTU %d want %d", innerMTU, got, want)
		}
	}
}

func TestGameInnerMTUBoundsFollowCurrentLinkPolicy(t *testing.T) {
	minInner, maxInner, err := gameInnerMTUBounds()
	if err != nil {
		t.Fatal(err)
	}
	policy := control.CurrentLinkPolicy()
	if minInner < minimumGameInnerMTU {
		t.Fatalf("minimum inner MTU=%d below product minimum=%d", minInner, minimumGameInnerMTU)
	}
	if maxInner+dataplane.HeaderLen+gamelane.HeaderSize != int(policy.MaxMTU) {
		t.Fatalf("maximum inner MTU=%d does not consume LINK max=%d exactly", maxInner, policy.MaxMTU)
	}
	if _, err := gameLinkPlaintextMTU(maxInner); err != nil {
		t.Fatalf("derived maximum inner MTU %d rejected: %v", maxInner, err)
	}
	if _, err := gameLinkPlaintextMTU(maxInner + 1); err == nil {
		t.Fatalf("inner MTU %d above derived maximum unexpectedly accepted", maxInner+1)
	}
}
