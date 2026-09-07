package windowsgui

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

// The Windows profile and Linux server configuration use the same inner-IP MTU
// contract. Keep the Windows product default explicit so a future default drift
// cannot silently produce a client/server LINK mismatch.
func TestProductInnerMTUDefaultIs1360(t *testing.T) {
	if windowsruntime.DefaultTunnelMTU != 1360 {
		t.Fatalf("Windows inner MTU default=%d want=1360", windowsruntime.DefaultTunnelMTU)
	}
}
