package runtimeentry

import (
	"bufio"
	"bytes"
	"context"
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

const (
	p5ConcurrentFlows        = 3
	p5ConcurrentResponseSize = 128 * 1024
)

type p5ConcurrentMeasurementEvent struct {
	Schema string `json:"schema"`
	Event  string `json:"event"`
	TNS    int64  `json:"t_ns"`

	FlowOrdinal    int    `json:"flow_ordinal,omitempty"`
	BusinessFlowID uint64 `json:"business_flow_id,omitempty"`
	OuterConnection int   `json:"outer_connection_id,omitempty"`

	RequestStartNS    int64 `json:"request_start_ns,omitempty"`
	RequestCompleteNS int64 `json:"request_complete_ns,omitempty"`
	BusinessLatencyNS int64 `json:"business_latency_ns,omitempty"`
	HTTPStatus         int   `json:"http_status,omitempty"`
	ResponseBytes      int   `json:"response_bytes,omitempty"`

	InnerTLSWireBytesC2S uint64 `json:"inner_tls_wire_bytes_c2s,omitempty"`
	InnerTLSWireBytesS2C uint64 `json:"inner_tls_wire_bytes_s2c,omitempty"`

	HandshakeNS    int64  `json:"handshake_ns,omitempty"`
	TLSVersion     uint16 `json:"tls_version,omitempty"`
	TLSCipherSuite uint16 `json:"tls_cipher_suite,omitempty"`
	HandshakeMode  string `json:"handshake_mode,omitempty"`
	TLSResumed     *bool  `json:"tls_resumed,omitempty"`
	FlowClosed     bool   `json:"flow_closed,omitempty"`

	CertificateScenario         string `json:"certificate_scenario,omitempty"`
	CertificateChainID          string `json:"certificate_chain_id,omitempty"`
	CertificateVerified         bool   `json:"certificate_verified,omitempty"`
	VerifiedChainLength         int    `json:"verified_chain_length,omitempty"`
	VerifiedRootSHA256          string `json:"verified_root_sha256,omitempty"`
	VerifiedIntermediateSHA256  string `json:"verified_intermediate_sha256,omitempty"`
	VerifiedLeafSHA256          string `json:"verified_leaf_sha256,omitempty"`

	RequestCount                     int    `json:"request_count,omitempty"`
	BarrierReleaseNS                 int64  `json:"barrier_release_ns,omitempty"`
	ApplicationStartModel            string `json:"application_start_model,omitempty"`
	MaxConcurrentBusinessFlowsClient int    `json:"max_concurrent_business_flows_client,omitempty"`
	MaxConcurrentBusinessFlowsServer int    `json:"max_concurrent_business_flows_server,omitempty"`
	MaxOverlappingRequestIntervals   int    `json:"max_overlapping_request_intervals,omitempty"`
}

type p5CountingConn struct {
	net.Conn
	mu      sync.Mutex
	read    uint64
	written uint64
}

func (c *p5CountingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.mu.Lock()
		c.read += uint64(n)
		c.mu.Unlock()
	}
	return n, err
}

func (c *p5CountingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.mu.Lock()
		c.written += uint64(n)
		c.mu.Unlock()
	}
	return n, err
}

func (c *p5CountingConn) snapshot() (read, written uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.read, c.written
}

type p5ConcurrentReady struct {
	ordinal int
	flowID  uint64
	err     error
}

type p5ConcurrentResult struct {
	ordinal int
	flowID  uint64
	startNS int64
	completeNS int64
	latencyNS int64
	status int
	responseBytes int
	wireC2S uint64
	wireS2C uint64
	handshakeNS int64
	state tls.ConnectionState
	err error
}

func (r *p5MeasurementRecorder) recordConcurrentEvent(kind string, ev p5ConcurrentMeasurementEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev.Schema = p5MeasurementSchema
	ev.Event = kind
	ev.TNS = time.Now().Sub(r.start).Nanoseconds()
	enc := json.NewEncoder(r.events)
	enc.SetEscapeHTML(false)
	return enc.Encode(ev)
}

func TestP5NaturalConcurrencyMeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_CONCURRENT_MEASURE") != "1" {
		t.Skip("P5 natural concurrency harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_CONCURRENT_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_CONCURRENT_ARTIFACT_DIR is required")
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

	responsePayload := bytes.Repeat([]byte{'x'}, p5ConcurrentResponseSize)
	certificate := newP5CertificateScenarioWithHandler(t, "controlled-concurrent-chain", "concurrent-chain", 4000, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-WBD-Path", req.URL.Path)
		_, _ = w.Write(responsePayload)
	}))
	defer certificate.server.Close()

	outerCert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("p5-concurrent"))
	var installation logicaltunnel.InstallationID
	installation[0] = 7
	lease := logicaltunnel.Lease{
		Account:        "p5-concurrent",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.66.0.7/32",
			Routes4: []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}

	clientEP, serverEP := memorySegmentPair()
	serverTUN := newMemoryPacketWriter()
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 8, serverTUN)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{
		IO: recorder.wrapIO("s2c", serverEP.io()),
		ListenPort: 443,
		MaxAssociations: 16,
		InitialRTO: time.Second,
		TickInterval: 10 * time.Millisecond,
		MaxFlows: 64,
		Admission: realityfront.ServerAdmissionConfig{
			TLS: realityfront.ServerConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				TLSConfig: &tls.Config{Certificates: []tls.Certificate{outerCert}},
				Timeout: 3 * time.Second,
			},
			ExpectedUsername: "p5-concurrent",
			ExpectedPassword: "concurrent-password",
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
		IO: recorder.wrapIO("c2s", clientEP.io()),
		Flow: faketcp.ClientFlow{
			LocalIP: [4]byte{192, 0, 2, 52},
			PeerIP: [4]byte{192, 0, 2, 60},
			LocalPort: 42200,
			PeerPort: 443,
		},
		ClientISN: 7000,
		InitialRTO: time.Second,
		Lease: lease,
		MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				Timeout: 3 * time.Second,
			},
			Username: "p5-concurrent",
			Password: "concurrent-password",
			TunnelID: tunnelID.Bytes(),
			ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU: 1500,
			TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		},
		Deliver: func(packets [][]byte, now time.Time) error {
			serviceMu.RLock()
			svc := clientService
			serviceMu.RUnlock()
			if svc == nil {
				return errors.New("p5 concurrent: client service not attached")
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
	svc, err := platformflow.NewClient(channel, platformflow.DefaultClientConfig(), func(_, _ netip.AddrPort, _ []byte) error {
		return nil
	})
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
	recorder.markSteady()
	initialRef := client.Ref()
	if initialRef.ID != 1 || initialRef.Generation == 0 {
		t.Fatalf("unexpected initial lane ref: %+v", initialRef)
	}

	start := make(chan struct{})
	ready := make(chan p5ConcurrentReady, p5ConcurrentFlows)
	results := make(chan p5ConcurrentResult, p5ConcurrentFlows)
	var activeRequests atomic.Int64
	var maxActiveRequests atomic.Int64
	for ordinal := 1; ordinal <= p5ConcurrentFlows; ordinal++ {
		go runP5ConcurrentWorker(recorder, svc, certificate, ordinal, start, ready, results, &activeRequests, &maxActiveRequests)
	}

	flowIDs := make(map[uint64]struct{}, p5ConcurrentFlows)
	var setupErr error
	for i := 0; i < p5ConcurrentFlows; i++ {
		item := <-ready
		if item.err != nil {
			setupErr = errors.Join(setupErr, item.err)
			continue
		}
		if item.flowID == 0 {
			setupErr = errors.Join(setupErr, fmt.Errorf("flow %d has zero business id", item.ordinal))
			continue
		}
		flowIDs[item.flowID] = struct{}{}
	}
	if setupErr != nil {
		close(start)
		t.Fatal(setupErr)
	}
	if len(flowIDs) != p5ConcurrentFlows {
		close(start)
		t.Fatalf("distinct business flows=%d want=%d", len(flowIDs), p5ConcurrentFlows)
	}
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, p5ConcurrentFlows, 2*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, p5ConcurrentFlows, 2*time.Second)
	maxClientFlows := svc.TCPFlows()
	maxServerFlows := serverTunnel.service.TCPFlows()
	barrierReleaseNS := time.Now().Sub(recorder.start).Nanoseconds()
	close(start)

	collected := make([]p5ConcurrentResult, 0, p5ConcurrentFlows)
	for i := 0; i < p5ConcurrentFlows; i++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		collected = append(collected, result)
	}
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 0, 3*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, 0, 3*time.Second)
	assertNoP5ClientRuntimeError(t, client, "after natural concurrency flows")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("natural concurrency replaced outer lane: got=%+v want=%+v", got, initialRef)
	}

	sort.Slice(collected, func(i, j int) bool { return collected[i].ordinal < collected[j].ordinal })
	maxOverlap := p5ConcurrentMaxOverlap(collected)
	if maxOverlap < 2 {
		t.Fatalf("request intervals did not naturally overlap: max=%d", maxOverlap)
	}
	if got := int(maxActiveRequests.Load()); got < 2 {
		t.Fatalf("active-request counter never observed concurrency: max=%d", got)
	}
	if maxClientFlows != p5ConcurrentFlows || maxServerFlows != p5ConcurrentFlows {
		t.Fatalf("max concurrent business flow registry client=%d server=%d want=%d", maxClientFlows, maxServerFlows, p5ConcurrentFlows)
	}

	for _, result := range collected {
		certificate.verifyState(t, result.state)
		if result.state.Version != tls.VersionTLS13 || result.state.DidResume {
			t.Fatalf("flow %d TLS state version=0x%x resumed=%v", result.ordinal, result.state.Version, result.state.DidResume)
		}
		if result.responseBytes != p5ConcurrentResponseSize || result.status != http.StatusOK {
			t.Fatalf("flow %d response status=%d bytes=%d", result.ordinal, result.status, result.responseBytes)
		}
		if result.wireC2S == 0 || result.wireS2C == 0 {
			t.Fatalf("flow %d inner TLS wire bytes c2s=%d s2c=%d", result.ordinal, result.wireC2S, result.wireS2C)
		}
		didResume := result.state.DidResume
		if err := recorder.recordConcurrentEvent("https_concurrent_flow", p5ConcurrentMeasurementEvent{
			FlowOrdinal: result.ordinal,
			BusinessFlowID: result.flowID,
			OuterConnection: 1,
			RequestStartNS: result.startNS,
			RequestCompleteNS: result.completeNS,
			BusinessLatencyNS: result.latencyNS,
			HTTPStatus: result.status,
			ResponseBytes: result.responseBytes,
			InnerTLSWireBytesC2S: result.wireC2S,
			InnerTLSWireBytesS2C: result.wireS2C,
			HandshakeNS: result.handshakeNS,
			TLSVersion: result.state.Version,
			TLSCipherSuite: result.state.CipherSuite,
			HandshakeMode: "full",
			TLSResumed: &didResume,
			FlowClosed: true,
			CertificateScenario: certificate.name,
			CertificateChainID: certificate.chainID,
			CertificateVerified: true,
			VerifiedChainLength: 3,
			VerifiedRootSHA256: certificate.rootSHA256,
			VerifiedIntermediateSHA256: certificate.intermediateSHA256,
			VerifiedLeafSHA256: certificate.leafSHA256,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.recordConcurrentEvent("https_concurrency_summary", p5ConcurrentMeasurementEvent{
		RequestCount: p5ConcurrentFlows,
		OuterConnection: 1,
		BarrierReleaseNS: barrierReleaseNS,
		ApplicationStartModel: "barrier-release-after-all-handshakes-ready",
		MaxConcurrentBusinessFlowsClient: maxClientFlows,
		MaxConcurrentBusinessFlowsServer: maxServerFlows,
		MaxOverlappingRequestIntervals: maxOverlap,
	}); err != nil {
		t.Fatal(err)
	}

	outerPackets, recordEvents, clientSYNs := recorder.counters()
	if clientSYNs != 1 {
		t.Fatalf("outer connection reuse violated: initial SYN count=%d want=1", clientSYNs)
	}
	if recordEvents == 0 {
		t.Fatal("no steady TLS-like record events captured")
	}
	clientTransport, ok := client.rt.TransportStats(client.Ref())
	if !ok {
		t.Fatal("client transport stats unavailable")
	}
	serverTransport, ok := serverTunnel.rt.TransportStats(serverTunnel.ref)
	if !ok {
		t.Fatal("server transport stats unavailable")
	}
	finalC2S, finalS2C := recorder.wireSnapshot()

	manifest := map[string]any{
		"schema": p5MeasurementSchema,
		"source_sha": sourceSHA,
		"harness_sha": sourceSHA,
		"branch": "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name": "controlled-real-https-natural-concurrency",
			"seed": 20260921,
			"cryptographic_rng": "system",
			"https_connections": p5ConcurrentFlows,
			"https_requests": p5ConcurrentFlows,
			"business_flow_count": p5ConcurrentFlows,
			"outer_connection_count": 1,
			"application_start_model": "barrier-release-after-all-handshakes-ready",
			"fixed_sleep": false,
			"transport_batch_wait": "none",
			"response_body_bytes": p5ConcurrentResponseSize,
			"max_concurrent_business_flows_client": maxClientFlows,
			"max_concurrent_business_flows_server": maxServerFlows,
			"max_overlapping_request_intervals": maxOverlap,
			"per_flow_wire_scope": "inner-tls-ciphertext-counted-at-application-socket",
			"connection_mtu": 1500,
			"desired_lanes": 1,
			"fec_parity_shards": 0,
			"padding": "off",
			"network_injection": "none",
			"burst_gap_ns": p5BurstGap.Nanoseconds(),
			"tls_version": "TLS1.3",
			"handshake_mode": "full",
			"certificate_chain_id": certificate.chainID,
		},
		"certificate_scenario": map[string]any{
			"name": certificate.name,
			"chain_id": certificate.chainID,
			"server_name": "target.test",
			"verified_chain_length": 3,
			"root_sha256": certificate.rootSHA256,
			"intermediate_sha256": certificate.intermediateSHA256,
			"leaf_sha256": certificate.leafSHA256,
		},
		"application": map[string]any{
			"requests": p5ConcurrentFlows,
			"responses": p5ConcurrentFlows,
			"loss": 0,
			"duplicates": 0,
			"max_overlapping_request_intervals": maxOverlap,
			"response_bytes_per_flow": p5ConcurrentResponseSize,
		},
		"capture": map[string]any{
			"type": "hosted-segmentio-serialized-pcap",
			"pcap": "outer.pcap",
			"events": "events.jsonl",
			"capture_loss_packets": 0,
			"capture_loss_method": "in-process SegmentIO emit interception; no kernel capture queue",
			"network_drop_injected": 0,
			"outer_packets": outerPackets,
			"tlslike_record_events": recordEvents,
			"client_initial_syns": clientSYNs,
			"outer_wire_bytes_c2s": finalC2S,
			"outer_wire_bytes_s2c": finalS2C,
		},
		"transport": map[string]any{
			"client": clientTransport,
			"server": serverTransport,
			"client_owner": client.Owner().Stats(),
			"server_owner": serverTunnel.owner.Stats(),
			"fec_recovery": 0,
			"padding_bytes": 0,
			"repair_retransmits": clientTransport.Retransmitted + serverTransport.Retransmitted,
		},
		"runner": map[string]any{
			"go_version": runtime.Version(),
			"goos": runtime.GOOS,
			"goarch": runtime.GOARCH,
			"num_cpu": runtime.NumCPU(),
			"github_actions": os.Getenv("GITHUB_ACTIONS"),
			"runner_os": os.Getenv("RUNNER_OS"),
			"runner_arch": os.Getenv("RUNNER_ARCH"),
			"image_os": os.Getenv("ImageOS"),
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

	fmt.Printf("WBD_P5_NATURAL_CONCURRENCY_CAPTURED source_sha=%s flows=3 outer_connections=1 max_business_flows=3 max_overlapping_requests=%d tls=full fec=off padding=off records=%d\n",
		sourceSHA, maxOverlap, recordEvents)
}

func runP5ConcurrentWorker(
	recorder *p5MeasurementRecorder,
	svc *platformflow.Client,
	certificate *p5CertificateScenario,
	ordinal int,
	start <-chan struct{},
	ready chan<- p5ConcurrentReady,
	results chan<- p5ConcurrentResult,
	active, maxActive *atomic.Int64,
) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		ready <- p5ConcurrentReady{ordinal: ordinal, err: err}
		return
	}
	type acceptResult struct {
		conn net.Conn
		err error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		accepted <- acceptResult{conn: conn, err: err}
	}()
	appConn, err := net.DialTimeout("tcp4", listener.Addr().String(), 2*time.Second)
	if err != nil {
		_ = listener.Close()
		ready <- p5ConcurrentReady{ordinal: ordinal, err: err}
		return
	}
	peer := <-accepted
	_ = listener.Close()
	if peer.err != nil {
		_ = appConn.Close()
		ready <- p5ConcurrentReady{ordinal: ordinal, err: peer.err}
		return
	}

	flowID, err := svc.AddTCP(peer.conn, certificate.targetAddr, time.Now())
	if err != nil {
		_ = appConn.Close()
		_ = peer.conn.Close()
		ready <- p5ConcurrentReady{ordinal: ordinal, err: err}
		return
	}

	counting := &p5CountingConn{Conn: appConn}
	tlsCfg := certificate.tlsConfig.Clone()
	tlsCfg.ClientSessionCache = nil
	tlsConn := tls.Client(counting, tlsCfg)
	hsStarted := time.Now()
	if err := tlsConn.Handshake(); err != nil {
		_ = tlsConn.Close()
		ready <- p5ConcurrentReady{ordinal: ordinal, flowID: flowID, err: err}
		return
	}
	handshakeNS := time.Since(hsStarted).Nanoseconds()
	state := tlsConn.ConnectionState()
	if state.Version != tls.VersionTLS13 || state.DidResume {
		_ = tlsConn.Close()
		ready <- p5ConcurrentReady{ordinal: ordinal, flowID: flowID, err: fmt.Errorf("flow %d TLS version=0x%x resumed=%v", ordinal, state.Version, state.DidResume)}
		return
	}
	ready <- p5ConcurrentReady{ordinal: ordinal, flowID: flowID}
	<-start

	beforeRead, beforeWritten := counting.snapshot()
	requestStarted := time.Now()
	current := active.Add(1)
	p5AtomicMax(maxActive, current)

	reqURL := &url.URL{
		Scheme: "https",
		Host: certificate.targetAddr.String(),
		Path: fmt.Sprintf("/concurrent/%d", ordinal),
	}
	req := &http.Request{
		Method: http.MethodGet,
		URL: reqURL,
		Host: "target.test",
		Header: make(http.Header),
	}
	req.Header.Set("Connection", "close")
	err = req.Write(tlsConn)
	var status int
	var responseBytes int
	if err == nil {
		resp, readErr := http.ReadResponse(bufio.NewReader(tlsConn), req)
		if readErr != nil {
			err = readErr
		} else {
			status = resp.StatusCode
			body, bodyErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if bodyErr != nil {
				err = bodyErr
			} else {
				responseBytes = len(body)
			}
		}
	}
	requestComplete := time.Now()
	active.Add(-1)
	_ = tlsConn.Close()
	afterRead, afterWritten := counting.snapshot()

	results <- p5ConcurrentResult{
		ordinal: ordinal,
		flowID: flowID,
		startNS: requestStarted.Sub(recorder.start).Nanoseconds(),
		completeNS: requestComplete.Sub(recorder.start).Nanoseconds(),
		latencyNS: requestComplete.Sub(requestStarted).Nanoseconds(),
		status: status,
		responseBytes: responseBytes,
		wireC2S: afterWritten - beforeWritten,
		wireS2C: afterRead - beforeRead,
		handshakeNS: handshakeNS,
		state: state,
		err: err,
	}
}

func p5AtomicMax(dst *atomic.Int64, value int64) {
	for {
		current := dst.Load()
		if value <= current || dst.CompareAndSwap(current, value) {
			return
		}
	}
}

func p5ConcurrentMaxOverlap(results []p5ConcurrentResult) int {
	maxOverlap := 0
	for _, pivot := range results {
		active := 0
		for _, candidate := range results {
			if candidate.startNS <= pivot.startNS && pivot.startNS < candidate.completeNS {
				active++
			}
		}
		if active > maxOverlap {
			maxOverlap = active
		}
	}
	return maxOverlap
}
