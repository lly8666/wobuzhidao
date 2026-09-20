package runtimeentry

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
	p5MeasurementSchema = "wbd-p5-https-measurement/v1"
	p5BurstGap          = 10 * time.Millisecond
)

type p5MeasurementEvent struct {
	Schema string `json:"schema"`
	Event  string `json:"event"`
	TNS    int64  `json:"t_ns"`

	Direction string `json:"direction,omitempty"`
	Phase     string `json:"phase,omitempty"`

	OuterPacketLen int    `json:"outer_packet_len,omitempty"`
	PayloadLen     int    `json:"payload_len,omitempty"`
	Seq            uint32 `json:"seq,omitempty"`
	Ack            uint32 `json:"ack,omitempty"`
	Flags          uint8  `json:"flags,omitempty"`
	InterArrivalNS int64  `json:"inter_arrival_ns,omitempty"`
	BurstID        uint64 `json:"burst_id,omitempty"`
	BurstWireBytes uint64 `json:"burst_wire_bytes,omitempty"`

	RecordLen     int `json:"record_len,omitempty"`
	RecordBodyLen int `json:"record_body_len,omitempty"`
	RecordOffset  int `json:"record_offset,omitempty"`

	FlowOrdinal       int    `json:"flow_ordinal,omitempty"`
	Scenario          string `json:"scenario,omitempty"`
	OuterConnection   int    `json:"outer_connection_id,omitempty"`
	BusinessFlowID    uint64 `json:"business_flow_id,omitempty"`
	FlowClosed        bool   `json:"flow_closed,omitempty"`
	HandshakeNS       int64  `json:"handshake_ns,omitempty"`
	BusinessLatencyNS int64  `json:"business_latency_ns,omitempty"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	ResponseBytes     int    `json:"response_bytes,omitempty"`
	WireBytesC2S      uint64 `json:"wire_bytes_c2s,omitempty"`
	WireBytesS2C      uint64 `json:"wire_bytes_s2c,omitempty"`
}

type p5BurstState struct {
	last  time.Time
	id    uint64
	bytes uint64
}

type p5MeasurementRecorder struct {
	mu sync.Mutex

	start time.Time
	dir   string

	events *os.File
	pcap   *os.File

	steady bool
	ipID   uint16

	burst      map[string]p5BurstState
	wire       map[string]uint64
	lastRecord map[string]time.Time

	outerPackets uint64
	recordEvents uint64
	clientSYNs   uint64
}

func newP5MeasurementRecorder(dir string) (*p5MeasurementRecorder, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	events, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return nil, err
	}
	pcap, err := os.Create(filepath.Join(dir, "outer.pcap"))
	if err != nil {
		_ = events.Close()
		return nil, err
	}
	r := &p5MeasurementRecorder{
		start:      time.Now(),
		dir:        dir,
		events:     events,
		pcap:       pcap,
		ipID:       1,
		burst:      map[string]p5BurstState{},
		wire:       map[string]uint64{},
		lastRecord: map[string]time.Time{},
	}
	if err := writeP5PCAPHeader(pcap); err != nil {
		_ = events.Close()
		_ = pcap.Close()
		return nil, err
	}
	return r, nil
}

func (r *p5MeasurementRecorder) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.events.Close(), r.pcap.Close())
}

func (r *p5MeasurementRecorder) markSteady() {
	r.mu.Lock()
	r.steady = true
	r.mu.Unlock()
}

func (r *p5MeasurementRecorder) wrapIO(direction string, base SegmentIO) SegmentIO {
	return SegmentIO{
		Read: base.Read,
		Emit: func(seg faketcp.Segment) error {
			if err := r.capture(direction, seg, time.Now()); err != nil {
				return err
			}
			return base.Emit(seg)
		},
		Close: base.Close,
	}
}

func (r *p5MeasurementRecorder) capture(direction string, seg faketcp.Segment, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pkt := faketcp.MarshalIPv4TCP(seg.SrcIP, seg.DstIP, seg.SrcPort, seg.DstPort, seg.Seq, seg.Ack, seg.Flags, seg.Window, seg.Payload, r.ipID)
	r.ipID++
	if err := writeP5PCAPPacket(r.pcap, now, pkt); err != nil {
		return err
	}

	phase := "bootstrap"
	if r.steady {
		phase = "steady"
	}
	state := r.burst[direction]
	gap := int64(0)
	if !state.last.IsZero() {
		gap = now.Sub(state.last).Nanoseconds()
	}
	if state.id == 0 || (!state.last.IsZero() && now.Sub(state.last) > p5BurstGap) {
		state.id++
		state.bytes = 0
	}
	state.last = now
	state.bytes += uint64(len(pkt))
	r.burst[direction] = state
	r.wire[direction] += uint64(len(pkt))
	r.outerPackets++
	if direction == "c2s" && seg.Flags&faketcp.FlagSYN != 0 && seg.Flags&faketcp.FlagACK == 0 {
		r.clientSYNs++
	}
	if err := r.writeEventLocked(p5MeasurementEvent{
		Schema: p5MeasurementSchema, Event: "outer_packet", TNS: now.Sub(r.start).Nanoseconds(),
		Direction: direction, Phase: phase,
		OuterPacketLen: len(pkt), PayloadLen: len(seg.Payload), Seq: seg.Seq, Ack: seg.Ack, Flags: seg.Flags,
		InterArrivalNS: gap, BurstID: state.id, BurstWireBytes: state.bytes,
	}); err != nil {
		return err
	}
	if r.steady && len(seg.Payload) != 0 {
		return r.captureRecordsLocked(direction, seg, now)
	}
	return nil
}

func (r *p5MeasurementRecorder) captureRecordsLocked(direction string, seg faketcp.Segment, now time.Time) error {
	for off := 0; off+5 <= len(seg.Payload); {
		if seg.Payload[off] != 0x17 || seg.Payload[off+1] != 0x03 || seg.Payload[off+2] != 0x03 {
			break
		}
		body := int(binary.BigEndian.Uint16(seg.Payload[off+3 : off+5]))
		total := 5 + body
		if body <= 0 || off+total > len(seg.Payload) {
			break
		}
		recordGap := int64(0)
		if last := r.lastRecord[direction]; !last.IsZero() {
			recordGap = now.Sub(last).Nanoseconds()
		}
		r.lastRecord[direction] = now
		if err := r.writeEventLocked(p5MeasurementEvent{
			Schema: p5MeasurementSchema, Event: "tlslike_record", TNS: now.Sub(r.start).Nanoseconds(),
			Direction: direction, Phase: "steady", RecordLen: total, RecordBodyLen: body, RecordOffset: off,
			Seq: seg.Seq + uint32(off), InterArrivalNS: recordGap,
		}); err != nil {
			return err
		}
		r.recordEvents++
		off += total
	}
	return nil
}

func (r *p5MeasurementRecorder) wireSnapshot() (uint64, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wire["c2s"], r.wire["s2c"]
}

func (r *p5MeasurementRecorder) recordHTTPSFlow(ev p5MeasurementEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev.Schema = p5MeasurementSchema
	ev.Event = "https_flow"
	ev.TNS = time.Now().Sub(r.start).Nanoseconds()
	return r.writeEventLocked(ev)
}

func (r *p5MeasurementRecorder) writeEventLocked(ev p5MeasurementEvent) error {
	enc := json.NewEncoder(r.events)
	enc.SetEscapeHTML(false)
	return enc.Encode(ev)
}

func (r *p5MeasurementRecorder) counters() (outer, records, clientSYNs uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.outerPackets, r.recordEvents, r.clientSYNs
}

func writeP5PCAPHeader(w io.Writer) error {
	fields := []any{uint32(0xa1b2c3d4), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(101)}
	for _, field := range fields {
		if err := binary.Write(w, binary.LittleEndian, field); err != nil {
			return err
		}
	}
	return nil
}

func writeP5PCAPPacket(w io.Writer, now time.Time, packet []byte) error {
	fields := []any{uint32(now.Unix()), uint32(now.Nanosecond() / 1000), uint32(len(packet)), uint32(len(packet))}
	for _, field := range fields {
		if err := binary.Write(w, binary.LittleEndian, field); err != nil {
			return err
		}
	}
	_, err := w.Write(packet)
	return err
}

func TestP5ControlledHTTPSMeasurementHarness(t *testing.T) {
	if os.Getenv("WBD_P5_HTTPS_MEASURE") != "1" {
		t.Skip("P5 measurement harness runs only in its dedicated Actions gate")
	}
	artifactDir := os.Getenv("WBD_P5_ARTIFACT_DIR")
	if artifactDir == "" {
		t.Fatal("WBD_P5_ARTIFACT_DIR is required")
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

	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintf(w, "wbd-p5 path=%s", req.URL.Path)
	}))
	defer target.Close()
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	targetAddr, err := netip.ParseAddrPort(targetURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	innerTLS := &tls.Config{RootCAs: roots, ServerName: targetURL.Hostname(), MinVersion: tls.VersionTLS12}

	outerCert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("p5-https-measure"))
	var installation logicaltunnel.InstallationID
	installation[0] = 5
	lease := logicaltunnel.Lease{
		Account:        "p5-measure",
		InstallationID: installation,
		Config:         logicaltunnel.TunnelConfig{TunnelID: tunnelID, Address4: "10.66.0.5/32", Routes4: []string{"0.0.0.0/0"}},
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
			TLS:              realityfront.ServerConfig{ServerName: "target.test", RouteKey: routeKey, TLSConfig: &tls.Config{Certificates: []tls.Certificate{outerCert}}, Timeout: 3 * time.Second},
			ExpectedUsername: "p5-measure", ExpectedPassword: "correct-password", ServerLimit: 1250,
		},
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
		IO:        recorder.wrapIO("c2s", clientEP.io()),
		Flow:      faketcp.ClientFlow{LocalIP: [4]byte{192, 0, 2, 50}, PeerIP: [4]byte{192, 0, 2, 60}, LocalPort: 42000, PeerPort: 443},
		ClientISN: 5000, InitialRTO: time.Second, Lease: lease, MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS:      realityfront.ClientConfig{ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second},
			Username: "p5-measure", Password: "correct-password", TunnelID: tunnelID.Bytes(), ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20, RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20},
		Deliver: func(packets [][]byte, now time.Time) error {
			serviceMu.RLock()
			svc := clientService
			serviceMu.RUnlock()
			if svc == nil {
				return errors.New("p5 measure: client service not attached")
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

	server.mu.Lock()
	serverTunnel := server.byTunnel[tunnelID]
	server.mu.Unlock()
	if serverTunnel == nil || serverTunnel.service == nil {
		t.Fatal("server platformflow service missing before HTTPS flows")
	}

	recorder.markSteady()
	initialRef := client.Ref()
	if initialRef.ID != 1 || initialRef.Generation == 0 {
		t.Fatalf("unexpected initial lane ref: %+v", initialRef)
	}

	runP5HTTPSFlow(t, recorder, svc, serverTunnel.service, innerTLS, targetAddr, 1, "first_https_flow_on_initial_outer_connection")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("first HTTPS flow replaced outer lane: got=%+v want=%+v", got, initialRef)
	}
	runP5HTTPSFlow(t, recorder, svc, serverTunnel.service, innerTLS, targetAddr, 2, "subsequent_https_flow_after_first_close_existing_lane")
	if got := client.Ref(); got != initialRef {
		t.Fatalf("second HTTPS flow replaced outer lane: got=%+v want=%+v", got, initialRef)
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

	manifest := map[string]any{
		"schema":      p5MeasurementSchema,
		"source_sha":  sourceSHA,
		"harness_sha": sourceSHA,
		"branch":      "next/tlslike-dataplane",
		"scenario": map[string]any{
			"name":                   "controlled-real-https-first-and-subsequent-flow",
			"seed":                   20260920,
			"cryptographic_rng":      "system",
			"https_flows":                  2,
			"outer_connection_count":       1,
			"sequential_close_before_next": true,
			"connection_mtu":               1500,
			"desired_lanes":          1,
			"fec_parity_shards":      0,
			"padding":                "off",
			"network_injection":      "none",
			"burst_gap_ns":           p5BurstGap.Nanoseconds(),
		},
		"capture": map[string]any{
			"type":                  "hosted-segmentio-serialized-pcap",
			"pcap":                  "outer.pcap",
			"events":                "events.jsonl",
			"capture_loss_packets":  0,
			"capture_loss_method":   "in-process SegmentIO emit interception; no kernel capture queue",
			"network_drop_injected": 0,
			"outer_packets":         outerPackets,
			"tlslike_record_events": recordEvents,
			"client_initial_syns":   clientSYNs,
		},
		"transport": map[string]any{
			"client":             clientTransport,
			"server":             serverTransport,
			"client_owner":       client.Owner().Stats(),
			"server_owner":       serverTunnel.owner.Stats(),
			"fec_recovery":       0,
			"padding_bytes":      0,
			"repair_retransmits": clientTransport.Retransmitted + serverTransport.Retransmitted,
		},
		"runner": map[string]any{
			"go_version":     runtime.Version(),
			"goos":           runtime.GOOS,
			"goarch":         runtime.GOARCH,
			"num_cpu":        runtime.NumCPU(),
			"github_actions": os.Getenv("GITHUB_ACTIONS"),
			"runner_os":      os.Getenv("RUNNER_OS"),
			"runner_arch":    os.Getenv("RUNNER_ARCH"),
			"image_os":       os.Getenv("ImageOS"),
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

	fmt.Printf("WBD_P5_HTTPS_MEASUREMENT_BASE_CAPTURED source_sha=%s flows=2 outer_connections=1 sequential_close=pass fec=off padding=off records=%d\n", sourceSHA, recordEvents)
}

func runP5HTTPSFlow(t *testing.T, recorder *p5MeasurementRecorder, svc *platformflow.Client, serverSvc *platformflow.Server, tlsCfg *tls.Config, target netip.AddrPort, ordinal int, scenario string) {
	t.Helper()
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
	tunnelConn := peer.conn

	beforeC2S, beforeS2C := recorder.wireSnapshot()
	started := time.Now()
	flowID, err := svc.AddTCP(tunnelConn, target, started)
	if err != nil {
		_ = tunnelConn.Close()
		t.Fatal(err)
	}

	tlsConn := tls.Client(appConn, tlsCfg.Clone())
	hsStarted := time.Now()
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	handshakeNS := time.Since(hsStarted).Nanoseconds()

	reqURL := &url.URL{Scheme: "https", Host: target.String(), Path: fmt.Sprintf("/flow/%d", ordinal)}
	req := &http.Request{Method: http.MethodGet, URL: reqURL, Host: "target.test", Header: make(http.Header)}
	req.Header.Set("Connection", "close")
	if err := req.Write(tlsConn); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	_ = tlsConn.Close()
	waitP5TCPFlowCount(t, "client", svc.TCPFlows, 0, 2*time.Second)
	waitP5TCPFlowCount(t, "server", serverSvc.TCPFlows, 0, 2*time.Second)
	wantBody := fmt.Sprintf("wbd-p5 path=/flow/%d", ordinal)
	if string(body) != wantBody {
		t.Fatalf("inner HTTPS body mismatch got=%q want=%q", body, wantBody)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inner HTTPS status=%d", resp.StatusCode)
	}
	afterC2S, afterS2C := recorder.wireSnapshot()
	if err := recorder.recordHTTPSFlow(p5MeasurementEvent{
		FlowOrdinal: ordinal, Scenario: scenario, OuterConnection: 1,
		BusinessFlowID: flowID, FlowClosed: true,
		HandshakeNS: handshakeNS, BusinessLatencyNS: time.Since(started).Nanoseconds(),
		HTTPStatus: resp.StatusCode, ResponseBytes: len(body),
		WireBytesC2S: afterC2S - beforeC2S, WireBytesS2C: afterS2C - beforeS2C,
	}); err != nil {
		t.Fatal(err)
	}
}


func waitP5TCPFlowCount(t *testing.T, side string, current func() int, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if got := current(); got == want {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("%s TCP flow count did not converge: got=%d want=%d", side, current(), want)
		}
		<-ticker.C
	}
}
