package runtimeentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestCandidateHandshakeRejectionDoesNotStopSharedServer(t *testing.T) {
	for _, lifecycle := range []bool{false, true} {
		name := "server"
		if lifecycle {
			name = "lifecycle"
		}
		t.Run(name, func(t *testing.T) {
			emitted := 0
			emit := func(faketcp.Segment) error { emitted++; return nil }
			table, err := faketcp.NewServerAssociationTable(4, emit)
			if err != nil {
				t.Fatal(err)
			}
			defer table.Close()
			syn := faketcp.Segment{SrcIP: [4]byte{10, 0, 0, 1}, DstIP: [4]byte{10, 0, 0, 2}, SrcPort: 41000, DstPort: 443, Seq: 100, Flags: faketcp.FlagSYN, Window: 65535}
			candidate, err := table.AddSYN(syn, 9000, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			otherSYN := syn
			otherSYN.SrcPort++
			other, err := table.AddSYN(otherSYN, 10000, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			otherACK := otherSYN
			otherACK.Seq, otherACK.Ack, otherACK.Flags = 101, 10001, faketcp.FlagACK
			if err := other.HandleHandshakeACK(otherACK); err != nil {
				t.Fatal(err)
			}
			// Suppress admission workers: this test exercises the actual packet
			// dispatcher and association validation, independently of TLS.
			started := map[faketcp.ServerFlow]bool{candidate.Flow(): true, other.Flow(): true}
			cfg := ServerConfig{ListenPort: 443, IO: SegmentIO{Emit: emit}}
			server := &Server{cfg: cfg, table: table, started: started}
			life := &LifecycleServer{cfg: LifecycleServerConfig{ServerConfig: cfg}, table: table, started: started}
			handle := server.handleSegment
			if lifecycle {
				handle = life.handleSegment
			}
			valid := syn
			valid.Seq, valid.Ack, valid.Flags = 101, 9001, faketcp.FlagACK
			later := valid
			later.Seq, later.Payload = 601, []byte{1, 2, 3}
			wrongACK := valid
			wrongACK.Ack++
			missingACK := valid
			missingACK.Flags = faketcp.FlagPSH
			for _, rejected := range []faketcp.Segment{later, wrongACK, missingACK} {
				if err := handle(context.Background(), rejected, time.Now()); err != nil {
					t.Fatalf("candidate rejection escaped to shared endpoint: %v", err)
				}
				if candidate.State() != faketcp.ServerAssociationAwaitACK || emitted != 0 {
					t.Fatal("rejected handshake advanced state or fabricated ACK")
				}
				if err := handle(context.Background(), otherACK, time.Now()); err != nil || other.State() != faketcp.ServerAssociationEstablished {
					t.Fatalf("another association was affected: %v", err)
				}
			}
			if err := handle(context.Background(), valid, time.Now()); err != nil || candidate.State() != faketcp.ServerAssociationEstablished {
				t.Fatalf("valid retry no longer establishes candidate: %v", err)
			}
			// Packet isolation must not hide a broken underlay emitter.
			underlayErr := errors.New("underlay emit failed")
			server.cfg.IO.Emit = func(faketcp.Segment) error { return underlayErr }
			life.cfg.IO.Emit = server.cfg.IO.Emit
			newSYN := syn
			newSYN.SrcPort += 2
			if err := handle(context.Background(), newSYN, time.Now()); !errors.Is(err, underlayErr) {
				t.Fatalf("underlay failure was incorrectly suppressed: %v", err)
			}
		})
	}
}
