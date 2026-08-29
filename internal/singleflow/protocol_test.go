package singleflow

import (
	"bytes"
	"testing"
)

func TestOrderedAssemblerBuffersOnlyBootstrapGap(t *testing.T) {
	a := NewOrderedAssembler(1000)
	if got := a.Push(1003, []byte("def")); got != nil {
		t.Fatalf("out-of-order bootstrap bytes delivered early: %q", got)
	}
	if got := a.Push(1000, []byte("abc")); !bytes.Equal(got, []byte("abcdef")) {
		t.Fatalf("contiguous bootstrap bytes = %q, want abcdef", got)
	}
	if a.Next() != 1006 {
		t.Fatalf("next=%d want 1006", a.Next())
	}
	if got := a.Push(1000, []byte("abc")); got != nil {
		t.Fatalf("duplicate bootstrap bytes delivered: %q", got)
	}
}

func TestOrderedAssemblerHandlesSequenceWrap(t *testing.T) {
	start := uint32(0xfffffffd)
	a := NewOrderedAssembler(start)
	if got := a.Push(1, []byte("B")); got != nil {
		t.Fatalf("wrapped out-of-order byte delivered early: %q", got)
	}
	if got := a.Push(start, []byte("AAAA")); !bytes.Equal(got, []byte("AAAAB")) {
		t.Fatalf("wrapped contiguous bytes = %q, want AAAAB", got)
	}
}

func TestSwitchFramesBindTicketWithoutExposingIt(t *testing.T) {
	ticket := bytes.Repeat([]byte{0x5a}, 32)
	req := SwitchRequest(ticket)
	ack := SwitchAck(ticket)
	if len(req) != SwitchFrameLen || len(ack) != SwitchFrameLen {
		t.Fatalf("switch lengths req=%d ack=%d", len(req), len(ack))
	}
	if bytes.Contains(req, ticket) || bytes.Contains(ack, ticket) {
		t.Fatal("switch frame exposes ticket")
	}
	if bytes.Equal(req, ack) {
		t.Fatal("request and ack must be distinct")
	}
	if !IsSwitchRequest(req, ticket) || IsSwitchAck(req, ticket) {
		t.Fatal("request classification failed")
	}
	if !IsSwitchAck(ack, ticket) || IsSwitchRequest(ack, ticket) {
		t.Fatal("ack classification failed")
	}
	wrong := append([]byte(nil), ticket...)
	wrong[0] ^= 0xff
	if IsSwitchRequest(req, wrong) || IsSwitchAck(ack, wrong) {
		t.Fatal("switch accepted wrong ticket binding")
	}
}
