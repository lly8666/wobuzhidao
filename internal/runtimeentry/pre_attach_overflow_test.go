package runtimeentry

import (
	"context"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// Exercise the actual post-detach, pre-publication handler branch, not a
// synthetic queue. Overload of one flow must not kill a shared endpoint.
func TestPreAttachOverflowIsBoundedFlowLocalLoss(t *testing.T) {
	for _, lifecycle := range []bool{false, true} {
		name := "server"
		if lifecycle { name = "lifecycle" }
		t.Run(name, func(t *testing.T) {
			emitted := 0
			table, err := faketcp.NewServerAssociationTable(4, func(faketcp.Segment) error {
				emitted++
				return nil
			})
			if err != nil { t.Fatal(err) }
			makeDetached := func(port uint16) (faketcp.ServerFlow, faketcp.Segment) {
				syn := faketcp.Segment{
					SrcIP: [4]byte{10, 0, 0, 1}, DstIP: [4]byte{10, 0, 0, 2},
					SrcPort: port, DstPort: 443, Seq: 100, Flags: faketcp.FlagSYN,
					Window: 65535,
				}
				assoc, err := table.AddSYN(syn, 9000, time.Second)
				if err != nil { t.Fatal(err) }
				t.Cleanup(func() { assoc.Close() })
				ack := syn
				ack.Seq, ack.Ack, ack.Flags = 101, 9001, faketcp.FlagACK
				if err := assoc.HandleHandshakeACK(ack); err != nil { t.Fatal(err) }
				boundary, err := assoc.PrepareTransition(1500)
				if err != nil { t.Fatal(err) }
				if _, err := assoc.DetachTransition(); err != nil { t.Fatal(err) }
				ack.Seq, ack.Flags, ack.Payload = boundary, faketcp.FlagACK|faketcp.FlagPSH, []byte{7, 8}
				return assoc.Flow(), ack
			}
			flow, seg := makeDetached(40000)
			other, otherSeg := makeDetached(40001)
			pending := make(map[faketcp.ServerFlow][]faketcp.Segment)
			started := map[faketcp.ServerFlow]bool{flow: true, other: true}
			server := &Server{cfg: ServerConfig{ListenPort: 443}, table: table, pending: pending, started: started}
			life := &LifecycleServer{cfg: LifecycleServerConfig{ServerConfig: ServerConfig{ListenPort: 443}}, table: table, pending: pending, started: started}
			handle := server.handleSegment
			if lifecycle { handle = life.handleSegment }
			beforeEmit := emitted
			for i := 0; i < faketcp.MaxBootstrapPendingChunks+17; i++ {
				seg.Seq++
				if err := handle(context.Background(), seg, time.Now()); err != nil {
					t.Fatalf("flow overflow escaped as endpoint error: %v", err)
				}
			}
			seg.Payload[0] = 99
			if len(pending[flow]) != faketcp.MaxBootstrapPendingChunks || pending[flow][0].Payload[0] != 7 {
				t.Fatal("queue grew or retained borrowed input")
			}
			drops := server.preAttachDrops
			if lifecycle { drops = life.preAttachDrops }
			if drops != 17 { t.Fatalf("drops=%d want=17", drops) }
			if err := handle(context.Background(), otherSeg, time.Now()); err != nil { t.Fatal(err) }
			if len(pending[other]) != 1 { t.Fatal("another flow was blocked by overflow") }
			// Simulate admission draining its own queue; the next record must
			// be accepted, with no sticky overflow or fabricated ACK/FIN.
			delete(pending, flow)
			if err := handle(context.Background(), seg, time.Now()); err != nil { t.Fatal(err) }
			if len(pending[flow]) != 1 || emitted != beforeEmit { t.Fatal("handoff did not resume or emitted fake progress") }
		})
	}
}
