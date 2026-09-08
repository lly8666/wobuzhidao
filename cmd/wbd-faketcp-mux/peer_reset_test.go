//go:build linux

package main

import (
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestHandlePeerResetRemovesCurrentSession(t *testing.T) {
	clientIP := [4]byte{10, 92, 0, 2}
	serverIP := [4]byte{10, 92, 0, 1}
	syn := faketcp.Segment{
		SrcIP: clientIP, DstIP: serverIP,
		SrcPort: 41002, DstPort: 40000,
		Seq: 1000, Flags: faketcp.FlagSYN,
	}
	table, err := faketcp.NewServerAssociationTable(4)
	if err != nil {
		t.Fatal(err)
	}
	assoc, err := table.AddSYN(syn, 7000, faketcp.RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	flow := faketcp.ServerFlowFromSegment(syn)
	sess := &muxSession{
		flow: flow, assoc: assoc, stage: stageHandshake,
		lastClientRX: time.Now(), ackNotify: make(chan struct{}, 1),
	}
	s := &muxServer{table: table, sessions: map[faketcp.ServerFlow]*muxSession{flow: sess}}

	rst := syn
	rst.Seq = 1001
	rst.Flags = faketcp.FlagRST
	if !s.handlePeerReset(flow, sess, rst) {
		t.Fatal("peer RST was not recognized as terminal")
	}
	if got := s.getSession(flow); got != nil {
		t.Fatal("peer RST left mux session registered")
	}
	if table.Len() != 0 {
		t.Fatalf("peer RST left association table entries=%d", table.Len())
	}
	if assoc.State() != faketcp.ServerAssociationClosed {
		t.Fatalf("association state=%v want closed", assoc.State())
	}
}
