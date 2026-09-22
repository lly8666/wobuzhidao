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
	clientPort := uint16(40000) + uint16(lane)
	serverPort := uint16(440) + uint16(lane)
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


func TestRuntimeTickFlushesPartialFixedFECOnAuthoritativeLane(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	emit := func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	lane := runtimeLane(t, datapath.RoleClient, lease, 10, 44)
	cfg, _ := transportPair(emit, func(faketcp.Segment) error { return nil }, 1, 7000)
	snap, err := rt.AttachInitial(1, lane, cfg)
	if err != nil {
		t.Fatal(err)
	}
	leaseAddr, _ := lease.Config.LeaseIPv4()
	packet := runtimeIPv4(leaseAddr, netip.MustParseAddr("203.0.113.8"), []byte("fec-partial"))
	t0 := time.Unix(5000, 0)
	records, err := owner.NormalOutbound(packet, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("source records=%d want=1", len(records))
	}
	if err := rt.SendNormal(records, t0); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 {
		t.Fatalf("wire after source=%d want=1", len(wire))
	}
	if err := rt.Tick(t0.Add(7 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 {
		t.Fatalf("early tick emitted parity: wire=%d", len(wire))
	}
	if err := rt.Tick(t0.Add(8 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 {
		t.Fatalf("due tick wire=%d want=2 (source+partial parity)", len(wire))
	}
	laneStats, ok := owner.LaneStats(snap.Ref)
	if !ok {
		t.Fatal("lane stats unavailable")
	}
	enc := laneStats.TxPath.Encoder
	if !laneStats.TxPath.FECEnabled || laneStats.TxPath.ParityShards != 10 ||
		enc.SourceShards != 1 || enc.ParityShards != 1 || enc.PartialBlocks != 1 || enc.PendingSources != 0 {
		t.Fatalf("FEC lane stats=%+v", laneStats.TxPath)
	}
	if laneStats.ExpireCalls != 2 {
		t.Fatalf("expire calls=%d want=2", laneStats.ExpireCalls)
	}
	if stats, ok := rt.TransportStats(snap.Ref); !ok || stats.FreshSent != 2 || stats.Retransmitted != 0 {
		t.Fatalf("transport stats=%+v ok=%v", stats, ok)
	}
}


func TestLaneTransportTickKeepsDueRepairWhenGapForgivenessAlsoDue(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	emit := func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	cfg, _ := transportPair(emit, func(faketcp.Segment) error { return nil }, 1, 12000)
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 61), cfg)
	if err != nil {
		t.Fatal(err)
	}
	transport := rt.lanes[snap.Ref]
	t0 := time.Unix(6000, 0)
	payload := []byte("steady-repair")
	if err := transport.send([]datapath.WireRecord{{Wire: payload}}, t0); err != nil {
		t.Fatal(err)
	}

	transport.mu.Lock()
	futureSeq := cfg.ReceiveNext + 100
	futurePayload := []byte("future")
	if _, err := transport.acceptPayloadLocked(futureSeq, futurePayload, t0.Add(-2*time.Second)); err != nil {
		transport.mu.Unlock()
		t.Fatal(err)
	}
	transport.mu.Unlock()

	if err := transport.tick(t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 {
		t.Fatalf("wire=%d want fresh+repair", len(wire))
	}
	if wire[1].Seq != wire[0].Seq || !bytes.Equal(wire[1].Payload, wire[0].Payload) {
		t.Fatalf("tick emitted=%+v want repair of=%+v", wire[1], wire[0])
	}
	wantACK := futureSeq + uint32(len(futurePayload))
	if wire[1].Ack != wantACK {
		t.Fatalf("repair ACK=%d want forgiven ACK=%d", wire[1].Ack, wantACK)
	}
	stats := transport.statsSnapshot()
	if stats.RepairSelected != 1 || stats.RepairAttempts != 1 ||
		stats.RepairSucceeded != 1 || stats.RepairFailures != 0 ||
		stats.Retransmitted != 1 || stats.ForgivenGaps != 1 {
		t.Fatalf("transport stats=%+v", stats)
	}
}

func TestLaneTransportTickCountsFailedRepairWithoutPretendingItWasSent(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	failRepair := false
	boom := errors.New("repair emit failed")
	emit := func(seg faketcp.Segment) error {
		if failRepair && len(seg.Payload) != 0 {
			return boom
		}
		wire = append(wire, seg)
		return nil
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	cfg, _ := transportPair(emit, func(faketcp.Segment) error { return nil }, 1, 22000)
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 62), cfg)
	if err != nil {
		t.Fatal(err)
	}
	transport := rt.lanes[snap.Ref]
	t0 := time.Unix(7000, 0)
	if err := transport.send([]datapath.WireRecord{{Wire: []byte("repair-me")}}, t0); err != nil {
		t.Fatal(err)
	}
	failRepair = true
	if err := transport.tick(t0.Add(time.Second)); !errors.Is(err, boom) {
		t.Fatalf("tick err=%v want=%v", err, boom)
	}
	stats := transport.statsSnapshot()
	if stats.RepairSelected != 1 || stats.RepairAttempts != 1 ||
		stats.RepairSucceeded != 0 || stats.RepairFailures != 1 ||
		stats.Retransmitted != 0 {
		t.Fatalf("failed repair stats=%+v", stats)
	}
	transport.mu.Lock()
	p := transport.pending[wire[0].Seq]
	if p == nil || !p.lastSent.Equal(t0) || p.retries != 0 {
		transport.mu.Unlock()
		t.Fatalf("failed repair mutated pending=%+v", p)
	}
	transport.mu.Unlock()

	failRepair = false
	if err := transport.tick(t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	stats = transport.statsSnapshot()
	if stats.RepairSelected != 2 || stats.RepairAttempts != 2 ||
		stats.RepairSucceeded != 1 || stats.RepairFailures != 1 ||
		stats.Retransmitted != 1 {
		t.Fatalf("retry stats=%+v", stats)
	}
	if len(wire) != 2 || wire[1].Seq != wire[0].Seq || !bytes.Equal(wire[1].Payload, wire[0].Payload) {
		t.Fatalf("successful retry wire=%+v", wire)
	}
}


func TestRuntimeSteadyFINTailHalfCloseAndReset(t *testing.T) {
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
	clientRuntime, err := New(clientOwner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientRuntime.Close()
	var delivered [][]byte
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

	clientCfg, serverCfg := transportPair(
		func(seg faketcp.Segment) error { clientWire = append(clientWire, seg); return nil },
		func(seg faketcp.Segment) error { serverWire = append(serverWire, seg); return nil },
		1, 31000,
	)
	clientSnap, err := clientRuntime.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 71), clientCfg)
	if err != nil {
		t.Fatal(err)
	}
	serverSnap, err := serverRuntime.AttachInitial(1, runtimeLane(t, datapath.RoleServer, lease, 0, 71), serverCfg)
	if err != nil {
		t.Fatal(err)
	}

	t0 := time.Unix(8000, 0)
	tail := runtimeIPv4(leaseAddr, netip.MustParseAddr("203.0.113.77"), []byte("tail-before-fin"))
	records, err := clientOwner.NormalOutbound(tail, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := clientRuntime.SendNormal(records, t0); err != nil {
		t.Fatal(err)
	}
	if err := clientRuntime.CloseWrite(clientSnap.Ref, t0.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(clientWire) != 2 || clientWire[1].Flags&(faketcp.FlagACK|faketcp.FlagFIN) != faketcp.FlagACK|faketcp.FlagFIN {
		t.Fatalf("client wire=%+v", clientWire)
	}
	if clientWire[1].Seq != clientWire[0].Seq+uint32(len(clientWire[0].Payload)) {
		t.Fatalf("FIN seq=%d tail end=%d", clientWire[1].Seq, clientWire[0].Seq+uint32(len(clientWire[0].Payload)))
	}
	if err := clientRuntime.SendNormal(records, t0.Add(2*time.Millisecond)); !errors.Is(err, ErrTransportWriteClosed) {
		t.Fatalf("send after FIN err=%v", err)
	}

	if err := serverRuntime.HandleSegment(serverSnap.Ref, clientWire[1], t0.Add(3*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if st, _ := serverRuntime.TransportStats(serverSnap.Ref); st.PeerFIN {
		t.Fatalf("out-of-order FIN closed receive side early: %+v", st)
	}
	if err := serverRuntime.HandleSegment(serverSnap.Ref, clientWire[0], t0.Add(4*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || !bytes.Equal(delivered[0], tail) {
		t.Fatalf("tail delivery=%d got=%x", len(delivered), firstPacket(delivered))
	}
	stServer, _ := serverRuntime.TransportStats(serverSnap.Ref)
	if !stServer.PeerFIN {
		t.Fatalf("server did not consume sequenced FIN: %+v", stServer)
	}
	if len(serverWire) < 2 {
		t.Fatalf("server ACKs=%d want >=2", len(serverWire))
	}
	if err := clientRuntime.HandleSegment(clientSnap.Ref, serverWire[len(serverWire)-1], t0.Add(5*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stClient, _ := clientRuntime.TransportStats(clientSnap.Ref)
	if !stClient.WriteClosed || !stClient.LocalFINAcked || stClient.FINAcked != 1 {
		t.Fatalf("client FIN state=%+v", stClient)
	}

	serverTransport := serverRuntime.lanes[serverSnap.Ref]
	before := len(serverWire)
	if err := serverTransport.send([]datapath.WireRecord{{Wire: []byte("server-after-peer-fin")}}, t0.Add(6*time.Millisecond)); err != nil {
		t.Fatalf("half-close blocked server write: %v", err)
	}
	if len(serverWire) != before+1 {
		t.Fatalf("server half-close write count=%d want=%d", len(serverWire), before+1)
	}
	if err := serverRuntime.CloseWrite(serverSnap.Ref, t0.Add(7*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	serverTail := serverWire[before]
	serverFIN := serverWire[len(serverWire)-1]
	if serverFIN.Flags&faketcp.FlagFIN == 0 ||
		serverFIN.Seq != serverTail.Seq+uint32(len(serverTail.Payload)) {
		t.Fatalf("server FIN=%+v tail=%+v", serverFIN, serverTail)
	}

	resetOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	var resetWire []faketcp.Segment
	resetRuntime, err := New(resetOwner, nil)
	if err != nil {
		t.Fatal(err)
	}
	resetCfg, _ := transportPair(func(seg faketcp.Segment) error {
		resetWire = append(resetWire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 91000)
	resetSnap, err := resetRuntime.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 72), resetCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := resetRuntime.Reset(resetSnap.Ref, t0); err != nil {
		t.Fatal(err)
	}
	if len(resetWire) != 1 || resetWire[0].Flags != faketcp.FlagACK|faketcp.FlagRST ||
		resetWire[0].Seq != resetCfg.SendNext || resetWire[0].Ack != resetCfg.ReceiveNext {
		t.Fatalf("steady RST=%+v", firstSegment(resetWire))
	}
	if st, _ := resetRuntime.TransportStats(resetSnap.Ref); !st.Closed || st.RSTSent != 1 {
		t.Fatalf("reset stats=%+v", st)
	}
	resetRuntime.Close()
}


func TestSteadySelectiveACKRetiresPayloadAndFastRepairsHole(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 33000)
	cfg.SACKPermitted = true
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 81), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9000, 0)
	records := make([]datapath.WireRecord, 5)
	for i := range records {
		records[i].Wire = bytes.Repeat([]byte{byte(i + 1)}, 32)
	}
	if err := tr.send(records, t0); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 5 {
		t.Fatalf("fresh wire=%d want=5", len(wire))
	}
	sack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext, Ack: cfg.SendNext,
		Flags: faketcp.FlagACK, Window: 65535,
		SACKN: 1,
	}
	sack.SACK[0] = faketcp.SACKBlock{
		Start: wire[1].Seq,
		End:   wire[4].Seq + uint32(len(wire[4].Payload)),
	}
	if err := rt.HandleSegment(snap.Ref, sack, t0.Add(40*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 6 {
		t.Fatalf("wire=%d want 5 fresh + 1 fast repair", len(wire))
	}
	if wire[5].Seq != wire[0].Seq || !bytes.Equal(wire[5].Payload, wire[0].Payload) {
		t.Fatalf("fast repair=%+v want first=%+v", wire[5], wire[0])
	}
	stats, ok := rt.TransportStats(snap.Ref)
	if !ok || stats.SACKed != 4 || stats.SACKRetired != 4 ||
		stats.FastRepairs != 1 || stats.Retransmitted != 1 {
		t.Fatalf("selective stats=%+v ok=%v", stats, ok)
	}
	tr.mu.Lock()
	for i := 1; i < 5; i++ {
		p := tr.pending[wire[i].Seq]
		if p == nil || !p.sacked || !p.retired || p.payload != nil {
			tr.mu.Unlock()
			t.Fatalf("SACKed record %d retained repair payload: %+v", i, p)
		}
	}
	tr.mu.Unlock()
}

func TestSteadyRepairBudgetDefersRepairButNeverFresh(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 43000)
	cfg.SACKPermitted = true
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 82), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	tr.mu.Lock()
	tr.repairCredit = 0
	tr.repairRemainder = 0
	tr.mu.Unlock()

	t0 := time.Unix(9100, 0)
	records := []datapath.WireRecord{{Wire: bytes.Repeat([]byte{0x51}, 1000)}}
	for i := 0; i < 4; i++ {
		records = append(records, datapath.WireRecord{Wire: bytes.Repeat([]byte{byte(0x60 + i)}, 10)})
	}
	if err := tr.send(records, t0); err != nil {
		t.Fatal(err)
	}
	sack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext, Ack: cfg.SendNext,
		Flags: faketcp.FlagACK, Window: 65535,
		SACKN: 1,
	}
	sack.SACK[0] = faketcp.SACKBlock{
		Start: wire[1].Seq,
		End:   wire[4].Seq + uint32(len(wire[4].Payload)),
	}
	if err := rt.HandleSegment(snap.Ref, sack, t0.Add(20*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 5 {
		t.Fatalf("budget exhaustion emitted repair: wire=%d", len(wire))
	}
	stats, _ := rt.TransportStats(snap.Ref)
	if stats.RepairDeferred == 0 || stats.FastRepairs != 0 {
		t.Fatalf("budget stats=%+v", stats)
	}
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("fresh-still-first")}}, t0.Add(21*time.Millisecond)); err != nil {
		t.Fatalf("fresh blocked by repair budget: %v", err)
	}
	if len(wire) != 6 || string(wire[5].Payload) != "fresh-still-first" {
		t.Fatalf("fresh wire after defer=%+v", wire)
	}
}

func TestSteadyRTTEstimatorAndTimeoutBackoffAreKarnSafe(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 53000)
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 83), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9200, 0)
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("rtt-probe")}}, t0); err != nil {
		t.Fatal(err)
	}
	ack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext,
		Ack: wire[0].Seq + uint32(len(wire[0].Payload)),
		Flags: faketcp.FlagACK, Window: 65535,
	}
	if err := rt.HandleSegment(snap.Ref, ack, t0.Add(600*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ := rt.TransportStats(snap.Ref)
	if stats.SRTT != 600*time.Millisecond || stats.RTO != 1800*time.Millisecond {
		t.Fatalf("RTT stats=%+v want srtt=600ms rto=1.8s", stats)
	}

	t1 := t0.Add(700 * time.Millisecond)
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("timeout-probe")}}, t1); err != nil {
		t.Fatal(err)
	}
	before := len(wire)
	if err := rt.Tick(t1.Add(1799 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != before {
		t.Fatalf("repair fired before estimator RTO: %d -> %d", before, len(wire))
	}
	if err := rt.Tick(t1.Add(1800 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != before+1 {
		t.Fatalf("timeout repair missing wire=%d want=%d", len(wire), before+1)
	}
	stats, _ = rt.TransportStats(snap.Ref)
	if stats.RTORepairs != 1 || stats.RTO != cfg.RepairHorizon {
		t.Fatalf("timeout backoff stats=%+v", stats)
	}
	srttBefore := stats.SRTT
	retry := wire[len(wire)-1]
	ack.Ack = retry.Seq + uint32(len(retry.Payload))
	if err := rt.HandleSegment(snap.Ref, ack, t1.Add(1900*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ = rt.TransportStats(snap.Ref)
	if stats.SRTT != srttBefore || stats.RTO != 1800*time.Millisecond {
		t.Fatalf("ambiguous ACK changed estimator/backoff reset stats=%+v", stats)
	}
}

func TestSteadyACKAdvertisesNegotiatedSACKWithoutGrowingDataRecord(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(faketcp.Segment) error { return nil }, func(faketcp.Segment) error { return nil }, 1, 63000)
	cfg.SACKPermitted = true
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 84), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9300, 0)
	tr.mu.Lock()
	seq := cfg.ReceiveNext + 100
	if _, err := tr.acceptPayloadLocked(seq, []byte("ooo"), t0); err != nil {
		tr.mu.Unlock()
		t.Fatal(err)
	}
	ackSeg := tr.outboundSegment(tr.sendNext, tr.recvNext, nil)
	dataSeg := tr.outboundSegment(tr.sendNext, tr.recvNext, []byte("payload"))
	tr.mu.Unlock()
	if ackSeg.SACKN != 1 || ackSeg.SACK[0] != (faketcp.SACKBlock{Start: seq, End: seq + 3}) {
		t.Fatalf("ACK SACK=%+v", ackSeg.SACK[:ackSeg.SACKN])
	}
	if dataSeg.SACKN != 0 {
		t.Fatalf("data record unexpectedly grew SACK options: %+v", dataSeg)
	}
}


func TestSteadyEffectiveRepairRTOKeepsBoundedRetryInsideHorizon(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 64000)
	cfg.InitialRTO = time.Second
	cfg.RepairHorizon = 3 * time.Second
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 85), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9300, 0)
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("bounded-rto-repair")}}, t0); err != nil {
		t.Fatal(err)
	}
	if err := rt.Tick(t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	stats, _ := rt.TransportStats(snap.Ref)
	if len(wire) != 2 || stats.RTORepairs != 1 || stats.RTO != 2*time.Second {
		t.Fatalf("first timeout wire=%d stats=%+v", len(wire), stats)
	}

	// The connection timeout episode is backed off to 2s, but this exact
	// record has already been repaired. Its effective timer returns to the
	// clean 1s base so the 3s absolute horizon still permits one final bounded
	// repair instead of silently making a second attempt impossible.
	if err := rt.Tick(t0.Add(1999 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 {
		t.Fatalf("second repair fired early wire=%d", len(wire))
	}
	if err := rt.Tick(t0.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	stats, _ = rt.TransportStats(snap.Ref)
	if len(wire) != 3 || stats.RTORepairs != 2 || stats.Retransmitted != 2 {
		t.Fatalf("bounded second repair wire=%d stats=%+v", len(wire), stats)
	}

	if err := rt.Tick(t0.Add(2999 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 3 {
		t.Fatalf("third repair fired inside horizon wire=%d", len(wire))
	}
	if err := rt.Tick(t0.Add(3001 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ = rt.TransportStats(snap.Ref)
	if len(wire) != 3 || stats.Abandoned != 1 || stats.Outstanding != 0 {
		t.Fatalf("horizon did not retire bounded debt wire=%d stats=%+v", len(wire), stats)
	}
}


func TestSteadyIncrementalIndexesDeepHoleSparseACKAndCleanup(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 65000)
	cfg.SACKPermitted = true
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 86), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9400, 0)

	records := make([]datapath.WireRecord, 1024)
	for i := range records {
		records[i].Wire = bytes.Repeat([]byte{byte(i)}, 8)
	}
	if err := tr.send(records, t0); err != nil {
		t.Fatal(err)
	}
	if tr.repairCount != 1024 || len(tr.pendingOrder) != 1024 {
		t.Fatalf("initial indexes repair=%d order=%d", tr.repairCount, len(tr.pendingOrder))
	}

	ack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext, Ack: wire[0].Seq,
		Flags: faketcp.FlagACK, Window: 65535, SACKN: 1,
	}
	ack.SACK[0] = faketcp.SACKBlock{
		Start: wire[1].Seq,
		End: wire[1023].Seq + uint32(len(wire[1023].Payload)),
	}
	if err := rt.HandleSegment(snap.Ref, ack, t0.Add(40*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ := rt.TransportStats(snap.Ref)
	if stats.SACKed != 1023 || stats.FastRepairs != 1 ||
		tr.sackedOutstanding != 1023 || tr.repairCount != 1 || tr.sackSeenN != 1 {
		t.Fatalf("deep-hole indexes stats=%+v sacked=%d repair=%d seen=%d",
			stats, tr.sackedOutstanding, tr.repairCount, tr.sackSeenN)
	}

	for i := 0; i < 32; i++ {
		if err := rt.HandleSegment(snap.Ref, ack, t0.Add(40*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	stats, _ = rt.TransportStats(snap.Ref)
	if stats.SACKed != 1023 || tr.sackSeenN != 1 || tr.repairCount != 1 {
		t.Fatalf("repeated SACK changed indexes stats=%+v seen=%d repair=%d",
			stats, tr.sackSeenN, tr.repairCount)
	}

	ack.SACKN = 0
	ack.Ack = wire[900].Seq + uint32(len(wire[900].Payload))
	if err := rt.HandleSegment(snap.Ref, ack, t0.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if tr.pendingHead != 901 || len(tr.pendingOrder) != 1024 {
		t.Fatalf("sparse ACK head=%d order=%d", tr.pendingHead, len(tr.pendingOrder))
	}
	ack.Ack = tr.sendNext
	if err := rt.HandleSegment(snap.Ref, ack, t0.Add(120*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(tr.pending) != 0 || len(tr.pendingOrder) != 0 || tr.pendingHead != 0 ||
		tr.repairCount != 0 || tr.sackedOutstanding != 0 || tr.sackSeenN != 0 {
		t.Fatalf("drain indexes pending=%d order=%d head=%d repair=%d sacked=%d seen=%d",
			len(tr.pending), len(tr.pendingOrder), tr.pendingHead, tr.repairCount,
			tr.sackedOutstanding, tr.sackSeenN)
	}

	tr.mu.Lock()
	tr.recvSACK[0] = faketcp.SACKBlock{Start: tr.recvNext + 10, End: tr.recvNext + 20}
	tr.recvSACKN = 1
	tr.sackSeen[0] = faketcp.SACKBlock{Start: tr.lastAck + 1, End: tr.lastAck + 2}
	tr.sackSeenN = 1
	tr.mu.Unlock()
	tr.close()
	if tr.pendingHead != 0 || tr.repairCount != 0 || tr.recvSACKN != 0 ||
		tr.sackSeenN != 0 || tr.sackedOutstanding != 0 {
		t.Fatalf("close left indexes head=%d repair=%d recv=%d seen=%d sacked=%d",
			tr.pendingHead, tr.repairCount, tr.recvSACKN, tr.sackSeenN, tr.sackedOutstanding)
	}
}

func TestSteadyIncrementalSACKHandlesSequenceWrapAndFourBlockCache(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 65100)
	cfg.SACKPermitted = true
	cfg.SendNext = ^uint32(0) - 47
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 87), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9500, 0)
	records := make([]datapath.WireRecord, 5)
	for i := range records {
		records[i].Wire = bytes.Repeat([]byte{byte(0x90 + i)}, 24)
	}
	if err := tr.send(records, t0); err != nil {
		t.Fatal(err)
	}
	ack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext, Ack: wire[0].Seq,
		Flags: faketcp.FlagACK, Window: 65535, SACKN: 1,
	}
	ack.SACK[0] = faketcp.SACKBlock{
		Start: wire[1].Seq,
		End: wire[4].Seq + uint32(len(wire[4].Payload)),
	}
	if ack.SACK[0].End >= ack.SACK[0].Start {
		t.Fatalf("test did not cross numeric sequence wrap: %+v", ack.SACK[0])
	}
	if err := rt.HandleSegment(snap.Ref, ack, t0.Add(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ := rt.TransportStats(snap.Ref)
	if stats.SACKed != 4 || stats.FastRepairs != 1 || tr.repairCount != 1 {
		t.Fatalf("wrap SACK stats=%+v repair=%d", stats, tr.repairCount)
	}

	tr.mu.Lock()
	tr.recvNext = ^uint32(0) - 100
	tr.recvSACKN = 0
	clear(tr.received)
	base := tr.recvNext
	for i := 0; i < 6; i++ {
		start := base + uint32(10+i*20)
		tr.received[start] = receiveSpan{end: start + 5, first: t0}
		tr.noteRecvSACKLocked(start, start+5)
	}
	blocks, n := tr.sackBlocksLocked()
	tr.mu.Unlock()
	if n != faketcp.MaxSACKBlocks {
		t.Fatalf("receiver SACK blocks=%d want=%d", n, faketcp.MaxSACKBlocks)
	}
	wantNewest := base + uint32(10+5*20)
	if blocks[0].Start != wantNewest || blocks[0].End != wantNewest+5 {
		t.Fatalf("primary SACK=%+v want=%d..%d", blocks[0], wantNewest, wantNewest+5)
	}
}

func TestSteadyDeliveredHistoryEvictionIsAmortizedAndBounded(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(faketcp.Segment) error { return nil }, func(faketcp.Segment) error { return nil }, 1, 65200)
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 88), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	tr.mu.Lock()
	seq := tr.recvNext
	now := time.Unix(9600, 0)
	for i := 0; i < 7000; i++ {
		if _, err := tr.acceptPayloadLocked(seq, []byte{byte(i)}, now); err != nil {
			tr.mu.Unlock()
			t.Fatal(err)
		}
		seq++
	}
	delivered := len(tr.delivered)
	order := len(tr.deliveredOrder)
	head := tr.deliveredHead
	tr.mu.Unlock()
	if delivered != MaxOutstandingRecords {
		t.Fatalf("delivered history=%d want=%d", delivered, MaxOutstandingRecords)
	}
	if order > MaxOutstandingRecords+steadyIndexCompactThreshold || head >= steadyIndexCompactThreshold {
		t.Fatalf("delivered order not amortized/bounded order=%d head=%d", order, head)
	}
}


func TestSteadyTransportPreservesHandoffWindowScale(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 65300)
	cfg.AdvertisedWindow = uint16(faketcp.MaxBootstrapBufferedBytes >> faketcp.DefaultWindowScale)
	cfg.AdvertisedWindowSet = true
	cfg.WindowScale = faketcp.DefaultWindowScale
	cfg.WindowScaleSet = true
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 89), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9700, 0)
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("persona-window")}}, t0); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 || wire[0].Window != cfg.AdvertisedWindow {
		t.Fatalf("steady data window=%+v want=%d", firstSegment(wire), cfg.AdvertisedWindow)
	}

	tr.mu.Lock()
	ack := tr.outboundSegment(tr.sendNext, tr.recvNext, nil)
	tr.mu.Unlock()
	if ack.Window != cfg.AdvertisedWindow {
		t.Fatalf("steady ACK window=%d want=%d", ack.Window, cfg.AdvertisedWindow)
	}
	stats, ok := rt.TransportStats(snap.Ref)
	if !ok || stats.AdvertisedWindow != cfg.AdvertisedWindow ||
		!stats.WindowScaleSet || stats.WindowScale != faketcp.DefaultWindowScale {
		t.Fatalf("steady presentation stats=%+v ok=%v", stats, ok)
	}
}


func TestSteadyFreshFastRepairWaitsForReorderingWindow(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 65400)
	cfg.SACKPermitted = true
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 90), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(9800, 0)
	records := make([]datapath.WireRecord, 5)
	for i := range records {
		records[i].Wire = bytes.Repeat([]byte{byte(0xa0 + i)}, 64)
	}
	if err := tr.send(records, t0); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 5 {
		t.Fatalf("fresh wire=%d want=5", len(wire))
	}
	sack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext, Ack: wire[0].Seq,
		Flags: faketcp.FlagACK, Window: 65535, SACKN: 1,
	}
	sack.SACK[0] = faketcp.SACKBlock{
		Start: wire[1].Seq,
		End: wire[4].Seq + uint32(len(wire[4].Payload)),
	}

	// A lossless concurrent path can expose three later records within a few
	// milliseconds while the first is merely reordered. SACK must still retire
	// later payload, but fresh fast repair waits for the existing RACK
	// reordering window (minimum 10ms).
	if err := rt.HandleSegment(snap.Ref, sack, t0.Add(3*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ := rt.TransportStats(snap.Ref)
	if len(wire) != 5 || stats.SACKed != 4 || stats.FastRepairs != 0 {
		t.Fatalf("premature fast repair wire=%d stats=%+v", len(wire), stats)
	}

	if err := rt.HandleSegment(snap.Ref, sack, t0.Add(12*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ = rt.TransportStats(snap.Ref)
	if len(wire) != 6 || wire[5].Seq != wire[0].Seq ||
		!bytes.Equal(wire[5].Payload, wire[0].Payload) ||
		stats.FastRepairs != 1 || stats.Retransmitted != 1 {
		t.Fatalf("aged fast repair wire=%+v stats=%+v", wire, stats)
	}
}
