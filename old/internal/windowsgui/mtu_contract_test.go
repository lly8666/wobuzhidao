package windowsgui

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

// The Settings MTU field is the maximum connection/carrier MTU. Runtime
// planning derives the smaller LINK and Wintun MTUs from enabled transport
// wrappers; the GUI must never label or freeze this value as inner-IP MTU.
func TestProductConnectionMTUDefaultIs1500(t *testing.T) {
	if windowsruntime.DefaultConnectionMTU != 1500 {
		t.Fatalf("Windows connection MTU default=%d want=1500", windowsruntime.DefaultConnectionMTU)
	}
	if windowsruntime.DefaultTunnelMTU != windowsruntime.DefaultConnectionMTU {
		t.Fatalf("compatibility MTU alias=%d want connection default=%d", windowsruntime.DefaultTunnelMTU, windowsruntime.DefaultConnectionMTU)
	}
}
