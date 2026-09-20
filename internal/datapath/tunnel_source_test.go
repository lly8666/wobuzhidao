package datapath

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func businessIPv4Packet(src, dst netip.Addr, payload []byte) []byte {
	packet := make([]byte, 20+len(payload))
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	s := src.As4()
	d := dst.As4()
	copy(packet[12:16], s[:])
	copy(packet[16:20], d[:])
	copy(packet[20:], payload)
	return packet
}

func TestLeasedClientOutboundAndServerInboundEnforceSourceLease(t *testing.T) {
	manager := datapathLeaseManager(t)
	lease, err := manager.Acquire("solo", datapathInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	leased, err := lease.Config.LeaseIPv4()
	if err != nil {
		t.Fatal(err)
	}

	clientOwner, err := NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer clientOwner.Close()
	serverOwner, err := NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer serverOwner.Close()

	clientLane := leasedTestLane(t, RoleClient, 0, 21, lease)
	serverLane := leasedTestLane(t, RoleServer, 0, 21, lease)
	clientSnap, err := clientOwner.AttachInitial(1, clientLane)
	if err != nil {
		t.Fatal(err)
	}
	serverSnap, err := serverOwner.AttachInitial(1, serverLane)
	if err != nil {
		t.Fatal(err)
	}

	a, err := clientOwner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	b, err := clientOwner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(400, 0)
	spoofed := businessIPv4Packet(
		netip.MustParseAddr("10.77.0.2"),
		netip.MustParseAddr("1.1.1.1"),
		[]byte("spoof"),
	)
	if records, err := a.Outbound(spoofed, now); !errors.Is(err, logicaltunnel.ErrSourceSpoof) || records != nil {
		t.Fatalf("spoofed outbound records=%d err=%v", len(records), err)
	}
	if stats := clientLane.Stats(); stats.OutboundDatagrams != 0 || stats.OutboundRecords != 0 {
		t.Fatalf("source rejection mutated lane state: %+v", stats)
	}
	if stats := clientOwner.Stats(); stats.SourceDiscards != 1 {
		t.Fatalf("client source discards=%d want=1", stats.SourceDiscards)
	}

	valid := businessIPv4Packet(leased, netip.MustParseAddr("1.1.1.1"), []byte("valid"))
	records, err := b.Outbound(valid, now.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PN != 0 {
		t.Fatalf("valid records=%d pn=%d", len(records), firstPN(records))
	}
	var delivered [][]byte
	for _, record := range records {
		result, err := serverOwner.InboundPayload(serverSnap.Ref, record.Wire, now.Add(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.RecordErrors) != 0 || len(result.PathErrors) != 0 {
			t.Fatalf("valid ingress record errors=%v path errors=%v", result.RecordErrors, result.PathErrors)
		}
		delivered = append(delivered, result.Datagrams...)
	}
	if len(delivered) != 1 || !bytes.Equal(delivered[0], valid) {
		t.Fatalf("valid deliveries=%d packet=%x", len(delivered), firstPacket(delivered))
	}

	// Simulate a compromised/bypassing client that reaches the lane directly.
	// The server owner must enforce the same lease after decrypt/FEC/LINK decode.
	maliciousWire, err := clientLane.Outbound(spoofed, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	result, err := serverOwner.InboundPayload(serverSnap.Ref, maliciousWire[0].Wire, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Datagrams) != 0 || len(result.PathErrors) != 1 || !errors.Is(result.PathErrors[0], logicaltunnel.ErrSourceSpoof) {
		t.Fatalf("spoofed server ingress datagrams=%d pathErrors=%v", len(result.Datagrams), result.PathErrors)
	}
	if stats := serverOwner.Stats(); stats.SourceDiscards != 1 {
		t.Fatalf("server source discards=%d want=1", stats.SourceDiscards)
	}

	// Client receive direction carries arbitrary internet source addresses and
	// must not be mistaken for the client-source lease fence.
	reply := businessIPv4Packet(netip.MustParseAddr("8.8.8.8"), leased, []byte("reply"))
	replyWire, err := serverLane.Outbound(reply, now.Add(3*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	clientResult, err := clientOwner.InboundPayload(clientSnap.Ref, replyWire[0].Wire, now.Add(3*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(clientResult.PathErrors) != 0 || len(clientResult.Datagrams) != 1 || !bytes.Equal(clientResult.Datagrams[0], reply) {
		t.Fatalf("client reply datagrams=%d pathErrors=%v", len(clientResult.Datagrams), clientResult.PathErrors)
	}
}

func TestLeasedSourceFenceRejectsNonIPv4AndMalformedBeforeClientPN(t *testing.T) {
	manager := datapathLeaseManager(t)
	lease, err := manager.Acquire("solo", datapathInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(lease, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	lane := leasedTestLane(t, RoleClient, 0, 22, lease)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}

	ipv6 := make([]byte, 40)
	ipv6[0] = 0x60
	if _, err := flow.Outbound(ipv6, time.Unix(410, 0)); !errors.Is(err, logicaltunnel.ErrSourceSpoof) {
		t.Fatalf("IPv6 outbound err=%v", err)
	}
	leaseAddr, _ := lease.Config.LeaseIPv4()
	malformed := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), nil)
	binary.BigEndian.PutUint16(malformed[2:4], uint16(len(malformed)-1))
	if _, err := flow.Outbound(malformed, time.Unix(410, int64(time.Millisecond))); !errors.Is(err, logicaltunnel.ErrInvalidIPv4Packet) {
		t.Fatalf("malformed outbound err=%v", err)
	}
	if stats := lane.Stats(); stats.OutboundDatagrams != 0 || stats.OutboundRecords != 0 {
		t.Fatalf("invalid packets mutated lane state: %+v", stats)
	}

	valid := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), nil)
	records, err := flow.Outbound(valid, time.Unix(410, 2*int64(time.Millisecond)))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PN != 0 {
		t.Fatalf("first valid PN=%d records=%d", firstPN(records), len(records))
	}
	if stats := owner.Stats(); stats.SourceDiscards != 2 {
		t.Fatalf("source discards=%d want=2", stats.SourceDiscards)
	}
}

func TestOwnerInboundRejectsRetiredGenerationBeforeBusinessDelivery(t *testing.T) {
	manager := datapathLeaseManager(t)
	lease, err := manager.Acquire("solo", datapathInstallation(t, "00112233445566778899aabbccddeeff"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(lease, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	oldServer := leasedTestLane(t, RoleServer, 0, 23, lease)
	oldClient := leasedTestLane(t, RoleClient, 0, 23, lease)
	defer oldClient.Close()
	oldSnap, err := owner.AttachInitial(1, oldServer)
	if err != nil {
		t.Fatal(err)
	}
	candidate := leasedTestLane(t, RoleServer, 0, 24, lease)
	if err := owner.BeginSameIDReplacement(oldSnap.Ref, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.PromoteSameIDReplacement(oldSnap.Ref); err != nil {
		t.Fatal(err)
	}

	leaseAddr, _ := lease.Config.LeaseIPv4()
	packet := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("late"))
	wire, err := oldClient.Outbound(packet, time.Unix(420, 0))
	if err != nil {
		t.Fatal(err)
	}
	result, err := owner.InboundPayload(oldSnap.Ref, wire[0].Wire, time.Unix(420, 0))
	if !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) || len(result.Datagrams) != 0 {
		t.Fatalf("retired generation result=%+v err=%v", result, err)
	}
}

func firstPacket(packets [][]byte) []byte {
	if len(packets) == 0 {
		return nil
	}
	return packets[0]
}
