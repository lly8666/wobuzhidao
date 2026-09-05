package windowsruntime

import "testing"

func TestBuildPlanPinsAuthenticatedTunnelSourceIntoTUN(t *testing.T) {
	profile := Profile{BinDir:`C:\wbd`, ServerFront:"158.101.94.176:443", ServerRaw:"158.101.94.176:443", ServerName:"www.cloudflare.com", RouteKey:"0123456789abcdef", Username:"user", Password:"password", InstallationID:"0123456789abcdef0123456789abcdef", Lanes:1, TunnelIPv4:"10.66.0.1/32", TicketPath:`C:\state\ticket`, TunnelConfigPath:`C:\state\tunnel.json`, RouteState:`C:\state\route.json`}
	underlay := Underlay{SourceIP:"192.0.2.10", PacketDevice:`\Device\NPF_{01234567-89AB-CDEF-0123-456789ABCDEF}`, SourceMAC:"00:11:22:33:44:55", NextHopMAC:"66:77:88:99:aa:bb", SourcePort:50000}
	plan, err := BuildPlan(profile, underlay, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil { t.Fatal(err) }
	found := false
	for i := 0; i+1 < len(plan.TUN.Args); i++ {
		if plan.TUN.Args[i] == "-expected-source-ipv4" && plan.TUN.Args[i+1] == "10.66.0.1" { found = true; break }
	}
	if !found { t.Fatalf("TUN args missing authenticated source fence: %v", plan.TUN.Args) }
}
