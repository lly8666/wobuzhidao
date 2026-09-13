package windowsruntime

import "testing"

func TestConfiguredMTUIsConnectionCeiling(t *testing.T) {
	const connectionMTU = 1500

	on, err := gameConnectionMTUBudget(connectionMTU, "20:20")
	if err != nil {
		t.Fatal(err)
	}
	if on.ConnectionMTU != connectionMTU || on.CarrierPayloadMTU != 1460 || on.DTLSPlaintextMTU != 1428 || on.LinkPlaintextMTU != 1372 || on.InnerMTU != 1332 {
		t.Fatalf("FEC+Game budget=%+v", on)
	}

	off, err := gameConnectionMTUBudget(connectionMTU, "off")
	if err != nil {
		t.Fatal(err)
	}
	if off.LinkPlaintextMTU != 1428 || off.InnerMTU != 1388 {
		t.Fatalf("Game-only budget=%+v", off)
	}
	if off.InnerMTU <= on.InnerMTU {
		t.Fatalf("disabling FEC should return its MTU budget: on=%d off=%d", on.InnerMTU, off.InnerMTU)
	}
}

func TestDefaultConnectionMTUProductContract(t *testing.T) {
	if DefaultConnectionMTU != 1500 {
		t.Fatalf("default connection MTU=%d want=1500", DefaultConnectionMTU)
	}
	if DefaultTunnelMTU != DefaultConnectionMTU {
		t.Fatalf("deprecated DefaultTunnelMTU alias=%d want=%d", DefaultTunnelMTU, DefaultConnectionMTU)
	}
	b, err := gameConnectionMTUBudget(DefaultConnectionMTU, "20:20")
	if err != nil {
		t.Fatal(err)
	}
	if b.GameOverhead != 40 || b.FECOverhead != 56 || b.DTLSRecordReserve != 32 {
		t.Fatalf("unexpected product overhead contract: %+v", b)
	}
}

func TestUnsupportedFECRejectedByMTUBudget(t *testing.T) {
	if _, err := gameConnectionMTUBudget(1500, "auto"); err == nil {
		t.Fatal("unsupported FEC mode unexpectedly accepted")
	}
}
