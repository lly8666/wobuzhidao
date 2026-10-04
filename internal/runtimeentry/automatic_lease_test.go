package runtimeentry

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net/netip"
	"sync"
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

func TestSharedCredentialsAutomaticNormalAndGameLeases(t *testing.T) {
	cert := runtimeCertificate(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	base, ep := memorySegmentPair()
	mux, err := NewSegmentMux(base.io())
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()
	tun := newMemoryPacketWriter()
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/24"), 8, tun)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := logicaltunnel.NewLeaseRegistry(netip.MustParsePrefix("10.66.0.0/24"), 8)
	var replyOverride atomic.Value
	replyOverride.Store("")
	server, err := NewLifecycleServer(LifecycleServerConfig{ServerConfig: ServerConfig{IO: ep.io(), ListenPort: 443, TickInterval: 10 * time.Millisecond,
		Admission: realityfront.ServerAdmissionConfig{TLS: realityfront.ServerConfig{ServerName: "target.test", RouteKey: key, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, Timeout: 3 * time.Second}, ExpectedUsername: "shared", ExpectedPassword: "same-password", ServerLimit: 1250,
			AllocateLease: func(req realityfront.AdmissionRequest) (string, error) {
				inst, err := logicaltunnel.ParseInstallationID(hex.EncodeToString(req.InstallationID))
				if err != nil {
					return "", err
				}
				id, err := logicaltunnel.TunnelIDFromBytes(req.TunnelID)
				if err != nil {
					return "", err
				}
				lease, err := registry.Acquire(req.Username, inst, id)
				if override := replyOverride.Load().(string); override != "" && inst[0] == 1 {
					return override, err
				}
				return lease.Config.Address4, err
			}},
		LookupLease: registry.Lookup, Router: router, Service: platformflow.DefaultServerConfig(), Lane: datapath.ServerLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20}}, DesiredLanes: 4})
	if err != nil {
		t.Fatal(err)
	}
	registry.BeforeExpire = server.ForgetInactiveTunnel
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go server.Run(ctx)
	defer server.Close()
	var clients [2]*TunnelClient
	var wg sync.WaitGroup
	delivered := []chan []byte{make(chan []byte, 8), make(chan []byte, 8)}
	for index := range clients {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			var inst logicaltunnel.InstallationID
			inst[0] = byte(index + 1)
			id := logicaltunnel.DerivedTunnelID("shared", inst)
			client, err := DialTunnelClient(ctx, TunnelClientConfig{Lease: logicaltunnel.Lease{Account: "shared", InstallationID: inst, Config: logicaltunnel.TunnelConfig{TunnelID: id, Address4: "0.0.0.0/32", Routes4: []string{"0.0.0.0/0"}}}, DesiredLanes: 1 + index*3,
				OpenLane: func(_ uint8, inc uint64) (SegmentIO, faketcp.ClientFlow, error) {
					port, err := RotatingSourcePort(uint16(40000+index*2048), inc)
					if err != nil {
						return SegmentIO{}, faketcp.ClientFlow{}, err
					}
					flow := faketcp.ClientFlow{LocalIP: [4]byte{192, 0, 2, 1}, PeerIP: [4]byte{198, 51, 100, 2}, LocalPort: port, PeerPort: 443}
					laneIO, err := mux.Open(flow)
					return laneIO, flow, err
				},
				Admission: realityfront.ClientAdmissionConfig{AutoLease: true, InstallationID: inst[:], TLS: realityfront.ClientConfig{ServerName: "target.test", RouteKey: key, Timeout: 3 * time.Second}, Username: "shared", Password: "same-password", TunnelID: id.Bytes(), ClientLimit: 1300},
				Lane:      datapath.ClientLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
				Deliver: func(packets [][]byte, _ time.Time) error {
					for _, packet := range packets {
						delivered[index] <- append([]byte(nil), packet...)
					}
					return nil
				}, TickInterval: 10 * time.Millisecond})
			if err != nil {
				t.Error(err)
				return
			}
			clients[index] = client
		}()
	}
	wg.Wait()
	for _, c := range clients {
		if c == nil {
			t.Fatal("client not connected")
		}
		defer c.Close()
	}
	a, _ := clients[0].Owner().Lease()
	b, _ := clients[1].Owner().Lease()
	if a.Config.Address4 == b.Config.Address4 {
		t.Fatal("shared address")
	}
	for index, c := range clients {
		lease, _ := c.Owner().Lease()
		addr, _ := lease.Config.LeaseIPv4()
		packet := ipv4Packet(addr.As4(), [4]byte{8, 8, 8, 8}, 17)
		if err := c.SendPacket(ctx, packet, time.Now()); err != nil {
			t.Fatal(err)
		}
		if got := tun.waitPacket(t, 3*time.Second); string(got) != string(packet) {
			t.Fatal("wrong tunnel delivery")
		}
		spoof := [4]byte{10, 66, 0, 254}
		if spoof == addr.As4() {
			spoof[3] = 253
		}
		if err := c.SendPacket(ctx, ipv4Packet(spoof, [4]byte{8, 8, 8, 8}, 17), time.Now()); err == nil {
			t.Fatal("source spoof accepted")
		}
		if err := c.RotateOldest(ctx); err != nil {
			t.Fatal(err)
		}
		again, _ := c.Owner().Lease()
		if again.Config.Address4 != lease.Config.Address4 {
			t.Fatal("rotation changed lease")
		}
		waitLifecycle(t, 3*time.Second, func() bool { return server.TunnelQualified(lease.Config.TunnelID) })
		reverse := ipv4Packet([4]byte{8, 8, 8, 8}, addr.As4(), 17)
		if err := server.RoutePacket(reverse, time.Now()); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-delivered[index]:
			if string(got) != string(reverse) {
				t.Fatal("reverse crossed client")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("reverse timeout")
		}
	}
	if snapshots := server.TunnelDiagnosticSnapshots(time.Now()); len(snapshots) != 2 || snapshots[0].Lease4 == snapshots[1].Lease4 {
		t.Fatal("automatic diagnostics omitted/crossed client leases")
	}
	// Fault injection changes only the protected admission reply. A healthy old
	// lane must survive a bad candidate; a fully dormant owner must request a
	// platform rebuild rather than quietly applying a different source address.
	changed := "10.66.0.250/32"
	if changed == a.Config.Address4 {
		changed = "10.66.0.251/32"
	}
	replyOverride.Store(changed)
	waitLifecycle(t, 5*time.Second, func() bool { clients[0].mu.Lock(); defer clients[0].mu.Unlock(); return len(clients[0].retiring) == 0 })
	if err := clients[0].RotateOldest(ctx); !errors.Is(err, ErrLeaseMismatch) {
		t.Fatalf("healthy mismatch: %v", err)
	}
	if current, _ := clients[0].Owner().Lease(); current.Config.Address4 != a.Config.Address4 {
		t.Fatal("candidate rebound healthy owner")
	}
	if err := clients[0].Dormant(); err != nil {
		t.Fatal(err)
	}
	if err := server.DormantTunnel(a.Config.TunnelID); err != nil {
		t.Fatal(err)
	}
	if err := clients[0].Wake(ctx); !errors.Is(err, ErrClientLeaseChanged) {
		t.Fatalf("dormant mismatch: %v", err)
	}
	server.admitMu.Lock()
	pinned := !server.forgetInactiveTunnelAt(b.Config.TunnelID, time.Now())
	server.admitMu.Unlock()
	if !pinned { t.Fatal("healthy Game owner not pinned") }
	waitLifecycle(t, 5*time.Second, func() bool { server.mu.Lock(); defer server.mu.Unlock(); return len(server.byTunnel[b.Config.TunnelID].retiring) == 0 })
	server.admitMu.Lock()
	reclaimed := server.forgetInactiveTunnelAt(b.Config.TunnelID, time.Now().Add(4*time.Hour))
	server.admitMu.Unlock()
	if !reclaimed { t.Fatal("stale disconnected owner cannot be reclaimed for expired lease") }
}
