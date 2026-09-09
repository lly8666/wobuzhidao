//go:build windows

package main

import (
	"testing"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestFakeTCPRetireResetSpecAndPacket(t *testing.T) {
	args := []string{
		"--local-udp", "127.0.0.1:40101",
		"--source", "192.0.2.20:54321",
		"--remote", "198.51.100.10:443",
		"--packet-device", `\\Device\\NPF_{TEST}`,
		"--source-mac", "00:11:22:33:44:55",
		"--next-hop-mac", "66:77:88:99:aa:bb",
	}
	spec, err := fakeTCPRetireResetSpecFromArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	if spec.sourcePort != 54321 || spec.remotePort != 443 {
		t.Fatalf("flow ports source=%d remote=%d", spec.sourcePort, spec.remotePort)
	}
	if spec.sourceIP != [4]byte{192, 0, 2, 20} || spec.remoteIP != [4]byte{198, 51, 100, 10} {
		t.Fatalf("flow ips source=%v remote=%v", spec.sourceIP, spec.remoteIP)
	}

	packet := fakeTCPRetireResetPacket(spec, 77)
	seg, err := faketcp.ParseIPv4TCP(packet)
	if err != nil {
		t.Fatal(err)
	}
	if seg.SrcIP != spec.sourceIP || seg.DstIP != spec.remoteIP || seg.SrcPort != spec.sourcePort || seg.DstPort != spec.remotePort {
		t.Fatalf("reset flow=%v:%d -> %v:%d", seg.SrcIP, seg.SrcPort, seg.DstIP, seg.DstPort)
	}
	if seg.Flags != faketcp.FlagRST {
		t.Fatalf("reset flags=%#x want RST=%#x", seg.Flags, faketcp.FlagRST)
	}
	if len(seg.Payload) != 0 {
		t.Fatalf("reset payload bytes=%d", len(seg.Payload))
	}
}

func TestFakeTCPRetireResetSpecRequiresExactFlowInputs(t *testing.T) {
	_, err := fakeTCPRetireResetSpecFromArgs([]string{
		"--source", "192.0.2.20:54321",
		"--remote", "198.51.100.10:443",
		"--source-mac", "00:11:22:33:44:55",
		"--next-hop-mac", "66:77:88:99:aa:bb",
	})
	if err == nil {
		t.Fatal("missing packet device must reject retirement reset")
	}
}
