package windowsruntime

import "testing"

func TestNextFakeTCPSourcePortStaysDynamicAndDoesNotImmediatelyReuse(t *testing.T) {
	seen := make(map[uint16]struct{}, 512)
	for i := 0; i < 512; i++ {
		port := nextFakeTCPSourcePort()
		if port < windowsDynamicPortMin || int(port) >= windowsDynamicPortMin+windowsDynamicPortCount {
			t.Fatalf("source port %d outside Windows dynamic range", port)
		}
		if _, ok := seen[port]; ok {
			t.Fatalf("source port %d reused within %d consecutive Connect allocations", port, i+1)
		}
		seen[port] = struct{}{}
	}
}

func TestBuildFakeTCPCommandUsesSessionSourcePort(t *testing.T) {
	u := testUnderlay()
	u.SourcePort = 54321
	cmd, err := BuildFakeTCPCommand(testProfile(), u)
	if err != nil {
		t.Fatal(err)
	}
	if !argPair(cmd.Args, "--source", "192.0.2.20:54321") {
		t.Fatalf("per-session source port missing from FakeTCP command: %v", cmd.Args)
	}
}

func TestUnderlayRejectsNonDynamicExplicitSourcePort(t *testing.T) {
	u := testUnderlay()
	u.SourcePort = 41001
	if err := u.Validate(); err == nil {
		t.Fatal("explicit product source port outside dynamic range must be rejected")
	}
}
