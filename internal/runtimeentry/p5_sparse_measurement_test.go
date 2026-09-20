package runtimeentry

import (
	"bufio"
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

var p5SparseScheduledIdle = []time.Duration{0, 150 * time.Millisecond, 300 * time.Millisecond}

type p5SparseMeasurementEvent struct {
	Schema string `json:"schema"`
	Event  string `json:"event"`
	TNS    int64  `json:"t_ns"`

	RequestOrdinal int    `json:"request_ordinal,omitempty"`
	RequestCount   int    `json:"request_count,omitempty"`
	BusinessFlowID uint64 `json:"business_flow_id,omitempty"`
	OuterConnection int   `json:"outer_connection_id,omitempty"`

	ScheduledGapNS   int64 `json:"scheduled_gap_ns,omitempty"`
	ApplicationIdleNS int64 `json:"application_idle_ns,omitempty"`
	RequestStartNS    int64 `json:"request_start_ns,omitempty"`
	RequestCompleteNS int64 `json:"request_complete_ns,omitempty"`
	BusinessLatencyNS int64 `json:"business_latency_ns,omitempty"`

	HTTPStatus    int    `json:"http_status,omitempty"`
	ResponseBytes int   `json:"response_bytes,omitempty"`
	WireBytesC2S  uint64 `json:"wire_bytes_c2s,omitempty"`
	WireBytesS2C  uint64 `json:"wire_bytes_s2c,omitempty"`

	FlowClosed     bool   `json:"flow_closed,omitempty"`
	HandshakeNS    int64  `json:"handshake_ns,omitempty"`
	TLSVersion     uint16 `json:"tls_version,omitempty"`
	TLSCipherSuite uint16 `json:"tls_cipher_suite,omitempty"`
	HandshakeMode  string `json:"handshake_mode,omitempty"`
	TLSResumed     *bool  `json:"tls_resumed,omitempty"`

	CertificateScenario       string `json:"certificate_scenario,omitempty"`
	CertificateChainID        string `json:"certificate_chain_id,omitempty"`
	CertificateVerified       bool   `json:"certificate_verified,omitempty"`
	VerifiedChainLength       int    `json:"verified_chain_length,omitempty"`
	VerifiedRootSHA256        string `json:"verified_root_sha256,omitempty"`
	VerifiedIntermediateSHA256 string `json:"verified_intermediate_sha256,omitempty"`
	VerifiedLeafSHA256        string `json:"verified_leaf_sha256,omitempty"`
}

func (r *p5MeasurementRecorder) recordSparseEvent(kind string, ev p5SparseMeasurementEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev.Schema = p5MeasurementSchema
	ev.Event = kind
	ev.TNS = time.Now().Sub(r.start).Nanoseconds()
	enc := json.NewEncoder(r.events)
	enc.SetEscapeHTML(false)
	return enc.Encode(ev)
}

func TestP5SparseSingleConnectionMeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_SPARSE_MEASURE") != "1" {
		t.Skip("P5 sparse measurement harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_SPARSE_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_SPARSE_ARTIFACT_DIR is required")
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

	certificate := newP5CertificateScenario(t, "controlled-sparse-chain", "sparse-chain", 3000)
	defer certificate.server.Close()

	outerCert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("p5-sparse-https"))
	var installation logicaltunnel.InstallationID
	installation[0] = 6
	lease := logicaltunnel.Lease{
		Account:        "p5-sparse",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.66.0.6/32",
			Routes4:  []string{"0.0.0.0/0"},
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
			ExpectedUsername: "p5-sparse",
			ExpectedPassword: "sparse-password",
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
			LocalIP: [4]byte{192, 0, 2, 51},
			PeerIP: [4]byte{192, 0, 2, 60},
			LocalPort: 42100,
			PeerPort: 443,
		},
		ClientISN: 6000,
		InitialRTO: time.Second,
		Lease: lease,
		MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				Timeout: 3 * time.Second,
			},
			Username: "p5-sparse",
			Password: "sparse-password",
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
				return errors.New("p5 sparse: client service not attached")
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

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		accepted <- acceptResult{conn: conn, err: err}
	}()
	appConn, err := net.DialTimeout("tcp4", listener.Addr().String(), 2*time.Second)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	defer appConn.Close()
	peer := <-accepted
	_ = listener.Close()
	if peer.err != nil {
		t.Fatal(peer.err)
	}

	flowStarted := time.Now()
	flowID, err := svc.AddTCP(peer.conn, certificate.targetAddr, flowStarted)
	if err != nil {
		_ = peer.conn.Close()
		t.Fatal(err)
	}

	tlsConn := tls.Client(appConn, certificate.tlsConfig.Clone())
	hsStarted := time.Now()
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	handshakeNS := time.Since(hsStarted).Nanoseconds()
	tlsState := tlsConn.ConnectionState()
	if tlsState.Version != tls.VersionTLS13 {
		t.Fatalf("inner TLS version=0x%x want TLS1.3", tlsState.Version)
	}
	if tlsState.DidResume {
		t.Fatal("sparse single-connection scenario must start with a full TLS handshake")
	}
	certificate.verifyState(t, tlsState)

	reader := bufio.NewReader(tlsConn)
	var previousComplete time.Time
	var totalResponseBytes int
	var totalRequestWireC2S uint64
	var totalRequestWireS2C uint64
	for i, scheduledGap := range p5SparseScheduledIdle {
		ordinal := i + 1
		if !previousComplete.IsZero() {
			waitP5SparseApplicationDeadline(previousComplete.Add(scheduledGap))
		}
		arrival := time.Now()
		actualIdle := int64(0)
		if !previousComplete.IsZero() {
			actualIdle = arrival.Sub(previousComplete).Nanoseconds()
		}
		beforeC2S, beforeS2C := recorder.wireSnapshot()
		requestStartNS := arrival.Sub(recorder.start).Nanoseconds()

		reqURL := &url.URL{
			Scheme: "https",
			Host: certificate.targetAddr.String(),
			Path: fmt.Sprintf("/sparse/%d", ordinal),
		}
		req := &http.Request{
			Method: http.MethodGet,
			URL: reqURL,
			Host: "target.test",
			Header: make(http.Header),
		}
		if err := req.Write(tlsConn); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		complete := time.Now()
		requestCompleteNS := complete.Sub(recorder.start).Nanoseconds()
		wantBody := fmt.Sprintf("wbd-p5 path=/sparse/%d", ordinal)
		if string(body) != wantBody {
			t.Fatalf("sparse HTTPS body mismatch got=%q want=%q", body, wantBody)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("sparse HTTPS status=%d", resp.StatusCode)
		}
		if svc.TCPFlows() != 1 || serverTunnel.service.TCPFlows() != 1 {
			t.Fatalf("sparse persistent flow not alive after request %d: client=%d server=%d",
				ordinal, svc.TCPFlows(), serverTunnel.service.TCPFlows())
		}
		afterC2S, afterS2C := recorder.wireSnapshot()
		wireC2S := afterC2S - beforeC2S
		wireS2C := afterS2C - beforeS2C
		totalResponseBytes += len(body)
		totalRequestWireC2S += wireC2S
		totalRequestWireS2C += wireS2C

		if err := recorder.recordSparseEvent("https_sparse_request", p5SparseMeasurementEvent{
			RequestOrdinal: ordinal,
			BusinessFlowID: flowID,
			OuterConnection: 1,
			ScheduledGapNS: scheduledGap.Nanoseconds(),
			ApplicationIdleNS: actualIdle,
			RequestStartNS: requestStartNS,
			RequestCompleteNS: requestCompleteNS,
			BusinessLatencyNS: complete.Sub(arrival).Nanoseconds(),
			HTTPStatus: resp.StatusCode,
			ResponseBytes: len(body),
			WireBytesC2S: wireC2S,
			WireBytesS2C: wireS2C,
		}); err != nil {
			t.Fatal(err)
		}
		previousComplete = complete
	}

	_ = tlsConn.Close()
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 0, 2*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, 0, 2*time.Second)
	assertNoP5ClientRuntimeError(t, client, "after sparse HTTPS connection close")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("sparse HTTPS connection replaced outer lane: got=%+v want=%+v", got, initialRef)
	}

	didResume := tlsState.DidResume
	if err := recorder.recordSparseEvent("https_sparse_connection", p5SparseMeasurementEvent{
		RequestCount: len(p5SparseScheduledIdle),
		BusinessFlowID: flowID,
		OuterConnection: 1,
		FlowClosed: true,
		HandshakeNS: handshakeNS,
		TLSVersion: tlsState.Version,
		TLSCipherSuite: tlsState.CipherSuite,
		HandshakeMode: "full",
		TLSResumed: &didResume,
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

	outerPackets, recordEvents, clientSYNs := recorder.counters()
	if clientSYNs != 1 {
		t.Fatalf("outer connection reuse violated: client initial SYN count=%d want=1", clientSYNs)
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

	scheduledIdleNS := make([]int64, 0, len(p5SparseScheduledIdle))
	for _, gap := range p5SparseScheduledIdle {
		scheduledIdleNS = append(scheduledIdleNS, gap.Nanoseconds())
	}
	manifest := map[string]any{
		"schema": p5MeasurementSchema,
		"source_sha": sourceSHA,
		"harness_sha": sourceSHA,
		"branch": "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name": "controlled-real-https-sparse-single-connection",
			"seed": 20260921,
			"cryptographic_rng": "system",
			"https_connections": 1,
			"https_requests": len(p5SparseScheduledIdle),
			"business_flow_count": 1,
			"outer_connection_count": 1,
			"max_concurrent_business_flows": 1,
			"application_arrival_model": "explicit-deadline-timer-after-previous-response",
			"scheduled_idle_ns": scheduledIdleNS,
			"transport_batch_wait": "none",
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
			"requests": len(p5SparseScheduledIdle),
			"responses": len(p5SparseScheduledIdle),
			"loss": 0,
			"duplicates": 0,
			"response_bytes": totalResponseBytes,
			"request_wire_bytes_c2s": totalRequestWireC2S,
			"request_wire_bytes_s2c": totalRequestWireS2C,
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

	fmt.Printf("WBD_P5_SPARSE_SINGLE_CONNECTION_CAPTURED source_sha=%s requests=3 business_flows=1 outer_connections=1 tls=full fec=off padding=off records=%d\n",
		sourceSHA, recordEvents)
}

func waitP5SparseApplicationDeadline(deadline time.Time) {
	if delay := time.Until(deadline); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
	}
}
