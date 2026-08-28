//go:build linux

package main

import (
	"context"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func testMuxHalfOpen(t *testing.T) (*muxServer, *muxSession, faketcp.Segment) {
	t.Helper()
	syn := faketcp.Segment{
		SrcIP:   [4]byte{10, 0, 0, 2},
		DstIP:   [4]byte{10, 0, 0, 1},
		SrcPort: 40000,
		DstPort: 443,
		Seq:     100,
		Flags:   faketcp.FlagSYN,
	}
	table, err := faketcp.NewServerAssociationTable(2)
	if err != nil {
		t.Fatal(err)
	}
	assoc, err := table.AddSYN(syn, 200, faketcp.RecoveryLegacy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	flow := faketcp.ServerFlowFromSegment(syn)
	sess := &muxSession{flow: flow, assoc: assoc}
	s := &muxServer{
		ctx:      context.Background(),
		table:    table,
		sessions: map[faketcp.ServerFlow]*muxSession{flow: sess},
	}
	return s, sess, syn
}

func TestExpireHalfOpenRemovesAwaitACK(t *testing.T) {
	s, sess, _ := testMuxHalfOpen(t)
	s.expireHalfOpenAfter(sess, 5*time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.getSession(sess.flow) == nil && s.table.Len() == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("half-open session was not reaped: session=%v table_len=%d", s.getSession(sess.flow) != nil, s.table.Len())
}

func TestExpireHalfOpenKeepsEstablished(t *testing.T) {
	s, sess, syn := testMuxHalfOpen(t)
	ack := faketcp.Segment{
		SrcIP:   syn.SrcIP,
		DstIP:   syn.DstIP,
		SrcPort: syn.SrcPort,
		DstPort: syn.DstPort,
		Seq:     syn.Seq + 1,
		Ack:     201,
		Flags:   faketcp.FlagACK,
	}
	if err := sess.assoc.HandleHandshakeACK(ack); err != nil {
		t.Fatal(err)
	}

	s.expireHalfOpenAfter(sess, 5*time.Millisecond)
	time.Sleep(25 * time.Millisecond)
	if got := s.getSession(sess.flow); got != sess {
		t.Fatalf("established session was reaped: got=%p want=%p", got, sess)
	}
	if got := s.table.Len(); got != 1 {
		t.Fatalf("established association table len=%d want=1", got)
	}
}
