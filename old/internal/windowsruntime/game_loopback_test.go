package windowsruntime

import (
	"net"
	"net/netip"
	"testing"
)

func commandArgValue(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func TestBuildMultiLanePlanAvoidsOccupiedGameLoopbackPair(t *testing.T) {
	loopback := net.IPv4(127, 0, 0, 1)
	blockedListen, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: defaultGameListenPort})
	if err != nil {
		t.Skipf("default Game listen port already unavailable: %v", err)
	}
	defer blockedListen.Close()
	blockedControl, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: defaultGameControlPort})
	if err != nil {
		t.Skipf("default Game control port already unavailable: %v", err)
	}
	defer blockedControl.Close()

	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 1
	tunnel := testAuthenticatedTunnel()
	plan, err := BuildMultiLanePlan(p, []LaneBootstrap{authenticatedLane(t, p, 1, windowsDynamicPortMin+1, tunnel)})
	if err != nil {
		t.Fatal(err)
	}

	gameListen := commandArgValue(plan.Game.Args, "-listen")
	if gameListen == "127.0.0.1:48101" || gameListen == "" {
		t.Fatalf("Game listen did not move off occupied default: %q", gameListen)
	}
	if plan.GameControl == "127.0.0.1:48102" || plan.GameControl == "" {
		t.Fatalf("Game control did not move off occupied default: %q", plan.GameControl)
	}
	if gameListen != commandArgValue(plan.TUN.Args, "-transport") {
		t.Fatalf("TUN transport=%q want selected Game listen=%q", commandArgValue(plan.TUN.Args, "-transport"), gameListen)
	}
	listenAddr, err := netip.ParseAddrPort(gameListen)
	if err != nil || !listenAddr.Addr().IsLoopback() || listenAddr.Port() == 0 {
		t.Fatalf("selected Game listen is not usable loopback: %q err=%v", gameListen, err)
	}
	controlAddr, err := netip.ParseAddrPort(plan.GameControl)
	if err != nil || !controlAddr.Addr().IsLoopback() || controlAddr.Port() == 0 || controlAddr == listenAddr {
		t.Fatalf("selected Game control is invalid: %q err=%v", plan.GameControl, err)
	}
}
