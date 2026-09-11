package main

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
)

// The operator-visible MTU belongs to the client tunnel interface. A shared
// server must accept every valid Game inner MTU and freeze that client's
// derived LINK plaintext MTU for the lifetime of the association. The server's
// own default must not force every client to use the same MTU.
func TestServerLinkPolicyAcceptsClientSelectedInnerMTU(t *testing.T) {
	policy, err := linkPolicyForInnerMTU(defaultInnerMTU)
	if err != nil {
		t.Fatalf("build server LINK policy: %v", err)
	}

	for _, innerMTU := range []int{576, 1280, 1300, 1360, 1460} {
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
}

func TestServerEchoesClientSelectedMTUWithoutRewrite(t *testing.T) {
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
