package runtimeentry

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

type lifecycleAuditHarness struct {
	dropPortsThrough *atomic.Uint32
	failOpen         *atomic.Bool
	ctx              context.Context
	cancel           context.CancelFunc
	client           *TunnelClient
	server           *LifecycleServer
	serverTUN        *memoryPacketWriter
	clientDeliver    chan []byte
	tunnelID         logicaltunnel.TunnelID
	lease            logicaltunnel.Lease

	mux          *SegmentMux
	serverCancel context.CancelFunc
	serverDone   chan error
	clientFIN    *atomic.Uint64
	serverFIN    *atomic.Uint64
}

func newLifecycleAuditHarness(t *testing.T, lanes int, dormantAfter, rotateEvery time.Duration) *lifecycleAuditHarness {
	t.Helper()

	cert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("closure-audit-v1"))
	tunnelID[15] = byte('0' + lanes)
	var installation logicaltunnel.InstallationID
	installation[0] = byte(lanes + 10)
	lease := logicaltunnel.Lease{
		Account:        "closure-audit",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.66.0.31/32",
			Routes4:  []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}

	clientBase, serverEP := memorySegmentPair()
	dropPortsThrough := &atomic.Uint32{}
	failOpen := &atomic.Bool{}
	clientFIN := &atomic.Uint64{}
	serverFIN := &atomic.Uint64{}
	clientIO := clientBase.io()
	clientEmit := clientIO.Emit
	clientIO.Emit = func(seg faketcp.Segment) error {
		if uint32(seg.SrcPort) <= dropPortsThrough.Load() {
			return nil
		}
		if seg.Flags&faketcp.FlagFIN != 0 {
			clientFIN.Add(1)
		}
		return clientEmit(seg)
	}
	serverIO := serverEP.io()
	serverEmit := serverIO.Emit
	serverIO.Emit = func(seg faketcp.Segment) error {
		if uint32(seg.DstPort) <= dropPortsThrough.Load() {
			return nil
		}
		if seg.Flags&faketcp.FlagFIN != 0 {
			serverFIN.Add(1)
		}
		return serverEmit(seg)
	}
	mux, err := NewSegmentMux(clientIO)
	if err != nil {
		t.Fatal(err)
	}

	serverTUN := newMemoryPacketWriter()
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 8, serverTUN)
	if err != nil {
		_ = mux.Close()
		t.Fatal(err)
	}
	server, err := NewLifecycleServer(LifecycleServerConfig{
		ServerConfig: ServerConfig{
			IO:              serverIO,
			ListenPort:      443,
			MaxAssociations: 64,
			InitialRTO:      time.Second,
			TickInterval:    10 * time.Millisecond,
			MaxFlows:        64,
			Admission: realityfront.ServerAdmissionConfig{
				TLS: realityfront.ServerConfig{
					ServerName: "target.test",
					RouteKey:   routeKey,
					TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
					Timeout:    3 * time.Second,
				},
				ExpectedUsername: "closure-audit",
				ExpectedPassword: "correct-password",
				ServerLimit:      1250,
			},
			LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
				if id != tunnelID {
					return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
				}
				return lease.Clone(), nil
			},
			Lane: datapath.ServerLaneParams{
				ConnectionMTU:   1500,
				TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
				RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
			},
			Router:  router,
			Service: platformflow.DefaultServerConfig(),
		},
		DesiredLanes:      lanes,
		DormantAfter:      dormantAfter,
		KeepaliveInterval: time.Second,
		ReplacementGrace:  50 * time.Millisecond,
	})
	if err != nil {
		_ = mux.Close()
		t.Fatal(err)
	}

	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	clientDeliver := make(chan []byte, 16)
	client, err := DialTunnelClient(ctx, TunnelClientConfig{
		OpenLane: func(_ uint8, incarnation uint64) (SegmentIO, faketcp.ClientFlow, error) {
			if failOpen.Load() {
				return SegmentIO{}, faketcp.ClientFlow{}, errors.New("injected candidate open failure")
			}
			flow := faketcp.ClientFlow{
				LocalIP:   [4]byte{192, 0, 2, 51},
				PeerIP:    [4]byte{192, 0, 2, 61},
				LocalPort: uint16(43000) + uint16(incarnation),
				PeerPort:  443,
			}
			ioCfg, err := mux.Open(flow)
			return ioCfg, flow, err
		},
		Lease:        lease,
		DesiredLanes: lanes,
		MaxFlows:     64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				Timeout:    3 * time.Second,
			},
			Username:    "closure-audit",
			Password:    "correct-password",
			TunnelID:    tunnelID.Bytes(),
			ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU:   1500,
			TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		},
		Deliver: func(packets [][]byte, _ time.Time) error {
			for _, packet := range packets {
				clientDeliver <- append([]byte(nil), packet...)
			}
			return nil
		},
		TickInterval:      10 * time.Millisecond,
		DormantAfter:      dormantAfter,
		KeepaliveInterval: time.Second, DeadAfter: 3 * time.Second,
		RotateMin:        rotateEvery,
		RotateMax:        rotateEvery,
		ReplacementGrace: 50 * time.Millisecond,
	})
	if err != nil {
		cancel()
		serverCancel()
		<-serverDone
		_ = mux.Close()
		t.Fatal(err)
	}

	h := &lifecycleAuditHarness{
		dropPortsThrough: dropPortsThrough, failOpen: failOpen,
		ctx: ctx, cancel: cancel,
		client: client, server: server,
		serverTUN: serverTUN, clientDeliver: clientDeliver,
		tunnelID: tunnelID, lease: lease,
		mux: mux, serverCancel: serverCancel, serverDone: serverDone,
		clientFIN: clientFIN, serverFIN: serverFIN,
	}
	t.Cleanup(func() {
		_ = h.client.Close()
		h.cancel()
		h.serverCancel()
		select {
		case err := <-h.serverDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("server close: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("timed out stopping lifecycle audit server")
		}
		_ = h.mux.Close()
	})
	return h
}

func (h *lifecycleAuditHarness) sendForward(t *testing.T, dst [4]byte) []byte {
	t.Helper()
	packet := ipv4Packet([4]byte{10, 66, 0, 31}, dst, 17)
	if err := h.client.SendPacket(h.ctx, packet, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := h.serverTUN.waitPacket(t, 3*time.Second); string(got) != string(packet) {
		t.Fatalf("forward packet mismatch got=%x want=%x", got, packet)
	}
	waitLifecycle(t, 3*time.Second, func() bool { return h.server.TunnelQualified(h.tunnelID) })
	return packet
}

func TestLifecycleEntryGameThreeAndFourLaneMatrix(t *testing.T) {
	for _, lanes := range []int{3, 4} {
		lanes := lanes
		t.Run(fmt.Sprintf("lanes-%d", lanes), func(t *testing.T) {
			h := newLifecycleAuditHarness(t, lanes, 0, 0)
			h.sendForward(t, [4]byte{8, 8, 8, byte(lanes)})
			waitLifecycle(t, 3*time.Second, func() bool {
				serverStats, ok := h.server.TunnelStats(h.tunnelID)
				return ok && serverStats.GameDelivered == 1 &&
					serverStats.GameDuplicates >= uint64(lanes-1)
			})

			clientStats := h.client.Owner().Stats()
			serverStats, ok := h.server.TunnelStats(h.tunnelID)
			if !ok {
				t.Fatal("server tunnel missing")
			}
			if clientStats.DesiredLanes != lanes || clientStats.ActiveLogicalLanes != lanes ||
				clientStats.PhysicalLanes != lanes || clientStats.GameLogicalOutbound != 1 ||
				clientStats.GameLaneCopies != uint64(lanes) {
				t.Fatalf("client lanes=%d stats=%+v", lanes, clientStats)
			}
			if serverStats.DesiredLanes != lanes || serverStats.ActiveLogicalLanes != lanes ||
				serverStats.PhysicalLanes != lanes || serverStats.GameDelivered != 1 ||
				serverStats.GameDuplicates < uint64(lanes-1) {
				t.Fatalf("server lanes=%d stats=%+v", lanes, serverStats)
			}

			reverse := ipv4Packet([4]byte{1, 1, 1, byte(lanes)}, [4]byte{10, 66, 0, 31}, 6)
			if err := h.server.RoutePacket(reverse, time.Now()); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-h.clientDeliver:
				if string(got) != string(reverse) {
					t.Fatalf("reverse packet mismatch got=%x want=%x", got, reverse)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for reverse Game packet")
			}
			waitLifecycle(t, 3*time.Second, func() bool {
				stats := h.client.Owner().Stats()
				return stats.GameDelivered == 1 && stats.GameDuplicates >= uint64(lanes-1)
			})
		})
	}
}

func TestLifecycleEntryAutomaticRotationTimerConverges(t *testing.T) {
	h := newLifecycleAuditHarness(t, 2, 0, 400*time.Millisecond)
	h.sendForward(t, [4]byte{8, 8, 4, 4})

	ownerPtr := h.client.Owner()
	beforeLease, ok := ownerPtr.Lease()
	if !ok {
		t.Fatal("client owner lost lease before automatic rotation")
	}
	before := laneGenerations(ownerPtr.ActiveLanes())

	waitLifecycle(t, 3*time.Second, func() bool {
		after := laneGenerations(ownerPtr.ActiveLanes())
		advanced := false
		for id, generation := range before {
			if after[id] > generation {
				advanced = true
			}
		}
		stats := ownerPtr.Stats()
		serverStats, ok := h.server.TunnelStats(h.tunnelID)
		return advanced && ok &&
			stats.ActiveLogicalLanes == 2 && stats.PhysicalLanes == 2 && stats.Retiring == 0 &&
			serverStats.ActiveLogicalLanes == 2 && serverStats.PhysicalLanes == 2 && serverStats.Retiring == 0
	})

	if h.clientFIN.Load() == 0 || h.serverFIN.Load() == 0 {
		t.Fatalf("rotation did not use steady FIN client=%d server=%d", h.clientFIN.Load(), h.serverFIN.Load())
	}
	if h.client.Owner() != ownerPtr {
		t.Fatal("automatic rotation replaced TunnelOwner")
	}
	afterLease, ok := ownerPtr.Lease()
	if !ok || afterLease.Config.TunnelID != beforeLease.Config.TunnelID ||
		afterLease.Config.Address4 != beforeLease.Config.Address4 ||
		afterLease.InstallationID != beforeLease.InstallationID {
		t.Fatalf("lease changed across automatic rotation before=%+v after=%+v ok=%v", beforeLease, afterLease, ok)
	}
}

func TestLifecycleEntryPayloadIdleAutoDormantAndBusinessWake(t *testing.T) {
	h := newLifecycleAuditHarness(t, 2, 180*time.Millisecond, 0)
	h.sendForward(t, [4]byte{9, 9, 9, 9})

	ownerPtr := h.client.Owner()
	beforeLease, ok := ownerPtr.Lease()
	if !ok {
		t.Fatal("client owner lost lease before idle")
	}

	// The lifecycle/runtime tick and FakeTCP ACK machinery remain active here.
	// They must not refresh payload idle; both ends should still become DORMANT.
	waitLifecycle(t, 3*time.Second, func() bool {
		serverStats, ok := h.server.TunnelStats(h.tunnelID)
		return h.client.IsDormant() && ok && serverStats.Dormant &&
			h.client.Owner().Stats().ActiveLogicalLanes == 0 &&
			serverStats.ActiveLogicalLanes == 0
	})
	if h.clientFIN.Load() < 2 || h.serverFIN.Load() < 2 {
		t.Fatalf("DORMANT did not emit steady FIN per lane client=%d server=%d", h.clientFIN.Load(), h.serverFIN.Load())
	}

	// A real business packet must wake the same leased owner automatically;
	// callers should not need to invoke Wake directly.
	wakePacket := ipv4Packet([4]byte{10, 66, 0, 31}, [4]byte{4, 4, 4, 4}, 17)
	if err := h.client.SendPacket(h.ctx, wakePacket, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := h.serverTUN.waitPacket(t, 3*time.Second); string(got) != string(wakePacket) {
		t.Fatalf("wake packet mismatch got=%x want=%x", got, wakePacket)
	}
	waitLifecycle(t, 3*time.Second, func() bool {
		clientStats := h.client.Owner().Stats()
		serverStats, ok := h.server.TunnelStats(h.tunnelID)
		return ok && !h.client.IsDormant() && !clientStats.Dormant && !serverStats.Dormant &&
			clientStats.ActiveLogicalLanes == 2 && serverStats.ActiveLogicalLanes == 2 &&
			h.server.TunnelQualified(h.tunnelID)
	})

	if h.client.Owner() != ownerPtr {
		t.Fatal("business wake replaced TunnelOwner")
	}
	afterLease, ok := ownerPtr.Lease()
	if !ok || afterLease.Config.TunnelID != beforeLease.Config.TunnelID ||
		afterLease.Config.Address4 != beforeLease.Config.Address4 ||
		afterLease.InstallationID != beforeLease.InstallationID {
		t.Fatalf("lease changed across automatic idle wake before=%+v after=%+v ok=%v", beforeLease, afterLease, ok)
	}
}

func TestLifecycleEntryExplicitCloseEmitsSteadyFINBeforeNetworkDetach(t *testing.T) {
	h := newLifecycleAuditHarness(t, 2, 0, 0)
	h.sendForward(t, [4]byte{7, 7, 7, 7})
	before := h.clientFIN.Load()
	if err := h.client.Close(); err != nil {
		t.Fatal(err)
	}
	if got := h.clientFIN.Load() - before; got < 2 {
		t.Fatalf("explicit close FINs=%d want at least one per active lane", got)
	}
}

func TestLifecycleServerDormantDeliveryFenceDropsLateBusiness(t *testing.T) {
	server := &LifecycleServer{}
	group := &serverLifecycleTunnel{dormant: true}
	if err := server.deliverTunnelPackets(group, [][]byte{{0xff}}, time.Unix(9400, 0)); err != nil {
		t.Fatalf("dormant late delivery err=%v", err)
	}
	if !group.lastPayload.IsZero() {
		t.Fatalf("dormant late delivery refreshed business activity: %v", group.lastPayload)
	}
}
