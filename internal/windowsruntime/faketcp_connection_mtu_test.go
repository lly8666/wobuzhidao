package windowsruntime

import "testing"

func TestBuildFakeTCPCommandPassesProfileConnectionMTU(t *testing.T) {
	p := testProfile()
	p.MTU = 1300
	cmd, err := BuildFakeTCPCommand(p, testUnderlay())
	if err != nil {
		t.Fatal(err)
	}
	if !argPair(cmd.Args, "--connection-mtu", "1300") {
		t.Fatalf("FakeTCP command lost product connection MTU: %v", cmd.Args)
	}
}
