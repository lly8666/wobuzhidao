package windowsruntime

import (
	"slices"
	"strings"
	"testing"
)

func testProfile() Profile {
	return Profile{
		BinDir:      `C:\Program Files\WBD`,
		ServerFront: "198.51.100.10:40443",
		ServerName:  "front.example",
		RouteKey:    "0123456789abcdef",
		Username:    "solo",
		Password:    "shared-password",
		ServerRaw:   "198.51.100.10:40000",
		FEC:         "20:20",
		IfName:      "WBD",
		MTU:         1400,
		RouteMode:   "Full",
		TunnelIPv4:  "10.66.0.2/30",
		TicketPath:  `C:\ProgramData\WBD\ticket.tmp`,
		RouteState:  `C:\ProgramData\WBD\route-state.json`,
	}
}

func testUnderlay() Underlay {
	return Underlay{
		SourceIP:     "192.0.2.20",
		PacketDevice: `\Device\NPF_{01234567-89AB-CDEF-0123-456789ABCDEF}`,
		SourceMAC:    "00:11:22:33:44:55",
		NextHopMAC:   "66:77:88:99:aa:bb",
	}
}

func TestBuildPlanUsesFrozenWindowsStack(t *testing.T) {
	p, err := BuildPlan(testProfile(), testUnderlay(), strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if p.FakeTCP.Name != "faketcp" || p.DTLS.Name != "dtls" || p.Link.Name != "link" || p.TUN.Name != "tun" {
		t.Fatalf("unexpected runtime commands: %+v", p)
	}
	if !slices.Contains(p.FakeTCP.Args, "legacy") {
		t.Fatalf("FakeTCP release recovery not pinned: %v", p.FakeTCP.Args)
	}
	if !slices.Contains(p.FakeTCP.Args, testUnderlay().PacketDevice) ||
		!slices.Contains(p.FakeTCP.Args, testUnderlay().SourceMAC) ||
		!slices.Contains(p.FakeTCP.Args, testUnderlay().NextHopMAC) {
		t.Fatalf("Npcap underlay identity missing: %v", p.FakeTCP.Args)
	}
	if got := p.DTLS.Args; !slices.Equal(got, []string{"client", "46101", "127.0.0.1", "45101", "none", "none"}) {
		t.Fatalf("DTLS client contract = %v", got)
	}
	if !slices.Contains(p.Link.Args, "20:20") || !slices.Contains(p.Link.Args, "1") {
		t.Fatalf("immutable LINK settings missing: %v", p.Link.Args)
	}
	if !slices.Contains(p.RouteApply.Args, "198.51.100.10") {
		t.Fatalf("raw endpoint underlay escape missing: %v", p.RouteApply.Args)
	}
}

func TestPlanStartAndStopOrder(t *testing.T) {
	p, err := BuildPlan(testProfile(), testUnderlay(), strings.Repeat("01", 32))
	if err != nil {
		t.Fatal(err)
	}
	start := p.StartSequence()
	wantStart := []string{"faketcp", "dtls", "link", "tun", "route-apply"}
	if got := commandNames(start); !slices.Equal(got, wantStart) {
		t.Fatalf("start order = %v want %v", got, wantStart)
	}
	stop := p.StopSequence()
	wantStop := []string{"route-cleanup", "tun", "link", "dtls", "faketcp"}
	if got := commandNames(stop); !slices.Equal(got, wantStop) {
		t.Fatalf("stop order = %v want %v", got, wantStop)
	}
}

func TestProfileRejectsUnfrozenOrUnsafeSettings(t *testing.T) {
	cases := []struct {
		name string
		mutate func(*Profile)
	}{
		{"auto-fec", func(p *Profile) { p.FEC = "auto" }},
		{"bad-route-key", func(p *Profile) { p.RouteKey = "short" }},
		{"hostname-raw", func(p *Profile) { p.ServerRaw = "example.com:40000" }},
		{"split-without-prefix", func(p *Profile) { p.RouteMode = "Split"; p.Prefix4 = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testProfile()
			tc.mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("expected validation failure: %+v", p)
			}
		})
	}
}

func commandNames(commands []Command) []string {
	out := make([]string, 0, len(commands))
	for _, cmd := range commands {
		out = append(out, cmd.Name)
	}
	return out
}
