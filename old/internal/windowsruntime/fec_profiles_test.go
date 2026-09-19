package windowsruntime

import (
	"fmt"
	"strings"
	"testing"
)

func validFECProfileForTest(fec string) Profile {
	return Profile{
		BinDir: `C:\\wbd`, ServerFront: "203.0.113.10:443", ServerName: "example.test",
		RouteKey: strings.Repeat("k", 16), Username: "u", Password: "p", ServerRaw: "203.0.113.10:443",
		FEC: fec, MTU: 1500, RouteMode: RouteFull, DNSMode: DNSSystem,
		InstallationID: strings.Repeat("a", 32), TicketPath: `C:\\state\\ticket`,
		TunnelConfigPath: `C:\\state\\tunnel.json`, RouteState: `C:\\state\\route.json`,
	}
}

func TestProfileAcceptsFixedFECMatrix(t *testing.T) {
	for _, parity := range []int{4, 8, 10, 12, 16, 20} {
		fec := fmt.Sprintf("20:%d", parity)
		if err := validFECProfileForTest(fec).Validate(); err != nil {
			t.Fatalf("%s: %v", fec, err)
		}
		if _, err := gameConnectionMTUBudget(1500, fec); err != nil {
			t.Fatalf("budget %s: %v", fec, err)
		}
	}
	if err := validFECProfileForTest("20:9").Validate(); err == nil {
		t.Fatal("20:9 unexpectedly accepted")
	}
}
