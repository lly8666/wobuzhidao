package gamepath

import "testing"

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
	if minMTU != 576 || maxMTU != 1460 {
		t.Fatalf("inner bounds=%d..%d want=576..1460", minMTU, maxMTU)
	}
	for _, mtu := range []int{575, 1461} {
		if _, err := LinkPlaintextMTU(mtu); err == nil {
			t.Fatalf("inner MTU %d unexpectedly accepted", mtu)
		}
	}
}
