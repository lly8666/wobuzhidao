package runtimeentry

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

const (
	p5SoakDuration      = 31 * time.Minute
	p5SoakRequestPeriod = 10 * time.Second
	p5SoakRequestLimit  = 15 * time.Second
	p5SoakOneWayDelay   = 300 * time.Millisecond
	p5SoakRotateEvery   = 60 * time.Second
	p5SoakDropPercent   = uint64(5)
	p5SoakBaseSeed      = uint64(20260935)
	p5SoakDesiredLanes  = 2
)

var p5SoakPhases = []p5WeakPhase{
	{Name: "sustained5", Start: 0, End: p5SoakDuration, DropPercent: p5SoakDropPercent},
}

func p5SoakRunConfig(t *testing.T) (int, uint64) {
	t.Helper()
	runText := os.Getenv("WBD_P5_SOAK_RUN")
	run, err := strconv.Atoi(runText)
	if err != nil || (run != 1 && run != 2) {
		t.Fatalf("WBD_P5_SOAK_RUN must be 1 or 2, got %q", runText)
	}
	return run, p5SoakBaseSeed + uint64(run-1)
}

func TestP5ThirtyMinuteSoakMeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_SOAK_MEASURE") != "1" {
		t.Skip("P5 soak harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_SOAK_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_SOAK_ARTIFACT_DIR is required")
	}
	sourceSHA := os.Getenv("GITHUB_SHA")
	if len(sourceSHA) != 40 {
		t.Fatalf("GITHUB_SHA must be an exact 40-hex source SHA, got %q", sourceSHA)
	}
	runOrdinal, seed := p5SoakRunConfig(t)

	recorder, err := newP5MeasurementRecorder(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.close()
	artifacts, err := newP5WeakArtifacts(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.close()
	network := newP5WeakNetworkWithConfig(artifacts, seed, p5SoakPhases, p5SoakOneWayDelay, p5SoakDuration)

	var serverCountsMu sync.Mutex
	serverCounts := make(map[int]int)
	certificate := newP5CertificateScenarioWithHandler(t, "controlled-soak-chain", "soak-chain", 6500, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ordinal, err1 := strconv.Atoi(req.URL.Query().Get("ordinal"))
		size, err2 := strconv.Atoi(req.URL.Query().Get("size"))
		if err1 != nil || err2 != nil || ordinal <= 0 || size <= 0 {
			http.Error(w, "invalid soak request", http.StatusBadRequest)
			return
		}
		serverCountsMu.Lock()
		serverCounts[ordinal]++
		serverCountsMu.Unlock()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-WBD-Ordinal", strconv.Itoa(ordinal))
		_, _ = w.Write(p5WeakBody(ordinal, size))
	}))
	defer certificate.server.Close()

	outerCert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("p5-soak-v1"))
	var installation logicaltunnel.InstallationID
	installation[0] = 19
	lease := logicaltunnel.Lease{
		Account: "p5-soak", InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{TunnelID: tunnelID, Address4: "10.66.0.19/32", Routes4: []string{"0.0.0.0/0"}},
	}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}

	clientEP, serverEP := memorySegmentPair()
	clientBase := recorder.wrapIO("c2s", network.wrap("c2s", clientEP.io()))
	serverIO := recorder.wrapIO("s2c", network.wrap("s2c", serverEP.io()))
	mux, err := NewSegmentMux(clientBase)
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
			IO: serverIO, ListenPort: 443, MaxAssociations: 64, InitialRTO: time.Second, TickInterval: 10 * time.Millisecond, MaxFlows: 128,
			Admission: realityfront.ServerAdmissionConfig{
				TLS: realityfront.ServerConfig{ServerName: "target.test", RouteKey: routeKey, TLSConfig: &tls.Config{Certificates: []tls.Certificate{outerCert}}, Timeout: 5 * time.Second},
				ExpectedUsername: "p5-soak", ExpectedPassword: "soak-password", ServerLimit: 1250,
			},
			LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
				if id != tunnelID {
					return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
				}
				return lease.Clone(), nil
			},
			Lane: datapath.ServerLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
			Router: router, Service: platformflow.DefaultServerConfig(),
		},
		DesiredLanes: p5SoakDesiredLanes, DormantAfter: 0, ReplacementGrace: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()
	defer func() {
		serverCancel()
		if err := <-serverDone; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("server shutdown: %v", err)
		}
	}()

	var serviceMu sync.RWMutex
	var clientService *platformflow.Client
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 20*time.Second)
	client, err := DialTunnelClient(dialCtx, TunnelClientConfig{
		OpenLane: func(_ uint8, incarnation uint64) (SegmentIO, faketcp.ClientFlow, error) {
			localPort, err := RotatingSourcePort(44000, incarnation)
			if err != nil {
				return SegmentIO{}, faketcp.ClientFlow{}, err
			}
			flow := faketcp.ClientFlow{
				LocalIP: [4]byte{192, 0, 2, 71}, PeerIP: [4]byte{192, 0, 2, 72},
				LocalPort: localPort, PeerPort: 443,
			}
			ioCfg, err := mux.Open(flow)
			return ioCfg, flow, err
		},
		Lease: lease, DesiredLanes: p5SoakDesiredLanes, MaxFlows: 128,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{ServerName: "target.test", RouteKey: routeKey, Timeout: 5 * time.Second},
			Username: "p5-soak", Password: "soak-password", TunnelID: tunnelID.Bytes(), ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
		Deliver: func(packets [][]byte, now time.Time) error {
			serviceMu.RLock()
			svc := clientService
			serviceMu.RUnlock()
			if svc == nil {
				return errors.New("p5 soak: client service not attached")
			}
			for _, packet := range packets {
				handled, err := svc.HandleServicePacket(packet, now)
				if err != nil {
					return err
				}
				if !handled {
					return platformflow.ErrUnsupported
				}
			}
			return nil
		},
		InitialRTO: time.Second, TickInterval: 10 * time.Millisecond,
		RotateMin: p5SoakRotateEvery, RotateMax: p5SoakRotateEvery, ReplacementGrace: 3 * time.Second,
	})
	dialCancel()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	channel, err := platformflow.NewTunnelChannel(client.Owner(), client.PlatformWireSink)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := platformflow.NewClient(channel, platformflow.DefaultClientConfig(), func(_, _ netip.AddrPort, _ []byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	serviceMu.Lock()
	clientService = svc
	serviceMu.Unlock()
	defer svc.Close()

	tickCtx, tickCancel := context.WithCancel(context.Background())
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-tickCtx.Done():
				return
			case now := <-ticker.C:
				svc.Tick(now)
			}
		}
	}()
	defer func() { tickCancel(); <-tickDone }()

	waitLifecycle(t, 10*time.Second, func() bool {
		clientStats := client.Owner().Stats()
		serverStats, ok := server.TunnelStats(tunnelID)
		return ok && server.TunnelQualified(tunnelID) &&
			clientStats.ActiveLogicalLanes == p5SoakDesiredLanes && clientStats.PhysicalLanes == p5SoakDesiredLanes && clientStats.Retiring == 0 &&
			serverStats.ActiveLogicalLanes == p5SoakDesiredLanes && serverStats.PhysicalLanes == p5SoakDesiredLanes && serverStats.Retiring == 0
	})
	group := p5SoakServerGroup(t, server, tunnelID)
	initialClientGen := laneGenerations(client.Owner().ActiveLanes())
	initialServerGen := laneGenerations(group.owner.ActiveLanes())

	recorder.markSteady()
	scenarioStart := time.Now()
	if err := network.arm(scenarioStart); err != nil {
		t.Fatal(err)
	}

	resourceCtx, resourceCancel := context.WithCancel(context.Background())
	resourceDone := make(chan struct{})
	go func() {
		defer close(resourceDone)
		runP5SoakResourceSampler(resourceCtx, artifacts, recorder, network, client, server, group, svc, initialClientGen)
	}()

	totalScheduled := int(p5SoakDuration / p5SoakRequestPeriod)
	requestSamples := make([]p5WeakRequestSample, 0, totalScheduled)
	var innerC2S, innerS2C uint64
	lifecycleErrors := make([]string, 0)
	for i := 0; i < totalScheduled; i++ {
		scheduled := time.Duration(i) * p5SoakRequestPeriod
		waitP5SparseApplicationDeadline(scenarioStart.Add(scheduled))
		if time.Since(scenarioStart) >= p5SoakDuration {
			break
		}
		ordinal := i + 1
		sample := executeP5SoakHTTPSRequest(t, svc, certificate, scenarioStart, ordinal, p5WeakResponseSizes[i%len(p5WeakResponseSizes)], scheduled)
		requestSamples = append(requestSamples, sample)
		innerC2S += sample.InnerTLSWireBytesC2S
		innerS2C += sample.InnerTLSWireBytesS2C
		if err := artifacts.writeRequest(sample); err != nil {
			t.Fatal(err)
		}
		waitP5TCPFlowCount(t, "soak client", svc.TCPFlows, 0, 8*time.Second)
		waitP5TCPFlowCount(t, "soak server", group.service.TCPFlows, 0, 8*time.Second)
		waitLifecycle(t, 8*time.Second, func() bool {
			return client.Owner().Stats().BusinessFlows == 0 && group.owner.Stats().BusinessFlows == 0
		})
		for {
			select {
			case err := <-client.Errors():
				if err != nil {
					lifecycleErrors = append(lifecycleErrors, err.Error())
					_ = artifacts.writeResource(map[string]any{
						"schema": p5MeasurementSchema, "event": "lifecycle_error",
						"t_ns": time.Since(scenarioStart).Nanoseconds(), "error": err.Error(),
					})
				}
			default:
				goto drained
			}
		}
	drained:
	}
	waitP5SparseApplicationDeadline(scenarioStart.Add(p5SoakDuration))
	resourceCancel()
	<-resourceDone

	waitLifecycle(t, 12*time.Second, func() bool {
		clientStats := client.Owner().Stats()
		serverStats, ok := server.TunnelStats(tunnelID)
		return ok && clientStats.ActiveLogicalLanes == p5SoakDesiredLanes && serverStats.ActiveLogicalLanes == p5SoakDesiredLanes &&
			clientStats.Retiring == 0 && serverStats.Retiring == 0 && clientStats.BusinessFlows == 0 && serverStats.BusinessFlows == 0
	})
	if _, _, err := network.queueSnapshot("c2s"); err != nil {
		t.Fatalf("soak c2s weak link error: %v", err)
	}
	if _, _, err := network.queueSnapshot("s2c"); err != nil {
		t.Fatalf("soak s2c weak link error: %v", err)
	}

	finalClientGen := laneGenerations(client.Owner().ActiveLanes())
	finalServerGen := laneGenerations(group.owner.ActiveLanes())
	rotationAdvances := p5SoakGenerationAdvances(initialClientGen, finalClientGen)
	serverRotationAdvances := p5SoakGenerationAdvances(initialServerGen, finalServerGen)
	if rotationAdvances < 15 || serverRotationAdvances < 15 {
		t.Fatalf("soak rotation coverage too small client=%d server=%d initial_client=%v final_client=%v initial_server=%v final_server=%v",
			rotationAdvances, serverRotationAdvances, initialClientGen, finalClientGen, initialServerGen, finalServerGen)
	}

	clientStats := client.Owner().Stats()
	serverStats, ok := server.TunnelStats(tunnelID)
	if !ok {
		t.Fatal("server soak tunnel disappeared")
	}
	if clientStats.Padding.PaddingBytes != 0 || serverStats.Padding.PaddingBytes != 0 {
		t.Fatalf("soak padding must remain off client=%d server=%d", clientStats.Padding.PaddingBytes, serverStats.Padding.PaddingBytes)
	}
	for _, lane := range client.Owner().ActiveLanes() {
		stats, ok := client.Owner().LaneStats(lane.Ref)
		if !ok || stats.TxPath.FECEnabled || stats.RxPath.FECEnabled {
			t.Fatalf("soak client lane FEC must remain off lane=%+v ok=%v stats=%+v", lane.Ref, ok, stats)
		}
	}
	for _, lane := range group.owner.ActiveLanes() {
		stats, ok := group.owner.LaneStats(lane.Ref)
		if !ok || stats.TxPath.FECEnabled || stats.RxPath.FECEnabled {
			t.Fatalf("soak server lane FEC must remain off lane=%+v ok=%v stats=%+v", lane.Ref, ok, stats)
		}
	}

	successes := 0
	responseBytes := 0
	for _, sample := range requestSamples {
		if sample.Success {
			successes++
			responseBytes += sample.ResponseBytes
		}
	}
	serverCountsMu.Lock()
	serverCountsCopy := make(map[string]int, len(serverCounts))
	keys := make([]int, 0, len(serverCounts))
	for k := range serverCounts {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	duplicates := 0
	for _, k := range keys {
		count := serverCounts[k]
		serverCountsCopy[strconv.Itoa(k)] = count
		if count > 1 {
			duplicates += count - 1
		}
	}
	serverCountsMu.Unlock()
	if duplicates != 0 {
		t.Fatalf("soak application duplicates=%d", duplicates)
	}
	if err := artifacts.writeRequest(map[string]any{
		"schema": p5MeasurementSchema, "event": "application_inventory",
		"scheduled_requests": totalScheduled, "attempted_requests": len(requestSamples), "successful_requests": successes,
		"application_loss": len(requestSamples) - successes, "application_duplicates": duplicates, "server_request_counts": serverCountsCopy,
	}); err != nil {
		t.Fatal(err)
	}

	outerC2S := network.wireSnapshot("c2s")
	outerS2C := network.wireSnapshot("s2c")
	innerTotal := innerC2S + innerS2C
	wireTotal := outerC2S + outerS2C
	wireAmplification := 0.0
	if innerTotal > 0 {
		wireAmplification = float64(wireTotal) / float64(innerTotal)
	}
	outerPackets, recordEvents, clientSYNs := recorder.counters()

	finalInventory := map[string]any{
		"schema": p5MeasurementSchema, "event": "final_inventory", "t_ns": p5SoakDuration.Nanoseconds(),
		"client_owner": clientStats, "server_owner": serverStats,
		"client_tcp_flows": svc.TCPFlows(), "server_tcp_flows": group.service.TCPFlows(),
		"initial_client_generations": initialClientGen, "final_client_generations": finalClientGen,
		"initial_server_generations": initialServerGen, "final_server_generations": finalServerGen,
		"client_rotation_advances": rotationAdvances, "server_rotation_advances": serverRotationAdvances,
		"lifecycle_error_count": len(lifecycleErrors), "lifecycle_errors": lifecycleErrors,
		"fec_profile": "off", "padding_bytes": clientStats.Padding.PaddingBytes + serverStats.Padding.PaddingBytes,
		"outer_wire_bytes_c2s": outerC2S, "outer_wire_bytes_s2c": outerS2C,
		"inner_tls_wire_bytes_c2s": innerC2S, "inner_tls_wire_bytes_s2c": innerS2C,
		"wire_amplification": wireAmplification,
	}
	if err := artifacts.writeResource(finalInventory); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]any{
		"schema": p5MeasurementSchema, "source_sha": sourceSHA, "harness_sha": sourceSHA, "branch": "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name": "p5-new-version-31m-sustained-loss-rotation-retirement-memory",
			"run_ordinal": runOrdinal, "seed": seed, "duration_seconds": int(p5SoakDuration / time.Second),
			"one_way_delay_ms": int(p5SoakOneWayDelay / time.Millisecond), "drop_percent": p5SoakDropPercent,
			"drop_directions": []string{"c2s", "s2c"}, "drop_decision": "deterministic-per-direction-per-phase-permutation-mod-100",
			"application_arrival_model": "absolute-clock every 10s; one real TLS1.3 HTTPS Connection: close flow per slot",
			"response_size_cycle_bytes": p5WeakResponseSizes,
			"desired_lanes": p5SoakDesiredLanes, "rotation_interval_seconds": int(p5SoakRotateEvery / time.Second),
			"replacement_grace_seconds": 3, "fec_profile": "off", "production_default_fec": "off", "padding": "off",
			"memory_plateau_method": "per-minute minimum heap_inuse; compare median minutes 10-19 vs 21-30 with +8MiB/+50pct bound",
			"resource_note": "CPU/PPS/memory are observational within one Actions VM; no cross-VM absolute gate",
		},
		"application": map[string]any{
			"scheduled_requests": totalScheduled, "attempted_requests": len(requestSamples), "successful_requests": successes,
			"loss": len(requestSamples) - successes, "duplicates": duplicates, "response_bytes": responseBytes,
			"inner_tls_wire_bytes_c2s": innerC2S, "inner_tls_wire_bytes_s2c": innerS2C,
		},
		"lifecycle": map[string]any{
			"initial_client_generations": initialClientGen, "final_client_generations": finalClientGen,
			"initial_server_generations": initialServerGen, "final_server_generations": finalServerGen,
			"client_rotation_advances": rotationAdvances, "server_rotation_advances": serverRotationAdvances,
			"final_client_owner": clientStats, "final_server_owner": serverStats,
			"lifecycle_error_count": len(lifecycleErrors),
		},
		"capture": map[string]any{
			"type": "hosted-segmentio-serialized-pcap-before-network-injector", "pcap": "outer.pcap", "events": "events.jsonl",
			"network_events": "network.jsonl", "request_samples": "requests.jsonl", "resource_samples": "resources.jsonl",
			"capture_loss_packets": 0, "outer_packets": outerPackets, "tlslike_record_events": recordEvents, "client_initial_syns": clientSYNs,
		},
		"cost": map[string]any{
			"wire_amplification_scope": "31m pre-injection outer IPv4/TCP bytes over attempted inner TLS wire bytes",
			"wire_amplification_numerator_bytes": wireTotal, "wire_amplification_denominator_bytes": innerTotal, "wire_amplification": wireAmplification,
		},
		"runner": map[string]any{
			"go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "num_cpu": runtime.NumCPU(),
			"github_actions": os.Getenv("GITHUB_ACTIONS"), "runner_os": os.Getenv("RUNNER_OS"), "runner_arch": os.Getenv("RUNNER_ARCH"), "image_os": os.Getenv("ImageOS"),
		},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(artifactDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("WBD_P5_SOAK_CAPTURED source_sha=%s run=%d seed=%d duration=%ds delay_ms=%d drop=%d lanes=%d rotations=%d success=%d attempted=%d loss=%d duplicate=%d lifecycle_errors=%d wire_amp=%.6f\n",
		sourceSHA, runOrdinal, seed, int(p5SoakDuration/time.Second), int(p5SoakOneWayDelay/time.Millisecond), p5SoakDropPercent,
		p5SoakDesiredLanes, rotationAdvances, successes, len(requestSamples), len(requestSamples)-successes, duplicates, len(lifecycleErrors), wireAmplification)
}

func p5SoakServerGroup(t *testing.T, server *LifecycleServer, id logicaltunnel.TunnelID) *serverLifecycleTunnel {
	t.Helper()
	server.mu.Lock()
	group := server.byTunnel[id]
	server.mu.Unlock()
	if group == nil || group.service == nil || group.owner == nil {
		t.Fatal("soak lifecycle server group not ready")
	}
	return group
}

func p5SoakGenerationAdvances(before, after map[uint8]uint64) int {
	total := 0
	for id, start := range before {
		if end := after[id]; end > start {
			total += int(end - start)
		}
	}
	return total
}

func executeP5SoakHTTPSRequest(t *testing.T, svc *platformflow.Client, certificate *p5CertificateScenario, scenarioStart time.Time, ordinal, responseSize int, scheduled time.Duration) p5WeakRequestSample {
	t.Helper()
	sample := p5WeakRequestSample{
		Schema: p5MeasurementSchema, Event: "https_request", Ordinal: ordinal,
		ScheduledStartNS: scheduled.Nanoseconds(), ScheduledPhase: "sustained5", ExpectedResponseBytes: responseSize,
	}
	actualStart := time.Now()
	if actualStart.Sub(scenarioStart) >= p5SoakDuration {
		sample.ErrorStage, sample.Error = "scenario_end_before_start", "measurement window ended before request start"
		return sample
	}
	sample.ActualStartNS = actualStart.Sub(scenarioStart).Nanoseconds()
	sample.SendLagNS = actualStart.Sub(scenarioStart.Add(scheduled)).Nanoseconds()
	sample.ActualPhase = "sustained5"
	deadline := actualStart.Add(p5SoakRequestLimit)
	if end := scenarioStart.Add(p5SoakDuration); deadline.After(end) {
		deadline = end
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		sample.ErrorStage, sample.Error = "local_listen", err.Error()
		return sample
	}
	type acceptResult struct{ conn net.Conn; err error }
	accepted := make(chan acceptResult, 1)
	go func() { conn, err := listener.Accept(); accepted <- acceptResult{conn: conn, err: err} }()
	appConn, err := net.DialTimeout("tcp4", listener.Addr().String(), 2*time.Second)
	if err != nil {
		_ = listener.Close()
		sample.ErrorStage, sample.Error = "local_dial", err.Error()
		return sample
	}
	peer := <-accepted
	_ = listener.Close()
	if peer.err != nil {
		_ = appConn.Close()
		sample.ErrorStage, sample.Error = "local_accept", peer.err.Error()
		return sample
	}
	flowID, err := svc.AddTCP(peer.conn, certificate.targetAddr, actualStart)
	if err != nil {
		_ = appConn.Close()
		_ = peer.conn.Close()
		sample.ErrorStage, sample.Error = "platformflow_open", err.Error()
		return sample
	}
	sample.BusinessFlowID = uint64(flowID)

	counting := &p5CountingConn{Conn: appConn}
	_ = counting.SetDeadline(deadline)
	tlsCfg := certificate.tlsConfig.Clone()
	tlsCfg.ClientSessionCache = nil
	tlsConn := tls.Client(counting, tlsCfg)
	defer tlsConn.Close()

	hsStart := time.Now()
	if err := tlsConn.Handshake(); err != nil {
		sample.HandshakeNS = time.Since(hsStart).Nanoseconds()
		sample.ErrorStage, sample.Error = "tls_handshake", err.Error()
		read, written := counting.snapshot()
		sample.InnerTLSWireBytesC2S, sample.InnerTLSWireBytesS2C = written, read
		return sample
	}
	sample.HandshakeNS = time.Since(hsStart).Nanoseconds()
	state := tlsConn.ConnectionState()
	if state.Version != tls.VersionTLS13 || state.DidResume {
		t.Fatalf("soak request %d TLS state version=0x%x resumed=%v", ordinal, state.Version, state.DidResume)
	}
	certificate.verifyState(t, state)

	requestStart := time.Now()
	sample.RequestStartNS = requestStart.Sub(scenarioStart).Nanoseconds()
	reqURL := &url.URL{Scheme: "https", Host: certificate.targetAddr.String(), Path: "/soak", RawQuery: fmt.Sprintf("ordinal=%d&size=%d", ordinal, responseSize)}
	req := &http.Request{Method: http.MethodGet, URL: reqURL, Host: "target.test", Header: make(http.Header)}
	req.Header.Set("Connection", "close")
	if err := req.Write(tlsConn); err != nil {
		sample.ErrorStage, sample.Error = "request_write", err.Error()
		read, written := counting.snapshot()
		sample.InnerTLSWireBytesC2S, sample.InnerTLSWireBytesS2C = written, read
		return sample
	}
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		sample.ErrorStage, sample.Error = "response_headers", err.Error()
		read, written := counting.snapshot()
		sample.InnerTLSWireBytesC2S, sample.InnerTLSWireBytesS2C = written, read
		return sample
	}
	body, bodyErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	complete := time.Now()
	sample.RequestCompleteNS = complete.Sub(scenarioStart).Nanoseconds()
	sample.RTTNS = complete.Sub(requestStart).Nanoseconds()
	sample.HTTPStatus = resp.StatusCode
	sample.ResponseBytes = len(body)
	read, written := counting.snapshot()
	sample.InnerTLSWireBytesC2S, sample.InnerTLSWireBytesS2C = written, read
	if bodyErr != nil {
		sample.ErrorStage, sample.Error = "response_body", bodyErr.Error()
		return sample
	}
	expected := p5WeakBody(ordinal, responseSize)
	if resp.StatusCode != http.StatusOK || !bytesEqual(body, expected) {
		t.Fatalf("soak HTTPS integrity ordinal=%d status=%d bytes=%d expected=%d", ordinal, resp.StatusCode, len(body), len(expected))
	}
	sum := sha256.Sum256(body)
	sample.BodySHA256 = fmt.Sprintf("%x", sum[:])
	sample.Success = true
	return sample
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func runP5SoakResourceSampler(ctx context.Context, artifacts *p5WeakArtifacts, recorder *p5MeasurementRecorder, network *p5WeakNetwork, client *TunnelClient, server *LifecycleServer, group *serverLifecycleTunnel, svc *platformflow.Client, initialGen map[uint8]uint64) {
	var initialMem runtime.MemStats
	runtime.ReadMemStats(&initialMem)
	sample := func(now time.Time) {
		outer, _, _ := recorder.counters()
		cDepth, cPeak, _ := network.queueSnapshot("c2s")
		sDepth, sPeak, _ := network.queueSnapshot("s2c")
		serverStats, _ := server.TunnelStats(group.id)
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		_ = artifacts.writeResource(map[string]any{
			"schema": p5MeasurementSchema, "event": "resource_sample", "t_ns": artifacts.rel(now),
			"outer_packets": outer,
			"outer_wire_bytes_c2s": network.wireSnapshot("c2s"), "outer_wire_bytes_s2c": network.wireSnapshot("s2c"),
			"injection_queue_depth_c2s": cDepth, "injection_queue_depth_s2c": sDepth,
			"injection_queue_peak_c2s": cPeak, "injection_queue_peak_s2c": sPeak,
			"client_owner": client.Owner().Stats(), "server_owner": serverStats,
			"client_generations": laneGenerations(client.Owner().ActiveLanes()),
			"server_generations": laneGenerations(group.owner.ActiveLanes()),
			"client_tcp_flows": svc.TCPFlows(), "server_tcp_flows": group.service.TCPFlows(),
			"mem_alloc_bytes": mem.Alloc, "heap_alloc_bytes": mem.HeapAlloc, "heap_inuse_bytes": mem.HeapInuse, "sys_bytes": mem.Sys,
			"num_gc": mem.NumGC, "pause_total_ns": mem.PauseTotalNs, "host_cpu": readP5WeakHostCPU(),
			"initial_heap_inuse_bytes": initialMem.HeapInuse, "initial_generations": initialGen,
		})
	}
	sample(time.Now())
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			sample(time.Now())
			return
		case now := <-ticker.C:
			sample(now)
		}
	}
}
