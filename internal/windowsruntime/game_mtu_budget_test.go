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

// Freeze the user-visible MTU contract shared by the Windows profile and Linux
// WBD_MTU setting. The configured value is the complete inner IP packet; Game
// adds its 40-byte private envelope before LINK, while DTLS/FakeTCP/outer IP/L2
// overhead remains outside this user-facing MTU.
func TestDefaultInnerMTUProductContract(t *testing.T) {
	const (
		wantDefaultInner = 1360
		wantGameEnvelope = 40
		wantDefaultLink  = 1400
		wantMaxInner     = 1460
		wantMaxLink      = 1500
	)

	if DefaultTunnelMTU != wantDefaultInner {
		t.Fatalf("default inner MTU=%d want=%d", DefaultTunnelMTU, wantDefaultInner)
	}
	envelope := dataplane.HeaderLen + gamelane.HeaderSize
	if envelope != wantGameEnvelope {
		t.Fatalf("Game private envelope=%d want=%d", envelope, wantGameEnvelope)
	}
	linkMTU, err := gameLinkPlaintextMTU(DefaultTunnelMTU)
	if err != nil {
		t.Fatalf("default inner MTU rejected: %v", err)
	}
	if linkMTU != wantDefaultLink {
		t.Fatalf("default inner MTU=%d -> LINK MTU=%d want=%d", DefaultTunnelMTU, linkMTU, wantDefaultLink)
	}
	_, maxInner, err := gameInnerMTUBounds()
	if err != nil {
		t.Fatal(err)
	}
	if maxInner != wantMaxInner {
		t.Fatalf("maximum inner MTU=%d want=%d", maxInner, wantMaxInner)
	}
	maxLink, err := gameLinkPlaintextMTU(maxInner)
	if err != nil {
		t.Fatalf("maximum inner MTU rejected: %v", err)
	}
	if maxLink != wantMaxLink {
		t.Fatalf("maximum inner MTU=%d -> LINK MTU=%d want=%d", maxInner, maxLink, wantMaxLink)
	}
}
