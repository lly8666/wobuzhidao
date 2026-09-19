package gamepath

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/control"
)

func TestLinkPlaintextMTUUsesPrivateEnvelopeBudget(t *testing.T) {
	if got := DatagramOverhead(); got != 40 {
		t.Fatalf("Game/WBDP plaintext overhead=%d want=40", got)
	}
	for inner, want := range map[int]int{1280: 1320, 1300: 1340, 1360: 1400, 1460: 1500} {
		got, err := LinkPlaintextMTU(inner)
		if err != nil {
			t.Fatalf("inner MTU %d: %v", inner, err)
		}
		if got != want {
			t.Fatalf("inner MTU %d -> LINK %d want %d", inner, got, want)
		}
	}
}

func TestInnerMTUBoundsAccountForEnvelope(t *testing.T) {
	minMTU, maxMTU, err := InnerMTUBounds()
	if err != nil {
		t.Fatal(err)
	}
	wantMax := int(control.MaxLinkMTU) - DatagramOverhead()
	if minMTU != MinimumInnerMTU || maxMTU != wantMax {
		t.Fatalf("inner bounds=%d..%d want=%d..%d", minMTU, maxMTU, MinimumInnerMTU, wantMax)
	}
	for _, tc := range []struct {
		mtu     int
		wantErr bool
	}{
		{mtu: minMTU - 1, wantErr: true},
		{mtu: minMTU},
		{mtu: maxMTU},
		{mtu: maxMTU + 1, wantErr: true},
	} {
		_, err := LinkPlaintextMTU(tc.mtu)
		if tc.wantErr && err == nil {
			t.Fatalf("inner MTU %d unexpectedly accepted", tc.mtu)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("inner MTU %d unexpectedly rejected: %v", tc.mtu, err)
		}
	}
}
