package runtimeentry

import (
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestLifecycleSYNACKRetiredDuplicateDoesNotStopSharedListener(t *testing.T) {
	var sent []faketcp.Segment
	s := &LifecycleServer{cfg: LifecycleServerConfig{ServerConfig: ServerConfig{IO: SegmentIO{
		Emit: func(seg faketcp.Segment) error { sent = append(sent, seg); return nil },
	}}}}
	table, err := faketcp.NewServerAssociationTable(2, s.cfg.IO.Emit)
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	syn := faketcp.Segment{SrcIP: [4]byte{192, 0, 2, 1}, DstIP: [4]byte{192, 0, 2, 2},
		SrcPort: 42000, DstPort: 443, Seq: 100, Flags: faketcp.FlagSYN, Window: 4096}
	assoc, err := table.AddSYN(syn, 200, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dup, err := table.AddSYN(syn, 300, time.Second)
	if err != nil || dup != assoc {
		t.Fatalf("duplicate lookup: %p %v", dup, err)
	}
	// Deterministic interleaving: rollback wins after duplicate lookup and
	// before its SYNACK snapshot. No sleep or scheduling-dependent retry.
	table.Remove(assoc.Flow())
	if err := s.emitSYNACK(dup); err != nil {
		t.Fatalf("retired duplicate killed shared server: %v", err)
	}
	if len(sent) != 0 || table.Len() != 0 {
		t.Fatal("retired candidate emitted or was resurrected")
	}
	// Another client and an ordinary duplicate still get the original ISN.
	syn.SrcPort++
	live, err := table.AddSYN(syn, 400, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.emitSYNACK(live); err != nil {
		t.Fatal(err)
	}
	if err := s.emitSYNACK(live); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0].Seq != 400 || sent[1].Seq != 400 || sent[0].Ack != 101 || sent[1].Ack != 101 {
		t.Fatalf("live duplicate changed SYNACK: %+v", sent)
	}
}

func TestLifecycleSYNACKDoesNotHideRealEmitFailure(t *testing.T) {
	for _, want := range []error{syscall.EPERM, syscall.EBADF, errors.New("injected raw send failure"), faketcp.ErrHandshakeState} {
		t.Run(want.Error(), func(t *testing.T) {
			s := &LifecycleServer{cfg: LifecycleServerConfig{ServerConfig: ServerConfig{IO: SegmentIO{
				Emit: func(faketcp.Segment) error { return want },
			}}}}
			a, err := faketcp.NewServerAssociation(faketcp.Segment{
				SrcIP: [4]byte{192, 0, 2, 1}, DstIP: [4]byte{192, 0, 2, 2},
				SrcPort: 42000, DstPort: 443, Seq: 100, Flags: faketcp.FlagSYN}, 200, time.Second, s.cfg.IO.Emit)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if got := s.emitSYNACK(a); !errors.Is(got, want) {
				t.Fatalf("wire failure hidden: %v, want %v", got, want)
			}
		})
	}
}
