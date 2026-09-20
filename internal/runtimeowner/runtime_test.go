package runtimeowner

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func runtimeLease(t *testing.T) logicaltunnel.Lease {
	t.Helper()
	tunnelID, err := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	installation, err := logicaltunnel.ParseInstallationID("ffeeddccbbaa99887766554433221100")
	if err != nil {
		t.Fatal(err)
	}
	lease := logicaltunnel.Lease{
		Account: "runtime",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.77.0.2/32",
			Routes4: []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}
	return lease
}

func runtimeKeys() tlsrecord.KeyPair {
	var out tlsrecord.KeyPair
	for i := range out.C2S.AEADKey {
		out.C2S.AEADKey[i] = byte(i + 1)
		out.C2S.HPKey[i] = byte(0x80 + i)
		out.S2C.AEADKey[i] = byte(0x40 + i)
		out.S2C.HPKey[i] = byte(0xc0 + i)
	}
	for i := range out.C2S.IV {
		out.C2S.IV[i] = byte(0x10 + i)
		out.S2C.IV[i] = byte(0x30 + i)
	}
	return out
}

func runtimeMTU(limit, parity int) pathmtu.Config {
	return pathmtu.Config{
		ConnectionMTU: 1500,
		IPv4HeaderLen: 20,
		TCPHeaderLen: 20,
		PeerMSS: faketcp.DefaultMSS,
		PeerMSSSet: true,
		RecordWireLimit: limit,
		ParityShards: parity,
	}
}

func runtimeLane(t *testing.T, role datapath.Role, lease logicaltunnel.Lease, parity int, seed byte) *datapath.Lane {
	t.Helper()
	cfg := datapath.LaneConfig{
		Role: role,
		ClientRecordLimit: 1300,
		ServerRecordLimit: 1250,
		Keys: runtimeKeys(),
		ParityShards: parity,
		TunnelID: lease.Config.TunnelID.Bytes(),
	}
	cfg.IncarnationNonce[0] = seed
	if parity != 0 {
		cfg.FlushAfter = 8 * time.Millisecond
		cfg.MaxBlocks = 8
	}
	if role == datapath.RoleClient {
		cfg.TxMTU = runtimeMTU(cfg.ServerRecordLimit, parity)
		cfg.RxMTU = runtimeMTU(cfg.ClientRecordLimit, parity)
	} else {
		cfg.TxMTU = runtimeMTU(cfg.ClientRecordLimit, parity)
		cfg.RxMTU = runtimeMTU(cfg.ServerRecordLimit, parity)
	}
	lane, err := datapath.NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return lane
}

func runtimeIPv4(src, dst netip.Addr, payload []byte) []byte {
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

func transportPair(clientEmit, serverEmit faketcp.SegmentEmitter, lane uint8, base uint32) (TransportConfig, TransportConfig) {
	clientPort := uint16(40000 + lane)
	serverPort := uint16(440 + lane)
	clientIP := [4]byte{192, 0, 2, 10}
	serverIP := [4]byte{198, 51, 100, 20}
	client := TransportConfig{
		LocalIP: clientIP, PeerIP: serverIP,
		LocalPort: clientPort, PeerPort: serverPort,
		SendNext: base, ReceiveNext: base + 500000,
		InitialRTO: time.Second, RepairHorizon: 3 * time.Second,
		Emit: clientEmit,
	}
	server := TransportConfig{
		LocalIP: serverIP, PeerIP: clientIP,
		LocalPort: serverPort, PeerPort: clientPort,
		SendNext: base + 500000, ReceiveNext: base,
		InitialRTO: time.Second, RepairHorizon: 3 * time.Second,
		Emit: serverEmit,
	}
	return client, server
}

func TestRuntimeNormalNoHOLRepairReplacementAndGenerationFence(t *testing.T) {
	lease := runtimeLease(t)
	leaseAddr, _ := lease.Config.LeaseIPv4()
	clientOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	serverOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}

	var clientWire, serverWire []faketcp.Segment
	clientEmit := func(seg faketcp.Segment) error {
		clientWire = append(clientWire, seg)
		return nil
	}
	serverEmit := func(seg faketcp.Segment) error {
		serverWire = append(serverWire, seg)
		return nil
	}
	var delivered [][]byte
	clientRuntime, err := New(clientOwner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientRuntime.Close()
	serverRuntime, err := New(serverOwner, func(packets [][]byte, _ time.Time) error {
		for _, packet := range packets {
			delivered = append(delivered, append([]byte(nil), packet...))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer serverRuntime.Close()

	clientLane := runtimeLane(t, datapath.RoleClient, lease, 0, 7)
	serverLane := runtimeLane(t, datapath.RoleServer, lease, 0, 7)
	clientCfg, serverCfg := transportPair(clientEmit, serverEmit, 1, 1000)
	clientSnap, err := clientRuntime.AttachInitial(1, clientLane, clientCfg)
	if err != nil {
		t.Fatal(err)
	}
	serverSnap, err := serverRuntime.AttachInitial(1, serverLane, serverCfg)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1000, 0)
	first := runtimeIPv4(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("first"))
	second := runtimeIPv4(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("second"))
	for _, packet := range [][]byte{first, second} {
		records, err := clientOwner.NormalOutbound(packet, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := clientRuntime.SendNormal(records, now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Millisecond)
	}
	if len(clientWire) != 2 {
		t.Fatalf("fresh transport segments=%d want=2", len(clientWire))
	}

	// Drop the first record and deliver the second. TLS-like records are
	// independent, so business delivery must not wait for the FakeTCP seq gap.
	if err := serverRuntime.HandleSegment(serverSnap.Ref, clientWire[1], now); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || !bytes.Equal(delivered[0], second) {
		t.Fatalf("out-of-order delivery=%d first=%x", len(delivered), firstPacket(delivered))
	}
	if len(serverWire) != 1 || serverWire[0].Ack != clientCfg.SendNext {
		t.Fatalf("gap ACK=%+v want=%d", firstSegment(serverWire), clientCfg.SendNext)
	}
	if err := clientRuntime.HandleSegment(clientSnap.Ref, serverWire[0], now); err != nil {
		t.Fatal(err)
	}

	// The finite repair queue retransmits the missing exact payload without
	// blocking newer record delivery.
	if err := clientRuntime.Tick(time.Unix(1000, 0).Add(1100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(clientWire) != 3 || clientWire[2].Seq != clientWire[0].Seq ||
		!bytes.Equal(clientWire[2].Payload, clientWire[0].Payload) {
		t.Fatalf("repair segment=%+v original=%+v", firstSegment(clientWire[2:]), firstSegment(clientWire))
	}
	if err := serverRuntime.HandleSegment(serverSnap.Ref, clientWire[2], now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 2 || !bytes.Equal(delivered[1], first) {
		t.Fatalf("repaired deliveries=%d second=%x", len(delivered), lastPacket(delivered))
	}
	if len(serverWire) < 2 {
		t.Fatal("missing cumulative ACK after repaired gap")
	}
	if err := clientRuntime.HandleSegment(clientSnap.Ref, serverWire[len(serverWire)-1], now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	stats, ok := clientRuntime.TransportStats(clientSnap.Ref)
	if !ok || stats.Retransmitted != 1 || stats.Outstanding != 0 {
		t.Fatalf("client transport stats=%+v ok=%v", stats, ok)
	}

	// Promote a same-ID replacement on both endpoints. The old runtime transport
	// remains only until explicit retirement and cannot bypass owner generation
	// fencing.
	clientCandidate := runtimeLane(t, datapath.RoleClient, lease, 0, 8)
	serverCandidate := runtimeLane(t, datapath.RoleServer, lease, 0, 8)
	clientCfg2, serverCfg2 := transportPair(clientEmit, serverEmit, 1, 900000)
	if err := clientRuntime.BeginSameIDReplacement(clientSnap.Ref, clientCandidate, clientCfg2); err != nil {
		t.Fatal(err)
	}
	if err := serverRuntime.BeginSameIDReplacement(serverSnap.Ref, serverCandidate, serverCfg2); err != nil {
		t.Fatal(err)
	}
	freshClient, err := clientRuntime.PromoteSameIDReplacement(clientSnap.Ref)
	if err != nil {
		t.Fatal(err)
	}
	freshServer, err := serverRuntime.PromoteSameIDReplacement(serverSnap.Ref)
	if err != nil {
		t.Fatal(err)
	}

	stalePacket := runtimeIPv4(leaseAddr, netip.MustParseAddr("9.9.9.9"), []byte("stale"))
	staleWire, err := clientLane.Outbound(stalePacket, now.Add(3*time.Second))
	if err != nil || len(staleWire) == 0 {
		t.Fatalf("stale wire records=%d err=%v", len(staleWire), err)
	}
	staleSeg := faketcp.Segment{
		SrcIP: clientCfg.LocalIP, DstIP: clientCfg.PeerIP,
		SrcPort: clientCfg.LocalPort, DstPort: clientCfg.PeerPort,
		Seq: clientWire[1].Seq + uint32(len(clientWire[1].Payload)),
		Ack: serverCfg.SendNext, Flags: faketcp.FlagACK | faketcp.FlagPSH,
		Window: 65535, Payload: staleWire[0].Wire,
	}
	if err := serverRuntime.HandleSegment(serverSnap.Ref, staleSeg, now.Add(3*time.Second)); !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
		t.Fatalf("old generation ingress err=%v", err)
	}
	if err := clientRuntime.RetireIncarnation(clientSnap.Ref); err != nil {
		t.Fatal(err)
	}
	if err := serverRuntime.RetireIncarnation(serverSnap.Ref); err != nil {
		t.Fatal(err)
	}

	third := runtimeIPv4(leaseAddr, netip.MustParseAddr("8.8.8.8"), []byte("fresh-generation"))
	records, err := clientOwner.NormalOutbound(third, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	before := len(clientWire)
	if err := clientRuntime.SendNormal(records, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(clientWire) != before+1 || clientWire[before].SrcPort != clientCfg2.LocalPort {
		t.Fatalf("replacement transport segment=%+v", firstSegment(clientWire[before:]))
	}
	if err := serverRuntime.HandleSegment(freshServer.Ref, clientWire[before], now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 3 || !bytes.Equal(delivered[2], third) {
		t.Fatalf("fresh replacement deliveries=%d last=%x", len(delivered), lastPacket(delivered))
	}
	if freshClient.Ref.Generation <= clientSnap.Ref.Generation || freshServer.Ref.Generation <= serverSnap.Ref.Generation {
		t.Fatalf("replacement generations client=%+v server=%+v", freshClient.Ref, freshServer.Ref)
	}
}

func TestRuntimeGameRacesExistingLaneTransportsAndDedupes(t *testing.T) {
	lease := runtimeLease(t)
	leaseAddr, _ := lease.Config.LeaseIPv4()
	clientOwner, err := datapath.NewLeasedTunnelOwner(lease, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	serverOwner, err := datapath.NewLeasedTunnelOwner(lease, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	clientOut := make(map[uint8][]faketcp.Segment)
	serverOut := make(map[uint8][]faketcp.Segment)
	clientRefs := make(map[uint8]logicaltunnel.LaneRef)
	serverRefs := make(map[uint8]logicaltunnel.LaneRef)
	var delivered [][]byte
	clientRuntime, _ := New(clientOwner, nil)
	defer clientRuntime.Close()
	serverRuntime, _ := New(serverOwner, func(packets [][]byte, _ time.Time) error {
		for _, packet := range packets {
			delivered = append(delivered, append([]byte(nil), packet...))
		}
		return nil
	})
	defer serverRuntime.Close()

	for id := uint8(1); id <= 2; id++ {
		laneID := id
		clientEmit := func(seg faketcp.Segment) error {
			clientOut[laneID] = append(clientOut[laneID], seg)
			return nil
		}
		serverEmit := func(seg faketcp.Segment) error {
			serverOut[laneID] = append(serverOut[laneID], seg)
			return nil
		}
		cc, sc := transportPair(clientEmit, serverEmit, id, uint32(id)*100000)
		cs, err := clientRuntime.AttachInitial(id, runtimeLane(t, datapath.RoleClient, lease, 0, 20+id), cc)
		if err != nil {
			t.Fatal(err)
		}
		ss, err := serverRuntime.AttachInitial(id, runtimeLane(t, datapath.RoleServer, lease, 0, 20+id), sc)
		if err != nil {
			t.Fatal(err)
		}
		clientRefs[id] = cs.Ref
		serverRefs[id] = ss.Ref
	}

	packet := runtimeIPv4(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("game"))
	out, err := clientOwner.GameOutbound(packet, time.Unix(2000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := clientRuntime.SendGame(out, time.Unix(2000, 0)); err != nil {
		t.Fatal(err)
	}
	if len(clientOut[1]) != 1 || len(clientOut[2]) != 1 {
		t.Fatalf("game transport copies lane1=%d lane2=%d", len(clientOut[1]), len(clientOut[2]))
	}

	// Lane 2 wins the race; lane 1 later decodes the same PacketID and is
	// suppressed by the existing tunnel-wide Game decoder.
	if err := serverRuntime.HandleSegment(serverRefs[2], clientOut[2][0], time.Unix(2000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := serverRuntime.HandleSegment(serverRefs[1], clientOut[1][0], time.Unix(2000, int64(time.Millisecond))); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || !bytes.Equal(delivered[0], packet) {
		t.Fatalf("game deliveries=%d packet=%x", len(delivered), firstPacket(delivered))
	}
	if stats := serverOwner.Stats(); stats.GameDelivered != 1 || stats.GameDuplicates != 1 || stats.ActiveLogicalLanes != 2 {
		t.Fatalf("server game stats=%+v", stats)
	}
	if _, ok := clientRuntime.TransportStats(clientRefs[1]); !ok {
		t.Fatal("client lane 1 transport missing")
	}
	if len(serverOut[1]) != 1 || len(serverOut[2]) != 1 {
		t.Fatalf("game ACKs lane1=%d lane2=%d", len(serverOut[1]), len(serverOut[2]))
	}
}

func firstPacket(packets [][]byte) []byte {
	if len(packets) == 0 {
		return nil
	}
	return packets[0]
}

func lastPacket(packets [][]byte) []byte {
	if len(packets) == 0 {
		return nil
	}
	return packets[len(packets)-1]
}

func firstSegment(segments []faketcp.Segment) faketcp.Segment {
	if len(segments) == 0 {
		return faketcp.Segment{}
	}
	return segments[0]
}
