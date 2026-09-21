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
	"strconv"
	"strings"
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
	p5LoadDuration = 15 * time.Second
	p5LoadBaseSeed = uint64(20260925)
)

var p5LoadRatesMbps = []int{5, 10, 15, 20}
var p5LoadSizes = []int{1024, 8 * 1024, 32 * 1024}

type p5LoadSlot struct {
	Ordinal   int
	Size      int
	Scheduled time.Duration
}

type p5LoadSample struct {
	Schema string `json:"schema"`
	Event  string `json:"event"`

	Ordinal          int   `json:"ordinal"`
	SizeBytes        int   `json:"size_bytes"`
	ScheduledStartNS int64 `json:"scheduled_start_ns"`
	ActualStartNS    int64 `json:"actual_start_ns,omitempty"`
	SendLagNS        int64 `json:"send_lag_ns,omitempty"`
	RequestCompleteNS int64 `json:"request_complete_ns,omitempty"`
	RTTNS            int64 `json:"rtt_ns,omitempty"`

	Accepted       bool   `json:"accepted"`
	Success        bool   `json:"success"`
	SendFailure    bool   `json:"send_failure,omitempty"`
	ErrorStage     string `json:"error_stage,omitempty"`
	Error          string `json:"error,omitempty"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	RequestBytes   int    `json:"request_bytes,omitempty"`
	ResponseBytes  int    `json:"response_bytes,omitempty"`
	InnerTLSWireC2S uint64 `json:"inner_tls_wire_bytes_c2s,omitempty"`
	InnerTLSWireS2C uint64 `json:"inner_tls_wire_bytes_s2c,omitempty"`
}

type p5LoadTransportResource struct {
	Outstanding     int    `json:"outstanding"`
	OutOfOrder      int    `json:"out_of_order"`
	PeakOutstanding int    `json:"peak_outstanding"`
	Retransmitted   uint64 `json:"retransmitted"`
	Abandoned       uint64 `json:"abandoned"`
	ForgivenGaps    uint64 `json:"forgiven_gaps"`
}

type p5LoadResourceSample struct {
	Schema string `json:"schema"`
	Event  string `json:"event"`
	TNS    int64  `json:"t_ns"`

	OuterPackets        uint64                  `json:"outer_packets"`
	OuterWireBytesC2S   uint64                  `json:"outer_wire_bytes_c2s"`
	OuterWireBytesS2C   uint64                  `json:"outer_wire_bytes_s2c"`
	ClientTransport     p5LoadTransportResource `json:"client_transport"`
	ServerTransport     p5LoadTransportResource `json:"server_transport"`
	ClientBusinessFlows int                     `json:"client_business_flows"`
	ServerBusinessFlows int                     `json:"server_business_flows"`

	MemAllocBytes uint64              `json:"mem_alloc_bytes"`
	HeapAllocBytes uint64             `json:"heap_alloc_bytes"`
	HeapInuseBytes uint64             `json:"heap_inuse_bytes"`
	SysBytes       uint64             `json:"sys_bytes"`
	NumGC          uint32             `json:"num_gc"`
	PauseTotalNS   uint64             `json:"pause_total_ns"`
	HostCPU        map[string][]uint64 `json:"host_cpu"`
}

type p5LoadArtifacts struct {
	mu        sync.Mutex
	load      *os.File
	resources *os.File
}

func newP5LoadArtifacts(dir string) (*p5LoadArtifacts, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	load, err := os.Create(filepath.Join(dir, "load.jsonl"))
	if err != nil {
		return nil, err
	}
	resources, err := os.Create(filepath.Join(dir, "resources.jsonl"))
	if err != nil {
		_ = load.Close()
		return nil, err
	}
	return &p5LoadArtifacts{load: load, resources: resources}, nil
}

func (a *p5LoadArtifacts) close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return errors.Join(a.load.Close(), a.resources.Close())
}

func (a *p5LoadArtifacts) writeLoad(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	enc := json.NewEncoder(a.load)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func (a *p5LoadArtifacts) writeResource(v any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	enc := json.NewEncoder(a.resources)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func p5LoadConfig(t *testing.T) (rateMbps, run int, seed uint64) {
	t.Helper()
	rateMbps, err := strconv.Atoi(os.Getenv("WBD_P5_LOAD_MBPS"))
	if err != nil {
		t.Fatalf("WBD_P5_LOAD_MBPS: %v", err)
	}
	rateIndex := -1
	for i, value := range p5LoadRatesMbps {
		if rateMbps == value {
			rateIndex = i
			break
		}
	}
	if rateIndex < 0 {
		t.Fatalf("WBD_P5_LOAD_MBPS must be one of %v, got %d", p5LoadRatesMbps, rateMbps)
	}
	run, err = strconv.Atoi(os.Getenv("WBD_P5_LOAD_RUN"))
	if err != nil || (run != 1 && run != 2) {
		t.Fatalf("WBD_P5_LOAD_RUN must be 1 or 2, got %q", os.Getenv("WBD_P5_LOAD_RUN"))
	}
	return rateMbps, run, p5LoadBaseSeed + uint64(rateIndex*2+run-1)
}

func buildP5LoadPlan(rateMbps int) []p5LoadSlot {
	targetBPS := int64(rateMbps) * 1_000_000
	var cumulativeBits int64
	slots := make([]p5LoadSlot, 0, rateMbps*80)
	for ordinal := 1; ; ordinal++ {
		scheduledNS := cumulativeBits * int64(time.Second) / targetBPS
		if scheduledNS >= p5LoadDuration.Nanoseconds() {
			break
		}
		size := p5LoadSizes[(ordinal-1)%len(p5LoadSizes)]
		slots = append(slots, p5LoadSlot{Ordinal: ordinal, Size: size, Scheduled: time.Duration(scheduledNS)})
		cumulativeBits += int64(size * 2 * 8)
	}
	return slots
}

func p5LoadBody(seed uint64, ordinal, size int) []byte {
	body := make([]byte, size)
	prefix := []byte(fmt.Sprintf("wbd-p5-load seed=%d ordinal=%06d:", seed, ordinal))
	copy(body, prefix)
	for i := len(prefix); i < len(body); i++ {
		body[i] = byte('a' + (int(seed)+ordinal+i)%26)
	}
	return body
}

func readP5LoadHostCPU() map[string][]uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	out := make(map[string][]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		values := make([]uint64, 0, len(fields)-1)
		ok := true
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				ok = false
				break
			}
			values = append(values, value)
		}
		if ok {
			out[fields[0]] = values
		}
	}
	return out
}


func runP5LoadResourceSampler(ctx context.Context, artifacts *p5LoadArtifacts, recorder *p5MeasurementRecorder, start time.Time, client *Client, serverTunnel *serverTunnel, clientRef logicaltunnel.LaneRef) {
	sample := func(now time.Time) {
		outer, _, _ := recorder.counters()
		wireC2S, wireS2C := recorder.wireSnapshot()
		clientStats, _ := client.rt.TransportStats(clientRef)
		serverStats, _ := serverTunnel.rt.TransportStats(serverTunnel.ref)
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		_ = artifacts.writeResource(p5LoadResourceSample{
			Schema: p5MeasurementSchema,
			Event:  "resource_sample",
			TNS:    now.Sub(start).Nanoseconds(),
			OuterPackets:      outer,
			OuterWireBytesC2S: wireC2S,
			OuterWireBytesS2C: wireS2C,
			ClientTransport: p5LoadTransportResource{
				Outstanding: clientStats.Outstanding, OutOfOrder: clientStats.OutOfOrder,
				PeakOutstanding: clientStats.PeakOutstanding, Retransmitted: clientStats.Retransmitted,
				Abandoned: clientStats.Abandoned, ForgivenGaps: clientStats.ForgivenGaps,
			},
			ServerTransport: p5LoadTransportResource{
				Outstanding: serverStats.Outstanding, OutOfOrder: serverStats.OutOfOrder,
				PeakOutstanding: serverStats.PeakOutstanding, Retransmitted: serverStats.Retransmitted,
				Abandoned: serverStats.Abandoned, ForgivenGaps: serverStats.ForgivenGaps,
			},
			ClientBusinessFlows: client.Owner().Stats().BusinessFlows,
			ServerBusinessFlows: serverTunnel.owner.Stats().BusinessFlows,
			MemAllocBytes: mem.Alloc, HeapAllocBytes: mem.HeapAlloc, HeapInuseBytes: mem.HeapInuse,
			SysBytes: mem.Sys, NumGC: mem.NumGC, PauseTotalNS: mem.PauseTotalNs,
			HostCPU: readP5LoadHostCPU(),
		})
	}

	sample(time.Now())
	ticker := time.NewTicker(250 * time.Millisecond)
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

func TestP5LoadMeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_LOAD_MEASURE") != "1" {
		t.Skip("P5 load harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_LOAD_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_LOAD_ARTIFACT_DIR is required")
	}
	sourceSHA := os.Getenv("GITHUB_SHA")
	if len(sourceSHA) != 40 {
		t.Fatalf("GITHUB_SHA must be an exact 40-hex source SHA, got %q", sourceSHA)
	}
	rateMbps, runOrdinal, seed := p5LoadConfig(t)

	recorder, err := newP5MeasurementRecorder(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.close()
	artifacts, err := newP5LoadArtifacts(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.close()

	var serverCountsMu sync.Mutex
	serverCounts := make(map[int]int)
	certificate := newP5CertificateScenarioWithHandler(t, "controlled-load-chain", "load-chain", 7000, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ordinal, err1 := strconv.Atoi(req.URL.Query().Get("ordinal"))
		size, err2 := strconv.Atoi(req.URL.Query().Get("size"))
		body, readErr := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err1 != nil || err2 != nil || readErr != nil || ordinal <= 0 || size <= 0 {
			http.Error(w, "invalid load request", http.StatusBadRequest)
			return
		}
		serverCountsMu.Lock()
		serverCounts[ordinal]++
		serverCountsMu.Unlock()
		if len(body) != size || !bytes.Equal(body, p5LoadBody(seed, ordinal, size)) {
			http.Error(w, "load body integrity", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("X-WBD-Ordinal", strconv.Itoa(ordinal))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer certificate.server.Close()

	outerCert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("p5-load"))
	var installation logicaltunnel.InstallationID
	installation[0] = 10
	lease := logicaltunnel.Lease{
		Account: "p5-load", InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{TunnelID: tunnelID, Address4: "10.66.0.10/32", Routes4: []string{"0.0.0.0/0"}},
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
		IO: recorder.wrapIO("s2c", serverEP.io()), ListenPort: 443, MaxAssociations: 16,
		InitialRTO: time.Second, TickInterval: 10 * time.Millisecond, MaxFlows: 64,
		Admission: realityfront.ServerAdmissionConfig{
			TLS: realityfront.ServerConfig{ServerName: "target.test", RouteKey: routeKey, TLSConfig: &tls.Config{Certificates: []tls.Certificate{outerCert}}, Timeout: 3 * time.Second},
			ExpectedUsername: "p5-load", ExpectedPassword: "load-password", ServerLimit: 1250,
		},
		LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
			if id != tunnelID {
				return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
			}
			return lease.Clone(), nil
		},
		Lane: datapath.ServerLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
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
		IO: recorder.wrapIO("c2s", clientEP.io()),
		Flow: faketcp.ClientFlow{LocalIP: [4]byte{192, 0, 2, 55}, PeerIP: [4]byte{192, 0, 2, 60}, LocalPort: 42500, PeerPort: 443},
		ClientISN: 10000, InitialRTO: time.Second, Lease: lease, MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second},
			Username: "p5-load", Password: "load-password", TunnelID: tunnelID.Bytes(), ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
		Deliver: func(packets [][]byte, now time.Time) error {
			serviceMu.RLock()
			svc := clientService
			serviceMu.RUnlock()
			if svc == nil {
				return errors.New("p5 load: client service not attached")
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
		t.Fatalf("load atom must keep client FEC off: ok=%v stats=%+v", ok, laneStats)
	}
	if laneStats, ok := serverTunnel.owner.LaneStats(serverTunnel.ref); !ok || laneStats.TxPath.FECEnabled {
		t.Fatalf("load atom must keep server FEC off: ok=%v stats=%+v", ok, laneStats)
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	type acceptResult struct {
		conn net.Conn
		err  error
	}
	acceptedLocal := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		acceptedLocal <- acceptResult{conn: conn, err: err}
	}()
	appConn, err := net.DialTimeout("tcp4", listener.Addr().String(), 2*time.Second)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	peer := <-acceptedLocal
	_ = listener.Close()
	if peer.err != nil {
		_ = appConn.Close()
		t.Fatal(peer.err)
	}
	flowID, err := svc.AddTCP(peer.conn, certificate.targetAddr, time.Now())
	if err != nil {
		_ = appConn.Close()
		_ = peer.conn.Close()
		t.Fatal(err)
	}
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 1, 2*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, 1, 2*time.Second)

	recorder.markSteady()
	counting := &p5CountingConn{Conn: appConn}
	tlsCfg := certificate.tlsConfig.Clone()
	tlsCfg.ClientSessionCache = nil
	tlsConn := tls.Client(counting, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	tlsState := tlsConn.ConnectionState()
	if tlsState.Version != tls.VersionTLS13 || tlsState.DidResume {
		t.Fatalf("load inner TLS version=0x%x resumed=%v", tlsState.Version, tlsState.DidResume)
	}
	certificate.verifyState(t, tlsState)
	reader := bufio.NewReader(tlsConn)

	plan := buildP5LoadPlan(rateMbps)
	if len(plan) == 0 {
		t.Fatal("load plan is empty")
	}
	var offeredAggregateBytes uint64
	for _, slot := range plan {
		offeredAggregateBytes += uint64(slot.Size * 2)
	}

	beforeInnerRead, beforeInnerWritten := counting.snapshot()
	loadStart := time.Now()
	loadStartNS := loadStart.Sub(recorder.start).Nanoseconds()
	loadWindowEnd := loadStart.Add(p5LoadDuration)
	_ = tlsConn.SetDeadline(loadWindowEnd.Add(10 * time.Second))

	resourceCtx, resourceCancel := context.WithCancel(context.Background())
	resourceDone := make(chan struct{})
	go func() {
		defer close(resourceDone)
		runP5LoadResourceSampler(resourceCtx, artifacts, recorder, loadStart, client, serverTunnel, initialRef)
	}()

	var acceptedTransactions, successfulTransactions, sendFailures, skippedSlots int
	var acceptedAggregateBytes, actualC2SPayloadBytes, actualS2CPayloadBytes uint64
	var maxSendLag time.Duration
	broke := false
	for i, slot := range plan {
		scheduledAt := loadStart.Add(slot.Scheduled)
		if wait := time.Until(scheduledAt); wait > 0 {
			timer := time.NewTimer(wait)
			<-timer.C
		}
		actualStart := time.Now()
		if !actualStart.Before(loadWindowEnd) {
			skippedSlots += len(plan) - i
			break
		}
		lag := actualStart.Sub(scheduledAt)
		if lag < 0 {
			lag = 0
		}
		if lag > maxSendLag {
			maxSendLag = lag
		}
		sample := p5LoadSample{
			Schema: p5MeasurementSchema, Event: "load_transaction",
			Ordinal: slot.Ordinal, SizeBytes: slot.Size, ScheduledStartNS: slot.Scheduled.Nanoseconds(),
			ActualStartNS: actualStart.Sub(loadStart).Nanoseconds(), SendLagNS: lag.Nanoseconds(),
			Accepted: true,
		}
		acceptedTransactions++
		acceptedAggregateBytes += uint64(slot.Size * 2)

		body := p5LoadBody(seed, slot.Ordinal, slot.Size)
		beforeRead, beforeWritten := counting.snapshot()
		reqURL := &url.URL{
			Scheme: "https", Host: certificate.targetAddr.String(), Path: "/load",
			RawQuery: fmt.Sprintf("ordinal=%d&size=%d", slot.Ordinal, slot.Size),
		}
		req := &http.Request{
			Method: http.MethodPost, URL: reqURL, Host: "target.test", Header: make(http.Header),
			Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)),
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		requestStart := time.Now()
		if err := req.Write(tlsConn); err != nil {
			sendFailures++
			sample.SendFailure = true
			sample.ErrorStage, sample.Error = "request_write", err.Error()
			afterRead, afterWritten := counting.snapshot()
			sample.InnerTLSWireC2S = afterWritten - beforeWritten
			sample.InnerTLSWireS2C = afterRead - beforeRead
			if err := artifacts.writeLoad(sample); err != nil {
				t.Fatal(err)
			}
			skippedSlots += len(plan) - i - 1
			broke = true
			break
		}
		sample.RequestBytes = len(body)
		actualC2SPayloadBytes += uint64(len(body))

		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			sample.ErrorStage, sample.Error = "response_headers", err.Error()
			afterRead, afterWritten := counting.snapshot()
			sample.InnerTLSWireC2S = afterWritten - beforeWritten
			sample.InnerTLSWireS2C = afterRead - beforeRead
			if err := artifacts.writeLoad(sample); err != nil {
				t.Fatal(err)
			}
			skippedSlots += len(plan) - i - 1
			broke = true
			break
		}
		responseBody, bodyErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		complete := time.Now()
		sample.RequestCompleteNS = complete.Sub(loadStart).Nanoseconds()
		sample.RTTNS = complete.Sub(requestStart).Nanoseconds()
		sample.HTTPStatus = resp.StatusCode
		sample.ResponseBytes = len(responseBody)
		afterRead, afterWritten := counting.snapshot()
		sample.InnerTLSWireC2S = afterWritten - beforeWritten
		sample.InnerTLSWireS2C = afterRead - beforeRead
		if bodyErr != nil {
			sample.ErrorStage, sample.Error = "response_body", bodyErr.Error()
		} else if resp.StatusCode != http.StatusOK {
			sample.ErrorStage, sample.Error = "http_status", resp.Status
		} else if !bytes.Equal(responseBody, body) {
			sample.ErrorStage, sample.Error = "integrity", "echo body mismatch"
		} else {
			sample.Success = true
			successfulTransactions++
			actualS2CPayloadBytes += uint64(len(responseBody))
		}
		if err := artifacts.writeLoad(sample); err != nil {
			t.Fatal(err)
		}
		if !sample.Success {
			skippedSlots += len(plan) - i - 1
			broke = true
			break
		}
	}
	_ = broke

	loadComplete := time.Now()
	loadCompleteNS := loadComplete.Sub(recorder.start).Nanoseconds()
	resourceCancel()
	<-resourceDone

	afterInnerRead, afterInnerWritten := counting.snapshot()
	innerTLSC2S := afterInnerWritten - beforeInnerWritten
	innerTLSS2C := afterInnerRead - beforeInnerRead
	actualAggregatePayloadBytes := actualC2SPayloadBytes + actualS2CPayloadBytes
	appLoss := acceptedTransactions - successfulTransactions

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
	duplicates := 0
	for _, count := range serverCountsCopy {
		if count > 1 {
			duplicates += count - 1
		}
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
	paddingBytes := client.Owner().Stats().Padding.PaddingBytes + serverTunnel.owner.Stats().Padding.PaddingBytes
	if paddingBytes != 0 {
		t.Fatalf("load atom must keep padding off, bytes=%d", paddingBytes)
	}
	if clientLane.TxPath.FECEnabled || serverLane.TxPath.FECEnabled {
		t.Fatal("load atom unexpectedly enabled FEC")
	}
	repairRetransmits := clientTransport.Retransmitted + serverTransport.Retransmitted

	if err := artifacts.writeLoad(map[string]any{
		"schema": p5MeasurementSchema, "event": "application_inventory",
		"offered_transactions": len(plan), "accepted_transactions": acceptedTransactions,
		"successful_transactions": successfulTransactions, "application_loss": appLoss,
		"application_duplicates": duplicates, "send_failures": sendFailures, "skipped_slots": skippedSlots,
		"offered_aggregate_payload_bytes": offeredAggregateBytes, "accepted_aggregate_payload_bytes": acceptedAggregateBytes,
		"actual_c2s_payload_bytes": actualC2SPayloadBytes, "actual_s2c_payload_bytes": actualS2CPayloadBytes,
		"actual_aggregate_payload_bytes": actualAggregatePayloadBytes, "server_request_counts": serverCountsCopy,
		"max_send_lag_ns": maxSendLag.Nanoseconds(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.writeResource(map[string]any{
		"schema": p5MeasurementSchema, "event": "final_inventory", "t_ns": loadComplete.Sub(loadStart).Nanoseconds(),
		"fec_profile": "off", "client_fec_enabled": clientLane.TxPath.FECEnabled, "server_fec_enabled": serverLane.TxPath.FECEnabled,
		"client_source_shards": clientLane.TxPath.Encoder.SourceShards, "client_parity_shards": clientLane.TxPath.Encoder.ParityShards,
		"client_reconstruction_events": clientLane.RxPath.Decoder.ReconstructionEvents, "client_recovered_sources": clientLane.RxPath.Decoder.RecoveredSources,
		"server_source_shards": serverLane.TxPath.Encoder.SourceShards, "server_parity_shards": serverLane.TxPath.Encoder.ParityShards,
		"server_reconstruction_events": serverLane.RxPath.Decoder.ReconstructionEvents, "server_recovered_sources": serverLane.RxPath.Decoder.RecoveredSources,
		"repair_retransmits": repairRetransmits, "padding_bytes": paddingBytes, "capture_loss_packets": 0,
		"inner_tls_wire_bytes_c2s": innerTLSC2S, "inner_tls_wire_bytes_s2c": innerTLSS2C,
	}); err != nil {
		t.Fatal(err)
	}

	outerPackets, recordEvents, clientSYNs := recorder.counters()
	if clientSYNs != 1 {
		t.Fatalf("load outer connection reuse violated: client initial SYN=%d", clientSYNs)
	}
	if recordEvents == 0 {
		t.Fatal("load captured no steady TLS-like record events")
	}

	manifest := map[string]any{
		"schema": p5MeasurementSchema, "source_sha": sourceSHA, "harness_sha": sourceSHA, "branch": "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name": "controlled-real-https-load", "run_ordinal": runOrdinal, "seed": seed,
			"duration_seconds": 15, "target_mbps": rateMbps, "target_scope": "aggregate-inner-application-payload-c2s-plus-s2c",
			"payload_size_cycle_bytes": p5LoadSizes, "application_model": "single persistent TLS1.3 HTTPS BusinessFlow; paced POST echo",
			"pacing": "absolute-clock cumulative offered bits; no batching wait", "desired_lanes": 1, "connection_mtu": 1500,
			"fec_profile": "off", "production_default_fec": "off", "padding": "off", "network_injection": "none",
			"load_start_event_ns": loadStartNS, "load_window_end_event_ns": loadStartNS + p5LoadDuration.Nanoseconds(),
			"load_complete_event_ns": loadCompleteNS,
		},
		"certificate_scenario": map[string]any{
			"name": certificate.name, "chain_id": certificate.chainID, "server_name": "target.test", "verified_chain_length": 3,
			"root_sha256": certificate.rootSHA256, "intermediate_sha256": certificate.intermediateSHA256, "leaf_sha256": certificate.leafSHA256,
		},
		"application": map[string]any{
			"business_flow_id": uint64(flowID), "business_flows": 1, "offered_transactions": len(plan),
			"accepted_transactions": acceptedTransactions, "successful_transactions": successfulTransactions,
			"loss": appLoss, "duplicates": duplicates, "send_failures": sendFailures, "skipped_slots": skippedSlots,
			"offered_aggregate_payload_bytes": offeredAggregateBytes, "accepted_aggregate_payload_bytes": acceptedAggregateBytes,
			"actual_c2s_payload_bytes": actualC2SPayloadBytes, "actual_s2c_payload_bytes": actualS2CPayloadBytes,
			"actual_aggregate_payload_bytes": actualAggregatePayloadBytes,
			"inner_tls_wire_bytes_c2s": innerTLSC2S, "inner_tls_wire_bytes_s2c": innerTLSS2C,
		},
		"capture": map[string]any{
			"type": "hosted-segmentio-serialized-pcap", "pcap": "outer.pcap", "events": "events.jsonl",
			"load_samples": "load.jsonl", "resource_samples": "resources.jsonl",
			"capture_loss_packets": 0, "capture_loss_method": "in-process SegmentIO emit interception; no kernel capture queue",
			"outer_packets": outerPackets, "tlslike_record_events": recordEvents, "client_initial_syns": clientSYNs,
		},
		"transport": map[string]any{
			"client": clientTransport, "server": serverTransport, "repair_retransmits": repairRetransmits,
			"fec_recovery": 0, "padding_bytes": paddingBytes,
		},
		"runner": map[string]any{
			"go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "num_cpu": runtime.NumCPU(),
			"github_actions": os.Getenv("GITHUB_ACTIONS"), "runner_os": os.Getenv("RUNNER_OS"), "runner_arch": os.Getenv("RUNNER_ARCH"), "image_os": os.Getenv("ImageOS"),
			"resource_metrics_policy": "observational-only-no-cross-vm-absolute-gate",
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

	_ = tlsConn.Close()
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 0, 4*time.Second)
	waitP5TCPFlowCount(t, "server", serverTunnel.service.TCPFlows, 0, 4*time.Second)
	assertNoP5ClientRuntimeError(t, client, "after P5 load workload")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("load workload replaced outer lane: got=%+v want=%+v", got, initialRef)
	}

	actualMbps := float64(actualAggregatePayloadBytes*8) / float64(p5LoadDuration/time.Second) / 1_000_000
	fmt.Printf("WBD_P5_LOAD_CAPTURED source_sha=%s target_mbps=%d scope=aggregate-inner run=%d seed=%d duration=15s offered_tx=%d accepted_tx=%d success=%d loss=%d duplicate=%d send_failures=%d skipped=%d actual_mbps=%.6f fec=off padding=off repair=%d\n",
		sourceSHA, rateMbps, runOrdinal, seed, len(plan), acceptedTransactions, successfulTransactions, appLoss, duplicates, sendFailures, skippedSlots, actualMbps, repairRetransmits)
}
