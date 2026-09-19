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
	tests := []struct {
		inner int
		want  int
	}{
		{inner: 1360, want: 1400},
		{inner: 1300, want: 1340},
	}

	for _, tt := range tests {
		got, err := gamepath.LinkPlaintextMTU(tt.inner)
		if err != nil {
			t.Fatalf("inner MTU %d: %v", tt.inner, err)
		}
		if got != tt.want {
			t.Fatalf("LINK plaintext MTU=%d want=%d for inner MTU %d", got, tt.want, tt.inner)
		}
	}
}
