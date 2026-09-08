package main

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/gamepath"
)

// The product-facing Game MTU is the inner IP MTU. wbd-link-proxy is a
// lower-level component whose -mtu flag receives LINK plaintext MTU instead.
// Keep this cross-layer contract explicit so the +40 Game envelope adjustment
// remains centralized in gamepath.LinkPlaintextMTU and is applied exactly once.
func TestGameInnerMTUContractFeedsLinkProxyPlaintextMTU(t *testing.T) {
	got, err := gamepath.LinkPlaintextMTU(1360)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1400 {
		t.Fatalf("LINK plaintext MTU=%d want=1400 for inner MTU 1360", got)
	}
}
