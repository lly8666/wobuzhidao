package main

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

// A shared server accepts different client-selected MTUs as long as each one
// fits both the Game protocol range and this server's actual local carrier
// budget. The server default must not force every client to one MTU.
func TestServerLinkPolicyAcceptsClientSelectedInnerMTUWithinCarrier(t *testing.T) {
	old := serverConnectionMTU
	serverConnectionMTU = pathmtu.DefaultConnectionMTU
	defer func() { serverConnectionMTU = old }()
	policy, err := linkPolicyForInnerMTU(defaultInnerMTU)
	if err != nil {
		t.Fatalf("build server LINK policy: %v", err)
	}
	budget, err := pathmtu.Derive(serverConnectionMTU, pathmtu.Features{Game: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, innerMTU := range []int{576, 900, 1280, budget.InnerMTU} {
		linkMTU, err := gamepath.LinkPlaintextMTU(innerMTU)
		if err != nil {
			t.Fatalf("inner MTU %d -> LINK plaintext MTU: %v", innerMTU, err)
		}
		cfg := control.LinkConfig{
			FECMode:   control.FECOff,
			Scheduler: control.FECSchedulerNone,
			LaneCount: 1,
			MTU:       uint16(linkMTU),
		}
		if err := policy.Validate(cfg); err != nil {
			t.Fatalf("server default inner MTU %d rejected client inner MTU %d (LINK %d): %v", defaultInnerMTU, innerMTU, linkMTU, err)
		}
	}

	over, err := gamepath.LinkPlaintextMTU(budget.InnerMTU + 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.Validate(control.LinkConfig{FECMode: control.FECOff, Scheduler: control.FECSchedulerNone, LaneCount: 1, MTU: uint16(over)}); err == nil {
		t.Fatalf("inner MTU %d above carrier-derived ceiling unexpectedly accepted", budget.InnerMTU+1)
	}
}

func TestServerEchoesClientSelectedMTUWithoutRewrite(t *testing.T) {
	old := serverConnectionMTU
	serverConnectionMTU = pathmtu.DefaultConnectionMTU
	defer func() { serverConnectionMTU = old }()
	policy, err := linkPolicyForInnerMTU(defaultInnerMTU)
	if err != nil {
		t.Fatal(err)
	}
	linkMTU, err := gamepath.LinkPlaintextMTU(1300)
	if err != nil {
		t.Fatal(err)
	}
	cfg := control.LinkConfig{
		FECMode:   control.FECOff,
		Scheduler: control.FECSchedulerNone,
		LaneCount: 1,
		MTU:       uint16(linkMTU),
	}
	server, err := control.NewLinkServerSession(1, 1, nil, policy)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := control.MarshalLink(control.LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	replyWire, err := server.HandleWire(wire, 1)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := control.UnmarshalLink(replyWire)
	if err != nil {
		t.Fatal(err)
	}
	accept, ok := reply.(control.LinkAccept)
	if !ok {
		t.Fatalf("reply=%T %#v want LinkAccept for client-selected inner MTU 1300", reply, reply)
	}
	if accept.Config.MTU != uint16(linkMTU) {
		t.Fatalf("accepted LINK MTU=%d want client proposal=%d", accept.Config.MTU, linkMTU)
	}
}
