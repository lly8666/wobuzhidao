package runtimeentry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
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

func p5WeakRunConfig(t *testing.T) (int, uint64) {
	t.Helper()
	runText := os.Getenv("WBD_P5_WEAKNET_RUN")
	run, err := strconv.Atoi(runText)
	if err != nil || (run != 1 && run != 2) {
		t.Fatalf("WBD_P5_WEAKNET_RUN must be 1 or 2, got %q", runText)
	}
	return run, p5WeakBaseSeed + uint64(run-1)
}

func TestP5WeakNetwork5205MeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_WEAKNET_MEASURE") != "1" {
		t.Skip("P5 weak-network harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_WEAKNET_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_WEAKNET_ARTIFACT_DIR is required")
	}
	sourceSHA := os.Getenv("GITHUB_SHA")
	if len(sourceSHA) != 40 {
		t.Fatalf("GITHUB_SHA must be an exact 40-hex source SHA, got %q", sourceSHA)
	}

	recorder, err := newP5MeasurementRecorder(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.close()
	weakArtifacts, err := newP5WeakArtifacts(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer weakArtifacts.close()
	runOrdinal, seed := p5WeakRunConfig(t)
	network := newP5WeakNetwork(weakArtifacts, seed)

	var serverCountsMu sync.Mutex
	serverCounts := make(map[int]int)
	certificate := newP5CertificateScenarioWithHandler(t, "controlled-weaknet-chain", "weaknet-chain", 6000, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ordinal, err1 := strconv.Atoi(req.URL.Query().Get("ordinal"))
		size, err2 := strconv.Atoi(req.URL.Query().Get("size"))
		if err1 != nil || err2 != nil || ordinal <= 0 || size <= 0 {
			http.Error(w, "invalid weaknet request", http.StatusBadRequest)
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
	copy(tunnelID[:], []byte("p5-weaknet-5205"))
	var installation logicaltunnel.InstallationID
	installation[0] = 9
	lease := logicaltunnel.Lease{Account: "p5-weaknet", InstallationID: installation, Config: logicaltunnel.TunnelConfig{TunnelID: tunnelID, Address4: "10.66.0.9/32", Routes4: []string{"0.0.0.0/0"}}}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}

	clientEP, serverEP := memorySegmentPair()
	clientIO := recorder.wrapIO("c2s", network.wrap("c2s", clientEP.io()))
	serverIO := recorder.wrapIO("s2c", network.wrap("s2c", serverEP.io()))
	serverTUN := newMemoryPacketWriter()
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 8, serverTUN)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{
		IO: serverIO, ListenPort: 443, MaxAssociations: 16, InitialRTO: time.Second, TickInterval: 10 * time.Millisecond, MaxFlows: 64,
		Admission: realityfront.ServerAdmissionConfig{TLS: realityfront.ServerConfig{ServerName: "target.test", RouteKey: routeKey, TLSConfig: &tls.Config{Certificates: []tls.Certificate{outerCert}}, Timeout: 3 * time.Second}, ExpectedUsername: "p5-weaknet", ExpectedPassword: "weaknet-password", ServerLimit: 1250},
		LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
			if id != tunnelID {
				return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
			}
			return lease.Clone(), nil
		},
		Lane:   datapath.ServerLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
		Router: router, Service: platformflow.DefaultServerConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()
	defer serverCancel()

	var serviceMu sync.RWMutex
	var clientService *platformflow.Client
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := DialClient(ctx, ClientConfig{
		IO:        clientIO,
		Flow:      faketcp.ClientFlow{LocalIP: [4]byte{192, 0, 2, 54}, PeerIP: [4]byte{192, 0, 2, 60}, LocalPort: 42400, PeerPort: 443},
		ClientISN: 9000, InitialRTO: time.Second, Lease: lease, MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{TLS: realityfront.ClientConfig{ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second}, Username: "p5-weaknet", Password: "weaknet-password", TunnelID: tunnelID.Bytes(), ClientLimit: 1300},
		Lane:      datapath.ClientLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
		Deliver: func(packets [][]byte, now time.Time) error {
			serviceMu.RLock()
			svc := clientService
			serviceMu.RUnlock()
			if svc == nil {
				return errors.New("p5 weaknet: client service not attached")
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
		TickInterval: 10 * time.Millisecond,
	})
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
	defer func() {
		serverCancel()
		if err := <-serverDone; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("server shutdown: %v", err)
		}
	}()

	tickCtx, tickCancel := context.WithCancel(context.Background())
	defer tickCancel()
	go func() {
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

	serverTunnel := waitP5ServerTunnelReady(t, server, tunnelID, 2*time.Second)
	initialRef := client.Ref()
	if initialRef.ID != 1 || initialRef.Generation == 0 {
		t.Fatalf("unexpected initial lane ref: %+v", initialRef)
	}
	if laneStats, ok := client.Owner().LaneStats(initialRef); !ok || laneStats.TxPath.FECEnabled {
		t.Fatalf("weaknet atom must keep FEC off: ok=%v stats=%+v", ok, laneStats)
	}
	if laneStats, ok := serverTunnel.owner.LaneStats(serverTunnel.ref); !ok || laneStats.TxPath.FECEnabled {
		t.Fatalf("server weaknet atom must keep FEC off: ok=%v stats=%+v", ok, laneStats)
	}

	recorder.markSteady()
	scenarioStart := time.Now()
	if err := network.arm(scenarioStart); err != nil {
		t.Fatal(err)
	}

	resourceCtx, resourceCancel := context.WithCancel(context.Background())
	resourceDone := make(chan struct{})
	go func() {
		defer close(resourceDone)
		runP5WeakResourceSampler(resourceCtx, weakArtifacts, recorder, network, client, serverTunnel, initialRef)
	}()

	totalScheduled := int(p5WeakScenarioTime / p5WeakRequestPeriod)
	requestSamples := make([]p5WeakRequestSample, 0, totalScheduled)
	var innerC2S, innerS2C uint64
	for i := 0; i < totalScheduled; i++ {
		scheduled := time.Duration(i) * p5WeakRequestPeriod
		waitP5SparseApplicationDeadline(scenarioStart.Add(scheduled))
		ordinal := i + 1
		if time.Since(scenarioStart) >= p5WeakScenarioTime {
			sample := p5WeakRequestSample{Schema: p5MeasurementSchema, Event: "https_request", Ordinal: ordinal, ScheduledStartNS: scheduled.Nanoseconds(), ScheduledPhase: p5WeakPhaseForOffset(scheduled), ExpectedResponseBytes: p5WeakResponseSizes[i%len(p5WeakResponseSizes)], ErrorStage: "scenario_end_before_start", Error: "measurement window ended before request start"}
			requestSamples = append(requestSamples, sample)
			if err := weakArtifacts.writeRequest(sample); err != nil {
				t.Fatal(err)
			}
			continue
		}
		sample := executeP5WeakHTTPSRequest(t, svc, certificate, scenarioStart, ordinal, p5WeakResponseSizes[i%len(p5WeakResponseSizes)], scheduled)
		requestSamples = append(requestSamples, sample)
		innerC2S += sample.InnerTLSWireBytesC2S
		innerS2C += sample.InnerTLSWireBytesS2C
		if err := weakArtifacts.writeRequest(sample); err != nil {
			t.Fatal(err)
		}
	}
	waitP5SparseApplicationDeadline(scenarioStart.Add(p5WeakScenarioTime))
	resourceCancel()
	<-resourceDone

	if _, _, err := network.queueSnapshot("c2s"); err != nil {
		t.Fatalf("c2s weak link error: %v", err)
	}
	if _, _, err := network.queueSnapshot("s2c"); err != nil {
		t.Fatalf("s2c weak link error: %v", err)
	}
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 0, 6*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, 0, 6*time.Second)
	assertNoP5ClientRuntimeError(t, client, "after weak-network HTTPS workload")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("weak-network workload replaced outer lane: got=%+v want=%+v", got, initialRef)
	}

	clientTransport, ok := client.rt.TransportStats(initialRef)
	if !ok {
		t.Fatal("client transport stats unavailable")
	}
	serverTransport, ok := serverTunnel.rt.TransportStats(serverTunnel.ref)
	if !ok {
		t.Fatal("server transport stats unavailable")
	}
	clientLane, ok := client.Owner().LaneStats(initialRef)
	if !ok {
		t.Fatal("client lane stats unavailable")
	}
	serverLane, ok := serverTunnel.owner.LaneStats(serverTunnel.ref)
	if !ok {
		t.Fatal("server lane stats unavailable")
	}
	clientPadding := client.Owner().Stats().Padding.PaddingBytes
	serverPadding := serverTunnel.owner.Stats().Padding.PaddingBytes
	paddingBytes := clientPadding + serverPadding
	if paddingBytes != 0 {
		t.Fatalf("weaknet atom must keep padding off, bytes=%d", paddingBytes)
	}
	if clientLane.TxPath.FECEnabled || serverLane.TxPath.FECEnabled {
		t.Fatal("weaknet atom unexpectedly enabled FEC")
	}

	outerScenarioC2S := network.wireSnapshot("c2s")
	outerScenarioS2C := network.wireSnapshot("s2c")
	outerScenarioTotal := outerScenarioC2S + outerScenarioS2C
	innerTotal := innerC2S + innerS2C
	if outerScenarioTotal == 0 || innerTotal == 0 {
		t.Fatalf("weaknet byte inventory outer=%d inner=%d", outerScenarioTotal, innerTotal)
	}
	wireAmplification := float64(outerScenarioTotal) / float64(innerTotal)

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
	for _, k := range keys {
		serverCountsCopy[strconv.Itoa(k)] = serverCounts[k]
	}
	serverCountsMu.Unlock()
	applicationDuplicates := 0
	for _, count := range serverCountsCopy {
		if count > 1 {
			applicationDuplicates += count - 1
		}
	}
	if applicationDuplicates != 0 {
		t.Fatalf("weaknet application duplicate requests=%d", applicationDuplicates)
	}
	if err := weakArtifacts.writeRequest(map[string]any{"schema": p5MeasurementSchema, "event": "application_inventory", "scheduled_requests": totalScheduled, "successful_requests": successes, "application_loss": totalScheduled - successes, "application_duplicates": applicationDuplicates, "server_request_counts": serverCountsCopy}); err != nil {
		t.Fatal(err)
	}

	repairRetransmits := clientTransport.Retransmitted + serverTransport.Retransmitted
	if err := weakArtifacts.writeResource(map[string]any{
		"schema": p5MeasurementSchema, "event": "final_inventory", "t_ns": p5WeakScenarioTime.Nanoseconds(),
		"fec_profile":        "off",
		"client_fec_enabled": clientLane.TxPath.FECEnabled, "server_fec_enabled": serverLane.TxPath.FECEnabled,
		"client_source_shards": clientLane.TxPath.Encoder.SourceShards, "client_parity_shards": clientLane.TxPath.Encoder.ParityShards,
		"client_reconstruction_events": clientLane.RxPath.Decoder.ReconstructionEvents, "client_recovered_sources": clientLane.RxPath.Decoder.RecoveredSources,
		"server_source_shards": serverLane.TxPath.Encoder.SourceShards, "server_parity_shards": serverLane.TxPath.Encoder.ParityShards,
		"server_reconstruction_events": serverLane.RxPath.Decoder.ReconstructionEvents, "server_recovered_sources": serverLane.RxPath.Decoder.RecoveredSources,
		"repair_retransmits": repairRetransmits, "padding_bytes": paddingBytes, "capture_loss_packets": 0,
		"outer_wire_bytes_c2s": outerScenarioC2S, "outer_wire_bytes_s2c": outerScenarioS2C,
		"inner_tls_wire_bytes_c2s": innerC2S, "inner_tls_wire_bytes_s2c": innerS2C,
		"wire_amplification_numerator_bytes": outerScenarioTotal, "wire_amplification_denominator_bytes": innerTotal, "wire_amplification": wireAmplification,
	}); err != nil {
		t.Fatal(err)
	}

	outerPackets, recordEvents, clientSYNs := recorder.counters()
	if clientSYNs != 1 {
		t.Fatalf("outer connection reuse violated: client initial SYN count=%d want=1", clientSYNs)
	}
	if recordEvents == 0 {
		t.Fatal("no steady TLS-like record events captured")
	}
	manifest := map[string]any{
		"schema": p5MeasurementSchema, "source_sha": sourceSHA, "harness_sha": sourceSHA, "branch": "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name": "controlled-real-https-weaknet-5-20-5", "run_ordinal": runOrdinal, "seed": seed, "cryptographic_rng": "system",
			"one_way_delay_ms": 300, "duration_seconds": 120, "phase_seconds": []int{30, 60, 30}, "drop_percent": []int{5, 20, 5},
			"drop_directions": []string{"c2s", "s2c"}, "drop_decision": "deterministic-per-direction-per-phase-permutation-mod-100",
			"injection_location":        "after runtimeentry SegmentIO emit capture and before peer memorySegmentEndpoint delivery",
			"application_arrival_model": "absolute-clock schedule every 3s; actual send lag recorded; no transport batching wait",
			"https_requests_scheduled":  totalScheduled, "response_size_cycle_bytes": p5WeakResponseSizes,
			"connection_mtu": 1500, "desired_lanes": 1, "fec_profile": "off", "fec_policy_reason": "keep production default fixed; no weak-network FEC tuning or profile competition", "production_default_fec": "off", "padding": "off",
			"post5_recovery_definition": "time from 90s transition to completion of the third consecutive successful scheduled HTTPS request in the final 5% phase",
			"rtt_sample_definition":     "HTTP request write start through complete verified response body read; inner TLS handshake reported separately",
		},
		"certificate_scenario": map[string]any{"name": certificate.name, "chain_id": certificate.chainID, "server_name": "target.test", "verified_chain_length": 3, "root_sha256": certificate.rootSHA256, "intermediate_sha256": certificate.intermediateSHA256, "leaf_sha256": certificate.leafSHA256},
		"application":          map[string]any{"scheduled_requests": totalScheduled, "successful_requests": successes, "loss": totalScheduled - successes, "duplicates": applicationDuplicates, "response_bytes": responseBytes, "inner_tls_wire_bytes_c2s": innerC2S, "inner_tls_wire_bytes_s2c": innerS2C},
		"capture":              map[string]any{"type": "hosted-segmentio-serialized-pcap-before-network-injector", "pcap": "outer.pcap", "events": "events.jsonl", "network_events": "network.jsonl", "request_samples": "requests.jsonl", "resource_samples": "resources.jsonl", "capture_loss_packets": 0, "capture_loss_method": "in-process SegmentIO emit interception; every runtime emit is synchronously serialized before deterministic network injection", "outer_packets": outerPackets, "tlslike_record_events": recordEvents, "client_initial_syns": clientSYNs},
		"transport":            map[string]any{"client": clientTransport, "server": serverTransport, "client_owner": client.Owner().Stats(), "server_owner": serverTunnel.owner.Stats(), "repair_retransmits": repairRetransmits, "padding_bytes": paddingBytes},
		"cost":                 map[string]any{"wire_amplification_scope": "120s pre-injection outer IPv4/TCP bytes over attempted inner TLS wire bytes", "wire_amplification_numerator_bytes": outerScenarioTotal, "wire_amplification_denominator_bytes": innerTotal, "wire_amplification": wireAmplification},
		"runner":               map[string]any{"go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "num_cpu": runtime.NumCPU(), "github_actions": os.Getenv("GITHUB_ACTIONS"), "runner_os": os.Getenv("RUNNER_OS"), "runner_arch": os.Getenv("RUNNER_ARCH"), "image_os": os.Getenv("ImageOS")},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(artifactDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("WBD_P5_WEAKNET_5205_CAPTURED source_sha=%s run=%d seed=%d duration=120s delay=300ms drop=5-20-5 fec=off padding=off scheduled=%d success=%d loss=%d repair=%d wire_amp=%.6f\n", sourceSHA, runOrdinal, seed, totalScheduled, successes, totalScheduled-successes, repairRetransmits, wireAmplification)
}
