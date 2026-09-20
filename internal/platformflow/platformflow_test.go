package platformflow

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func testLease(t *testing.T) logicaltunnel.Lease {
	t.Helper()
	id, err := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	return logicaltunnel.Lease{
		Account: "platformflow-test",
		Config: logicaltunnel.TunnelConfig{TunnelID: id, Address4: "10.66.0.7/32"},
	}
}

func testKeys() tlsrecord.KeyPair {
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

func testLane(t *testing.T, role datapath.Role, lease logicaltunnel.Lease, seed byte) *datapath.Lane {
	t.Helper()
	mtu := func(limit int) pathmtu.Config {
		return pathmtu.Config{
			ConnectionMTU: 1500, IPv4HeaderLen: 20, TCPHeaderLen: 20,
			PeerMSS: faketcp.DefaultMSS, PeerMSSSet: true,
			RecordWireLimit: limit,
		}
	}
	cfg := datapath.LaneConfig{
		Role: role, TunnelID: lease.Config.TunnelID.Bytes(),
		ClientRecordLimit: 1300, ServerRecordLimit: 1250,
		Keys: testKeys(),
	}
	cfg.IncarnationNonce[0] = seed
	if role == datapath.RoleClient {
		cfg.TxMTU = mtu(cfg.ServerRecordLimit)
		cfg.RxMTU = mtu(cfg.ClientRecordLimit)
	} else {
		cfg.TxMTU = mtu(cfg.ClientRecordLimit)
		cfg.RxMTU = mtu(cfg.ServerRecordLimit)
	}
	lane, err := datapath.NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return lane
}

func TestServicePacketRoundTripAndReservedMalformedFailClosed(t *testing.T) {
	lease := netip.MustParseAddr("10.66.0.7")
	frame := Frame{
		Kind: KindUDPDatagram, FlowID: 9,
		Peer: netip.MustParseAddrPort("198.51.100.10:53"),
		Payload: []byte("dns"),
	}
	packet, err := MarshalPacket(lease, frame)
	if err != nil {
		t.Fatal(err)
	}
	got, handled, err := ParsePacket(packet, lease)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if got.Kind != frame.Kind || got.FlowID != frame.FlowID || got.Peer != frame.Peer || !bytes.Equal(got.Payload, frame.Payload) {
		t.Fatalf("got=%+v payload=%q", got, got.Payload)
	}

	bad := append([]byte(nil), packet...)
	bad[2] = 0
	bad[3] = 20
	if _, handled, err := ParsePacket(bad, lease); !handled || !errors.Is(err, ErrMalformed) {
		t.Fatalf("malformed reserved handled=%v err=%v", handled, err)
	}

	ordinary := append([]byte(nil), packet...)
	ordinary[9] = 17
	ordinary[10], ordinary[11] = 0, 0
	ordinary[10] = byte(ipv4Checksum(ordinary[:20]) >> 8)
	ordinary[11] = byte(ipv4Checksum(ordinary[:20]))
	if _, handled, err := ParsePacket(ordinary, lease); handled || err != nil {
		t.Fatalf("ordinary handled=%v err=%v", handled, err)
	}
}

func TestTunnelChannelNormalUDPMappingEIMEIFAndExpiry(t *testing.T) {
	lease := testLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	clientLane := testLane(t, datapath.RoleClient, lease, 11)
	serverLane := testLane(t, datapath.RoleServer, lease, 11)
	defer serverLane.Close()
	if _, err := owner.AttachInitial(1, clientLane); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var frames []Frame
	channel, err := NewTunnelChannel(owner, func(out Outbound) error {
		if out.IsGame {
			return errors.New("unexpected game output")
		}
		for _, record := range out.Normal {
			result, err := serverLane.InboundPayload(record.Wire, time.Unix(100, 0))
			if err != nil {
				return err
			}
			for _, packet := range result.Datagrams {
				frame, handled, err := ParsePacket(packet, netip.MustParseAddr("10.66.0.7"))
				if err != nil || !handled {
					return errors.New("service packet decode failed")
				}
				mu.Lock()
				frames = append(frames, frame)
				mu.Unlock()
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var reversePeer, reverseClient netip.AddrPort
	udp, err := NewUDPClient(channel, 5*time.Second, 4, func(peer, client netip.AddrPort, payload []byte) error {
		reversePeer, reverseClient = peer, client
		if string(payload) != "reverse" {
			t.Fatalf("reverse payload=%q", payload)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()

	start := time.Unix(200, 0)
	client := netip.MustParseAddrPort("192.0.2.10:40000")
	peerA := netip.MustParseAddrPort("198.51.100.53:53")
	peerB := netip.MustParseAddrPort("203.0.113.53:5353")
	if err := udp.Forward(client, peerA, []byte("a"), start); err != nil {
		t.Fatal(err)
	}
	if err := udp.Forward(client, peerB, []byte("b"), start.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(frames) != 2 || frames[0].FlowID == 0 || frames[1].FlowID != frames[0].FlowID || frames[0].Peer != peerA || frames[1].Peer != peerB {
		t.Fatalf("frames=%+v", frames)
	}
	flowID := frames[0].FlowID
	mu.Unlock()
	unseen := netip.MustParseAddrPort("203.0.113.77:7777")
	if err := udp.Handle(Frame{Kind: KindUDPDatagram, FlowID: flowID, Peer: unseen, Payload: []byte("reverse")}, start.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if reversePeer != unseen || reverseClient != client {
		t.Fatalf("reverse peer=%s client=%s", reversePeer, reverseClient)
	}
	if st := owner.Stats(); st.BusinessFlows != 1 || st.ActiveLogicalLanes != 1 {
		t.Fatalf("owner stats=%+v", st)
	}
	udp.Tick(start.Add(8 * time.Second))
	if udp.Len() != 0 || owner.Stats().BusinessFlows != 0 {
		t.Fatalf("expiry udp=%d owner=%+v", udp.Len(), owner.Stats())
	}
}

func TestTunnelChannelGameReusesExistingLanes(t *testing.T) {
	lease := testLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 3, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	for id := 1; id <= 3; id++ {
		if _, err := owner.AttachInitial(uint8(id), testLane(t, datapath.RoleClient, lease, byte(20+id))); err != nil {
			t.Fatal(err)
		}
	}
	var got Outbound
	channel, err := NewTunnelChannel(owner, func(out Outbound) error {
		got = out
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := channel.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	frame := Frame{Kind: KindUDPDatagram, FlowID: 1, Peer: netip.MustParseAddrPort("198.51.100.1:53"), Payload: []byte("g")}
	if err := flow.Send(frame, time.Unix(300, 0)); err != nil {
		t.Fatal(err)
	}
	if !got.IsGame || len(got.Game.Lanes) != 3 || len(got.Game.Failures) != 0 {
		t.Fatalf("game out=%+v", got.Game)
	}
	st := owner.Stats()
	if st.BusinessFlows != 1 || st.ActiveLogicalLanes != 3 || st.GameLogicalOutbound != 1 || st.GameLaneCopies != 3 {
		t.Fatalf("stats=%+v", st)
	}
	if err := flow.Close(); err != nil {
		t.Fatal(err)
	}
	if st := owner.Stats(); st.BusinessFlows != 0 || st.ActiveLogicalLanes != 3 {
		t.Fatalf("post-close stats=%+v", st)
	}
}

func TestTCPReliabilityOutOfOrderAndRetransmit(t *testing.T) {
	cfg := TCPReliabilityConfig{ChunkSize: 4, MaxInFlight: 4, RTO: 10 * time.Millisecond, MaxRetransmits: 2, MaxReorderBytes: 16}
	tx, err := NewTCPTransmit(5, cfg)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(400, 0)
	sent, err := tx.Queue([]byte("abcdefgh"), false, start)
	if err != nil || len(sent) != 2 {
		t.Fatalf("sent=%d err=%v", len(sent), err)
	}
	rx, err := NewTCPReceive(5, cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rx.Push(sent[1])
	if err != nil || len(second.Delivered) != 0 || second.Ack.Offset != 0 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	first, err := rx.Push(sent[0])
	if err != nil || string(first.Delivered) != "abcdefgh" || first.Ack.Offset != 8 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if err := tx.Ack(4); err != nil {
		t.Fatal(err)
	}
	retry, err := tx.RetransmitDue(start.Add(10 * time.Millisecond))
	if err != nil || len(retry) != 1 || retry[0].Offset != 4 || string(retry[0].Payload) != "efgh" {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
}


func TestTCPRetiredFlowTailIsBoundedAndUnknownStillFailsClosed(t *testing.T) {
	start := time.Unix(500, 0)
	retiredTimeout := 20 * time.Millisecond
	client := &TCPClient{
		flows:   make(map[uint64]*tcpClientFlow),
		retired: newTCPRetiredSet(retiredTimeout, 2),
	}
	server := &TCPServer{
		flows:   make(map[uint64]*tcpServerFlow),
		retired: newTCPRetiredSet(retiredTimeout, 2),
		dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("retired TCPOpen must not redial an upstream")
			return nil, ErrMalformed
		},
	}

	client.mu.Lock()
	client.retired.add(7, start)
	client.mu.Unlock()
	server.mu.Lock()
	server.retired.add(7, start)
	server.mu.Unlock()

	tails := []Frame{
		{Kind: KindTCPAck, FlowID: 7, Offset: 99},
		{Kind: KindTCPData, FlowID: 7, Offset: 99, Payload: []byte("late")},
		{Kind: KindTCPData, FlowID: 7, Offset: 103, FIN: true},
		{Kind: KindTCPClose, FlowID: 7},
	}
	for _, frame := range tails {
		if err := client.Handle(frame, start.Add(time.Millisecond)); err != nil {
			t.Fatalf("client retired tail kind=%d err=%v", frame.Kind, err)
		}
		if err := server.Handle(frame, start.Add(time.Millisecond)); err != nil {
			t.Fatalf("server retired tail kind=%d err=%v", frame.Kind, err)
		}
	}

	open := Frame{
		Kind: KindTCPOpen, FlowID: 7,
		Peer: netip.MustParseAddrPort("198.51.100.20:443"),
	}
	if err := server.Handle(open, start.Add(time.Millisecond)); err != nil {
		t.Fatalf("retired duplicate open err=%v", err)
	}
	if server.Len() != 0 {
		t.Fatalf("retired duplicate open recreated flow: %d", server.Len())
	}

	unknown := Frame{Kind: KindTCPAck, FlowID: 8}
	if err := client.Handle(unknown, start.Add(time.Millisecond)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("unknown client flow err=%v want ErrMalformed", err)
	}
	if err := server.Handle(unknown, start.Add(time.Millisecond)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("unknown server flow err=%v want ErrMalformed", err)
	}

	expired := start.Add(retiredTimeout)
	if err := client.Handle(Frame{Kind: KindTCPAck, FlowID: 7}, expired); !errors.Is(err, ErrMalformed) {
		t.Fatalf("expired client tombstone err=%v want ErrMalformed", err)
	}
	if err := server.Handle(Frame{Kind: KindTCPAck, FlowID: 7}, expired); !errors.Is(err, ErrMalformed) {
		t.Fatalf("expired server tombstone err=%v want ErrMalformed", err)
	}

	set := newTCPRetiredSet(time.Second, 2)
	set.add(1, start)
	set.add(2, start.Add(time.Millisecond))
	set.add(3, start.Add(2*time.Millisecond))
	if got := set.len(start.Add(3 * time.Millisecond)); got != 2 {
		t.Fatalf("retired set size=%d want=2", got)
	}
	if set.contains(1, start.Add(3*time.Millisecond)) {
		t.Fatal("oldest retired flow was not evicted at capacity")
	}
	if !set.contains(2, start.Add(3*time.Millisecond)) || !set.contains(3, start.Add(3*time.Millisecond)) {
		t.Fatal("newest retired flow tombstones missing")
	}

	closedClient := &tcpClientFlow{id: 11, closed: true}
	if err := client.handleAck(closedClient, Frame{Kind: KindTCPAck, FlowID: 11}, start); err != nil {
		t.Fatalf("concurrent closed client ACK err=%v", err)
	}
	if err := client.handleData(closedClient, Frame{Kind: KindTCPData, FlowID: 11, Payload: []byte("late")}, start); err != nil {
		t.Fatalf("concurrent closed client data err=%v", err)
	}
	closedServer := &tcpServerFlow{id: 12, closed: true}
	if err := server.handleAck(closedServer, Frame{Kind: KindTCPAck, FlowID: 12}, start); err != nil {
		t.Fatalf("concurrent closed server ACK err=%v", err)
	}
	if err := server.handleData(closedServer, Frame{Kind: KindTCPData, FlowID: 12, Payload: []byte("late")}, start); err != nil {
		t.Fatalf("concurrent closed server data err=%v", err)
	}
}
