package runtimeentry

import (
	"context"
	"crypto/tls"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease(t *testing.T) {
	cert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("runtime-life-v1!"))
	var installation logicaltunnel.InstallationID
	installation[0] = 2
	lease := logicaltunnel.Lease{
		Account: "game",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.66.0.3/32",
			Routes4: []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}

	clientBase, serverEP := memorySegmentPair()
	mux, err := NewSegmentMux(clientBase.io())
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()

	serverTUN := newMemoryPacketWriter()
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 8, serverTUN)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLifecycleServer(LifecycleServerConfig{
		ServerConfig: ServerConfig{
			IO: serverEP.io(),
			ListenPort: 443,
			MaxAssociations: 32,
			InitialRTO: time.Second,
			TickInterval: 10 * time.Millisecond,
			MaxFlows: 64,
			Admission: realityfront.ServerAdmissionConfig{
				TLS: realityfront.ServerConfig{
					ServerName: "target.test",
					RouteKey: routeKey,
					TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
					Timeout: 3 * time.Second,
				},
				ExpectedUsername: "game",
				ExpectedPassword: "correct-password",
				ServerLimit: 1250,
			},
			LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
				if id != tunnelID {
					return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
				}
				return lease.Clone(), nil
			},
			Lane: datapath.ServerLaneParams{
				ConnectionMTU: 1500,
				TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
				RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
			},
			Router: router,
			Service: platformflow.DefaultServerConfig(),
		},
		DesiredLanes: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()

	clientDeliver := make(chan []byte, 8)
	failNextOpen := false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := DialTunnelClient(ctx, TunnelClientConfig{
		OpenLane: func(laneID uint8, incarnation uint64) (SegmentIO, faketcp.ClientFlow, error) {
			if failNextOpen {
				failNextOpen = false
				return SegmentIO{}, faketcp.ClientFlow{}, errors.New("synthetic replacement candidate failure")
			}
			flow := faketcp.ClientFlow{
				LocalIP: [4]byte{192, 0, 2, 30}, PeerIP: [4]byte{192, 0, 2, 40},
				LocalPort: uint16(41000 + incarnation), PeerPort: 443,
			}
			ioCfg, err := mux.Open(flow)
			return ioCfg, flow, err
		},
		Lease: lease,
		DesiredLanes: 2,
		MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second,
			},
			Username: "game", Password: "correct-password",
			TunnelID: tunnelID.Bytes(), ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU: 1500,
			TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		},
		Deliver: func(packets [][]byte, _ time.Time) error {
			for _, packet := range packets {
				clientDeliver <- append([]byte(nil), packet...)
			}
			return nil
		},
		TickInterval: 10 * time.Millisecond,
	})
	if err != nil {
		serverCancel()
		<-serverDone
		t.Fatal(err)
	}
	defer client.Close()
	ownerPtr := client.Owner()
	beforeLease, ok := ownerPtr.Lease()
	if !ok {
		t.Fatal("client owner lost stable lease")
	}

	forward1 := ipv4Packet([4]byte{10, 66, 0, 3}, [4]byte{8, 8, 8, 8}, 17)
	if err := client.SendPacket(ctx, forward1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := serverTUN.waitPacket(t, 3*time.Second); string(got) != string(forward1) {
		t.Fatalf("first game packet mismatch got=%x want=%x", got, forward1)
	}
	waitLifecycle(t, 3*time.Second, func() bool {
		stats, ok := server.TunnelStats(tunnelID)
		return ok && stats.ActiveLogicalLanes == 2 && stats.GameDelivered == 1
	})

	before := laneGenerations(client.Owner().ActiveLanes())
	if before[1] == 0 || before[2] == 0 {
		t.Fatalf("initial generations=%v", before)
	}
	failNextOpen = true
	if err := client.RotateOldest(ctx); err == nil {
		t.Fatal("failed replacement candidate unexpectedly succeeded")
	}
	if got := laneGenerations(client.Owner().ActiveLanes()); got[1] != before[1] || got[2] != before[2] {
		t.Fatalf("failed candidate changed authoritative lanes before=%v after=%v", before, got)
	}
	if stats := client.Owner().Stats(); stats.Retiring != 0 || stats.Candidates != 0 {
		t.Fatalf("failed candidate left transition state: %+v", stats)
	}
	if err := client.RotateOldest(ctx); err != nil {
		t.Fatal(err)
	}
	if stats := client.Owner().Stats(); stats.Retiring != 1 || stats.PhysicalLanes != 3 || stats.ActiveLogicalLanes != 2 {
		t.Fatalf("client A+B overlap stats=%+v", stats)
	}
	waitLifecycle(t, 3*time.Second, func() bool {
		stats, ok := server.TunnelStats(tunnelID)
		return ok && stats.ActiveLogicalLanes == 2 && stats.Retiring == 1 && stats.PhysicalLanes == 3
	})

	forward2 := ipv4Packet([4]byte{10, 66, 0, 3}, [4]byte{1, 1, 1, 1}, 17)
	if err := client.SendPacket(ctx, forward2, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := serverTUN.waitPacket(t, 3*time.Second); string(got) != string(forward2) {
		t.Fatalf("replacement game packet mismatch got=%x want=%x", got, forward2)
	}
	waitLifecycle(t, 3*time.Second, func() bool {
		cs := client.Owner().Stats()
		ss, ok := server.TunnelStats(tunnelID)
		return ok && cs.ActiveLogicalLanes == 2 && ss.ActiveLogicalLanes == 2 &&
			cs.Retiring == 0 && ss.Retiring == 0 &&
			cs.GameLogicalOutbound >= 2 && ss.GameDelivered >= 2
	})
	after := laneGenerations(client.Owner().ActiveLanes())
	if after[1] <= before[1] || after[2] != before[2] {
		t.Fatalf("replacement generations before=%v after=%v", before, after)
	}

	if err := client.Dormant(); err != nil {
		t.Fatal(err)
	}
	if err := server.DormantTunnel(tunnelID); err != nil {
		t.Fatal(err)
	}
	if stats := client.Owner().Stats(); !stats.Dormant || stats.ActiveLogicalLanes != 0 {
		t.Fatalf("client dormant stats=%+v", stats)
	}
	if stats, ok := server.TunnelStats(tunnelID); !ok || !stats.Dormant || stats.ActiveLogicalLanes != 0 {
		t.Fatalf("server dormant stats=%+v ok=%v", stats, ok)
	}

	if err := client.Wake(ctx); err != nil {
		t.Fatal(err)
	}
	if client.Owner() != ownerPtr {
		t.Fatal("wake replaced the stable TunnelOwner")
	}
	afterLease, ok := client.Owner().Lease()
	if !ok || afterLease.Config.TunnelID != beforeLease.Config.TunnelID ||
		afterLease.Config.Address4 != beforeLease.Config.Address4 ||
		afterLease.InstallationID != beforeLease.InstallationID {
		t.Fatalf("lease changed across wake before=%+v after=%+v ok=%v", beforeLease, afterLease, ok)
	}

	forward3 := ipv4Packet([4]byte{10, 66, 0, 3}, [4]byte{9, 9, 9, 9}, 17)
	if err := client.SendPacket(ctx, forward3, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := serverTUN.waitPacket(t, 3*time.Second); string(got) != string(forward3) {
		t.Fatalf("wake game packet mismatch got=%x want=%x", got, forward3)
	}
	waitLifecycle(t, 3*time.Second, func() bool {
		cs := client.Owner().Stats()
		ss, ok := server.TunnelStats(tunnelID)
		return ok && cs.ActiveLogicalLanes == 2 && ss.ActiveLogicalLanes == 2 &&
			cs.GameLogicalOutbound == 3 && ss.GameDelivered == 3
	})

	reverse := ipv4Packet([4]byte{8, 8, 4, 4}, [4]byte{10, 66, 0, 3}, 6)
	if err := server.RoutePacket(reverse, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clientDeliver:
		if string(got) != string(reverse) {
			t.Fatalf("reverse packet mismatch got=%x want=%x", got, reverse)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reverse packet after wake")
	}

	serverCancel()
	if err := <-serverDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func laneGenerations(snapshots []datapath.TunnelLaneSnapshot) map[uint8]uint64 {
	out := make(map[uint8]uint64, len(snapshots))
	for _, snapshot := range snapshots {
		out[snapshot.Ref.ID] = snapshot.Ref.Generation
	}
	return out
}

func waitLifecycle(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if ready() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for lifecycle state")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
