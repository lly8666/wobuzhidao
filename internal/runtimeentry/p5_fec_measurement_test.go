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
	p5FECParity       = 10
	p5FECResponseSize = 256 * 1024
)

type p5FECMeasurementEvent struct {
	Schema string `json:"schema"`
	Event  string `json:"event"`
	TNS    int64  `json:"t_ns"`

	BusinessFlowID  uint64 `json:"business_flow_id,omitempty"`
	OuterConnection int    `json:"outer_connection_id,omitempty"`

	RequestStartNS    int64 `json:"request_start_ns,omitempty"`
	RequestCompleteNS int64 `json:"request_complete_ns,omitempty"`
	BusinessLatencyNS int64 `json:"business_latency_ns,omitempty"`
	HandshakeNS       int64 `json:"handshake_ns,omitempty"`
	HTTPStatus        int   `json:"http_status,omitempty"`
	ResponseBytes     int   `json:"response_bytes,omitempty"`

	InnerTLSWireBytesC2S uint64 `json:"inner_tls_wire_bytes_c2s,omitempty"`
	InnerTLSWireBytesS2C uint64 `json:"inner_tls_wire_bytes_s2c,omitempty"`
	OuterWireBytesC2S    uint64 `json:"outer_wire_bytes_c2s,omitempty"`
	OuterWireBytesS2C    uint64 `json:"outer_wire_bytes_s2c,omitempty"`

	TLSVersion     uint16 `json:"tls_version,omitempty"`
	TLSCipherSuite uint16 `json:"tls_cipher_suite,omitempty"`
	HandshakeMode  string `json:"handshake_mode,omitempty"`
	TLSResumed     *bool  `json:"tls_resumed,omitempty"`

	CertificateScenario        string `json:"certificate_scenario,omitempty"`
	CertificateChainID         string `json:"certificate_chain_id,omitempty"`
	CertificateVerified        bool   `json:"certificate_verified,omitempty"`
	VerifiedChainLength        int    `json:"verified_chain_length,omitempty"`
	VerifiedRootSHA256         string `json:"verified_root_sha256,omitempty"`
	VerifiedIntermediateSHA256 string `json:"verified_intermediate_sha256,omitempty"`
	VerifiedLeafSHA256         string `json:"verified_leaf_sha256,omitempty"`

	FECParityShards int   `json:"fec_parity_shards,omitempty"`
	FECFlushAfterNS int64 `json:"fec_flush_after_ns,omitempty"`
	FECMaxBlocks    int   `json:"fec_max_blocks,omitempty"`

	ClientSourceShards          uint64 `json:"client_source_shards,omitempty"`
	ClientParityShards          uint64 `json:"client_parity_shards,omitempty"`
	ClientFullBlocks            uint64 `json:"client_full_blocks,omitempty"`
	ClientPartialBlocks         uint64 `json:"client_partial_blocks,omitempty"`
	ClientPendingSources        int    `json:"client_pending_sources"`
	ClientReconstructionEvents  uint64 `json:"client_reconstruction_events"`
	ClientRecoveredSources      uint64 `json:"client_recovered_sources"`

	ServerSourceShards          uint64 `json:"server_source_shards,omitempty"`
	ServerParityShards          uint64 `json:"server_parity_shards,omitempty"`
	ServerFullBlocks            uint64 `json:"server_full_blocks,omitempty"`
	ServerPartialBlocks         uint64 `json:"server_partial_blocks,omitempty"`
	ServerPendingSources        int    `json:"server_pending_sources"`
	ServerReconstructionEvents  uint64 `json:"server_reconstruction_events"`
	ServerRecoveredSources      uint64 `json:"server_recovered_sources"`

	RepairRetransmits uint64 `json:"repair_retransmits"`
	PaddingBytes      uint64 `json:"padding_bytes"`

	WireAmplificationNumeratorBytes   uint64  `json:"wire_amplification_numerator_bytes,omitempty"`
	WireAmplificationDenominatorBytes uint64  `json:"wire_amplification_denominator_bytes,omitempty"`
	WireAmplification                 float64 `json:"wire_amplification,omitempty"`
}

func (r *p5MeasurementRecorder) recordFECEvent(kind string, ev p5FECMeasurementEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev.Schema = p5MeasurementSchema
	ev.Event = kind
	ev.TNS = time.Now().Sub(r.start).Nanoseconds()
	enc := json.NewEncoder(r.events)
	enc.SetEscapeHTML(false)
	return enc.Encode(ev)
}

func TestP5FixedFECMeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_FEC_MEASURE") != "1" {
		t.Skip("P5 fixed-FEC harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_FEC_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_FEC_ARTIFACT_DIR is required")
	}
	sourceSHA := os.Getenv("GITHUB_SHA")
	if len(sourceSHA) != 40 {
		t.Fatalf("GITHUB_SHA must be an exact 40-hex source SHA, got %q", sourceSHA)
	}
	flushAfter, maxBlocks, err := datapath.FixedFECRuntimeDefaults(p5FECParity)
	if err != nil {
		t.Fatal(err)
	}
	if flushAfter != 8*time.Millisecond || maxBlocks != 8 {
		t.Fatalf("fixed FEC runtime defaults changed: flush=%s max_blocks=%d", flushAfter, maxBlocks)
	}

	recorder, err := newP5MeasurementRecorder(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.close()

	responsePayload := bytes.Repeat([]byte{'f'}, p5FECResponseSize)
	certificate := newP5CertificateScenarioWithHandler(t, "controlled-fec-chain", "fec-chain", 5000, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-WBD-Path", req.URL.Path)
		_, _ = w.Write(responsePayload)
	}))
	defer certificate.server.Close()

	outerCert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("p5-fec-fixed"))
	var installation logicaltunnel.InstallationID
	installation[0] = 8
	lease := logicaltunnel.Lease{
		Account:        "p5-fec",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.66.0.8/32",
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
		TickInterval: 2 * time.Millisecond,
		MaxFlows: 64,
		Admission: realityfront.ServerAdmissionConfig{
			TLS: realityfront.ServerConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				TLSConfig: &tls.Config{Certificates: []tls.Certificate{outerCert}},
				Timeout: 3 * time.Second,
			},
			ExpectedUsername: "p5-fec",
			ExpectedPassword: "fec-password",
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
			ParityShards: p5FECParity, FlushAfter: flushAfter, MaxBlocks: maxBlocks,
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
			LocalIP: [4]byte{192, 0, 2, 53},
			PeerIP: [4]byte{192, 0, 2, 60},
			LocalPort: 42300,
			PeerPort: 443,
		},
		ClientISN: 8000,
		InitialRTO: time.Second,
		Lease: lease,
		MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				Timeout: 3 * time.Second,
			},
			Username: "p5-fec",
			Password: "fec-password",
			TunnelID: tunnelID.Bytes(),
			ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU: 1500,
			TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
			ParityShards: p5FECParity, FlushAfter: flushAfter, MaxBlocks: maxBlocks,
		},
		Deliver: func(packets [][]byte, now time.Time) error {
			serviceMu.RLock()
			svc := clientService
			serviceMu.RUnlock()
			if svc == nil {
				return errors.New("p5 fec: client service not attached")
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
		TickInterval: 2 * time.Millisecond,
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
		ticker := time.NewTicker(2 * time.Millisecond)
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
	outerBeforeC2S, outerBeforeS2C := recorder.wireSnapshot()

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
	peer := <-accepted
	_ = listener.Close()
	if peer.err != nil {
		_ = appConn.Close()
		t.Fatal(peer.err)
	}
	flowStarted := time.Now()
	flowID, err := svc.AddTCP(peer.conn, certificate.targetAddr, flowStarted)
	if err != nil {
		_ = appConn.Close()
		_ = peer.conn.Close()
		t.Fatal(err)
	}

	counting := &p5CountingConn{Conn: appConn}
	tlsCfg := certificate.tlsConfig.Clone()
	tlsCfg.ClientSessionCache = nil
	tlsConn := tls.Client(counting, tlsCfg)
	hsStarted := time.Now()
	if err := tlsConn.Handshake(); err != nil {
		_ = tlsConn.Close()
		t.Fatal(err)
	}
	handshakeNS := time.Since(hsStarted).Nanoseconds()
	state := tlsConn.ConnectionState()
	if state.Version != tls.VersionTLS13 || state.DidResume {
		_ = tlsConn.Close()
		t.Fatalf("inner TLS version=0x%x resumed=%v", state.Version, state.DidResume)
	}
	certificate.verifyState(t, state)

	requestStarted := time.Now()
	reqURL := &url.URL{Scheme: "https", Host: certificate.targetAddr.String(), Path: "/fec/20x10"}
	req := &http.Request{Method: http.MethodGet, URL: reqURL, Host: "target.test", Header: make(http.Header)}
	req.Header.Set("Connection", "close")
	if err := req.Write(tlsConn); err != nil {
		_ = tlsConn.Close()
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		_ = tlsConn.Close()
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	requestComplete := time.Now()
	_ = tlsConn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || len(body) != p5FECResponseSize || !bytes.Equal(body, responsePayload) {
		t.Fatalf("FEC HTTPS response status=%d bytes=%d", resp.StatusCode, len(body))
	}
	innerRead, innerWritten := counting.snapshot()
	if innerRead == 0 || innerWritten == 0 {
		t.Fatalf("inner TLS wire bytes c2s=%d s2c=%d", innerWritten, innerRead)
	}

	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 0, 3*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, 0, 3*time.Second)
	clientLane := waitP5FECLaneSettled(t, "client", client.Owner(), initialRef, 3*time.Second)
	serverLane := waitP5FECLaneSettled(t, "server", serverTunnel.owner, serverTunnel.ref, 3*time.Second)
	assertNoP5ClientRuntimeError(t, client, "after fixed-FEC HTTPS close")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("fixed-FEC flow replaced outer lane: got=%+v want=%+v", got, initialRef)
	}

	clientTx := clientLane.TxPath.Encoder
	serverTx := serverLane.TxPath.Encoder
	if !clientLane.TxPath.FECEnabled || !serverLane.TxPath.FECEnabled ||
		clientLane.TxPath.ParityShards != p5FECParity || serverLane.TxPath.ParityShards != p5FECParity {
		t.Fatalf("FEC profile mismatch client=%+v server=%+v", clientLane.TxPath, serverLane.TxPath)
	}
	if clientTx.SourceShards == 0 || clientTx.ParityShards == 0 {
		t.Fatalf("client FEC inventory source=%d parity=%d", clientTx.SourceShards, clientTx.ParityShards)
	}
	if serverTx.SourceShards == 0 || serverTx.ParityShards == 0 || serverTx.FullBlocks == 0 {
		t.Fatalf("server FEC inventory source=%d parity=%d full_blocks=%d", serverTx.SourceShards, serverTx.ParityShards, serverTx.FullBlocks)
	}
	if clientTx.PendingSources != 0 || serverTx.PendingSources != 0 {
		t.Fatalf("FEC pending sources client=%d server=%d", clientTx.PendingSources, serverTx.PendingSources)
	}

	clientRecovered := clientLane.RxPath.Decoder.RecoveredSources
	serverRecovered := serverLane.RxPath.Decoder.RecoveredSources
	clientReconstruct := clientLane.RxPath.Decoder.ReconstructionEvents
	serverReconstruct := serverLane.RxPath.Decoder.ReconstructionEvents
	if clientRecovered != 0 || serverRecovered != 0 || clientReconstruct != 0 || serverReconstruct != 0 {
		t.Fatalf("lossless FEC unexpectedly reconstructed client=%d/%d server=%d/%d",
			clientReconstruct, clientRecovered, serverReconstruct, serverRecovered)
	}

	clientTransport, ok := client.rt.TransportStats(initialRef)
	if !ok {
		t.Fatal("client transport stats unavailable")
	}
	serverTransport, ok := serverTunnel.rt.TransportStats(serverTunnel.ref)
	if !ok {
		t.Fatal("server transport stats unavailable")
	}
	repairRetransmits := clientTransport.Retransmitted + serverTransport.Retransmitted
	if repairRetransmits != 0 {
		t.Fatalf("lossless fixed-FEC scenario used repair retransmits=%d", repairRetransmits)
	}
	clientPadding := client.Owner().Stats().Padding.PaddingBytes
	serverPadding := serverTunnel.owner.Stats().Padding.PaddingBytes
	paddingBytes := clientPadding + serverPadding
	if paddingBytes != 0 {
		t.Fatalf("padding must remain off, bytes=%d", paddingBytes)
	}

	outerAfterC2S, outerAfterS2C := recorder.wireSnapshot()
	outerWireC2S := outerAfterC2S - outerBeforeC2S
	outerWireS2C := outerAfterS2C - outerBeforeS2C
	outerWireTotal := outerWireC2S + outerWireS2C
	innerWireTotal := innerWritten + innerRead
	if outerWireTotal == 0 || innerWireTotal == 0 {
		t.Fatalf("wire inventory outer=%d inner=%d", outerWireTotal, innerWireTotal)
	}
	wireAmplification := float64(outerWireTotal) / float64(innerWireTotal)

	outerPackets, recordEvents, clientSYNs := recorder.counters()
	if clientSYNs != 1 {
		t.Fatalf("outer connection reuse violated: initial SYN count=%d want=1", clientSYNs)
	}
	if recordEvents == 0 {
		t.Fatal("no steady TLS-like record events captured")
	}
	didResume := state.DidResume
	if err := recorder.recordFECEvent("https_fec_flow", p5FECMeasurementEvent{
		BusinessFlowID: uint64(flowID), OuterConnection: 1,
		RequestStartNS: requestStarted.Sub(recorder.start).Nanoseconds(),
		RequestCompleteNS: requestComplete.Sub(recorder.start).Nanoseconds(),
		BusinessLatencyNS: requestComplete.Sub(requestStarted).Nanoseconds(),
		HandshakeNS: handshakeNS, HTTPStatus: resp.StatusCode, ResponseBytes: len(body),
		InnerTLSWireBytesC2S: innerWritten, InnerTLSWireBytesS2C: innerRead,
		OuterWireBytesC2S: outerWireC2S, OuterWireBytesS2C: outerWireS2C,
		TLSVersion: state.Version, TLSCipherSuite: state.CipherSuite,
		HandshakeMode: "full", TLSResumed: &didResume,
		CertificateScenario: certificate.name, CertificateChainID: certificate.chainID,
		CertificateVerified: true, VerifiedChainLength: 3,
		VerifiedRootSHA256: certificate.rootSHA256,
		VerifiedIntermediateSHA256: certificate.intermediateSHA256,
		VerifiedLeafSHA256: certificate.leafSHA256,
		FECParityShards: p5FECParity, FECFlushAfterNS: flushAfter.Nanoseconds(), FECMaxBlocks: maxBlocks,
		RepairRetransmits: repairRetransmits, PaddingBytes: paddingBytes,
		WireAmplificationNumeratorBytes: outerWireTotal,
		WireAmplificationDenominatorBytes: innerWireTotal,
		WireAmplification: wireAmplification,
	}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.recordFECEvent("fec_inventory", p5FECMeasurementEvent{
		FECParityShards: p5FECParity, FECFlushAfterNS: flushAfter.Nanoseconds(), FECMaxBlocks: maxBlocks,
		ClientSourceShards: clientTx.SourceShards, ClientParityShards: clientTx.ParityShards,
		ClientFullBlocks: clientTx.FullBlocks, ClientPartialBlocks: clientTx.PartialBlocks,
		ClientPendingSources: clientTx.PendingSources,
		ClientReconstructionEvents: clientReconstruct, ClientRecoveredSources: clientRecovered,
		ServerSourceShards: serverTx.SourceShards, ServerParityShards: serverTx.ParityShards,
		ServerFullBlocks: serverTx.FullBlocks, ServerPartialBlocks: serverTx.PartialBlocks,
		ServerPendingSources: serverTx.PendingSources,
		ServerReconstructionEvents: serverReconstruct, ServerRecoveredSources: serverRecovered,
		RepairRetransmits: repairRetransmits, PaddingBytes: paddingBytes,
		WireAmplificationNumeratorBytes: outerWireTotal,
		WireAmplificationDenominatorBytes: innerWireTotal,
		WireAmplification: wireAmplification,
	}); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]any{
		"schema": p5MeasurementSchema,
		"source_sha": sourceSHA,
		"harness_sha": sourceSHA,
		"branch": "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name": "controlled-real-https-fixed-fec-20x10-lossless",
			"seed": 20260921,
			"cryptographic_rng": "system",
			"https_connections": 1,
			"https_requests": 1,
			"business_flow_count": 1,
			"outer_connection_count": 1,
			"connection_mtu": 1500,
			"desired_lanes": 1,
			"fec_profile": "20:10",
			"fec_data_shards": 20,
			"fec_parity_shards": p5FECParity,
			"fec_flush_after_ns": flushAfter.Nanoseconds(),
			"fec_max_blocks": maxBlocks,
			"fec_profile_role": "explicit-controlled-capability-cost-scenario-not-production-default",
			"production_default_fec": "off",
			"padding": "off",
			"network_injection": "none",
			"tls_version": "TLS1.3",
			"handshake_mode": "full",
			"certificate_chain_id": certificate.chainID,
			"response_body_bytes": p5FECResponseSize,
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
			"requests": 1,
			"responses": 1,
			"loss": 0,
			"duplicates": 0,
			"response_bytes": len(body),
			"handshake_ns": handshakeNS,
			"business_latency_ns": requestComplete.Sub(requestStarted).Nanoseconds(),
			"inner_tls_wire_bytes_c2s": innerWritten,
			"inner_tls_wire_bytes_s2c": innerRead,
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
			"steady_outer_wire_bytes_c2s": outerWireC2S,
			"steady_outer_wire_bytes_s2c": outerWireS2C,
		},
		"fec": map[string]any{
			"profile": "20:10",
			"client_source_shards": clientTx.SourceShards,
			"client_parity_shards": clientTx.ParityShards,
			"client_full_blocks": clientTx.FullBlocks,
			"client_partial_blocks": clientTx.PartialBlocks,
			"client_pending_sources": clientTx.PendingSources,
			"client_reconstruction_events": clientReconstruct,
			"client_recovered_sources": clientRecovered,
			"server_source_shards": serverTx.SourceShards,
			"server_parity_shards": serverTx.ParityShards,
			"server_full_blocks": serverTx.FullBlocks,
			"server_partial_blocks": serverTx.PartialBlocks,
			"server_pending_sources": serverTx.PendingSources,
			"server_reconstruction_events": serverReconstruct,
			"server_recovered_sources": serverRecovered,
			"recovery_sources_total": clientRecovered + serverRecovered,
		},
		"transport": map[string]any{
			"client": clientTransport,
			"server": serverTransport,
			"client_owner": client.Owner().Stats(),
			"server_owner": serverTunnel.owner.Stats(),
			"repair_retransmits": repairRetransmits,
			"padding_bytes": paddingBytes,
		},
		"cost": map[string]any{
			"wire_amplification_scope": "steady-outer-ipv4-tcp-bytes-over-inner-tls-ciphertext-bytes",
			"wire_amplification_numerator_bytes": outerWireTotal,
			"wire_amplification_denominator_bytes": innerWireTotal,
			"wire_amplification": wireAmplification,
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

	fmt.Printf("WBD_P5_FIXED_FEC_CAPTURED source_sha=%s profile=20:10 flows=1 outer_connections=1 client_source=%d client_parity=%d server_source=%d server_parity=%d fec_recovery=0 repair=0 padding=off wire_amp=%.6f records=%d\n",
		sourceSHA, clientTx.SourceShards, clientTx.ParityShards, serverTx.SourceShards, serverTx.ParityShards, wireAmplification, recordEvents)
}

func waitP5FECLaneSettled(t *testing.T, side string, owner *datapath.TunnelOwner, ref logicaltunnel.LaneRef, timeout time.Duration) datapath.LaneStats {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		stats, ok := owner.LaneStats(ref)
		if ok && stats.TxPath.FECEnabled && stats.TxPath.Encoder.PendingSources == 0 {
			return stats
		}
		if !time.Now().Before(deadline) {
			stats, ok := owner.LaneStats(ref)
			t.Fatalf("%s FEC lane did not settle: ok=%v stats=%+v", side, ok, stats)
		}
		<-ticker.C
	}
}
