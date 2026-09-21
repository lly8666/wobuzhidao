package runtimeentry

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformflow"
)

type p5WeakRequestSample struct {
	Schema                string `json:"schema"`
	Event                 string `json:"event"`
	Ordinal               int    `json:"ordinal"`
	ScheduledStartNS      int64  `json:"scheduled_start_ns"`
	ActualStartNS         int64  `json:"actual_start_ns,omitempty"`
	SendLagNS             int64  `json:"send_lag_ns,omitempty"`
	ScheduledPhase        string `json:"scheduled_phase"`
	ActualPhase           string `json:"actual_phase,omitempty"`
	Success               bool   `json:"success"`
	ErrorStage            string `json:"error_stage,omitempty"`
	Error                 string `json:"error,omitempty"`
	BusinessFlowID        uint64 `json:"business_flow_id,omitempty"`
	HandshakeNS           int64  `json:"handshake_ns,omitempty"`
	RequestStartNS        int64  `json:"request_start_ns,omitempty"`
	RequestCompleteNS     int64  `json:"request_complete_ns,omitempty"`
	RTTNS                 int64  `json:"rtt_ns,omitempty"`
	HTTPStatus            int    `json:"http_status,omitempty"`
	ResponseBytes         int    `json:"response_bytes,omitempty"`
	ExpectedResponseBytes int    `json:"expected_response_bytes"`
	InnerTLSWireBytesC2S  uint64 `json:"inner_tls_wire_bytes_c2s,omitempty"`
	InnerTLSWireBytesS2C  uint64 `json:"inner_tls_wire_bytes_s2c,omitempty"`
	BodySHA256            string `json:"body_sha256,omitempty"`
}

func p5WeakBody(ordinal, size int) []byte {
	b := make([]byte, size)
	prefix := []byte(fmt.Sprintf("wbd-p5-weak-%03d:", ordinal))
	copy(b, prefix)
	for i := len(prefix); i < len(b); i++ {
		b[i] = byte('a' + (ordinal+i)%26)
	}
	return b
}

func p5WeakPhaseForOffset(offset time.Duration) string {
	for _, phase := range p5WeakPhases {
		if offset >= phase.Start && offset < phase.End {
			return phase.Name
		}
	}
	return "post"
}

func executeP5WeakHTTPSRequest(t *testing.T, svc *platformflow.Client, certificate *p5CertificateScenario, scenarioStart time.Time, ordinal, responseSize int, scheduled time.Duration) p5WeakRequestSample {
	t.Helper()
	sample := p5WeakRequestSample{
		Schema:                p5MeasurementSchema,
		Event:                 "https_request",
		Ordinal:               ordinal,
		ScheduledStartNS:      scheduled.Nanoseconds(),
		ScheduledPhase:        p5WeakPhaseForOffset(scheduled),
		ExpectedResponseBytes: responseSize,
	}
	actualStart := time.Now()
	if actualStart.Sub(scenarioStart) >= p5WeakScenarioTime {
		sample.ErrorStage = "scenario_end_before_start"
		sample.Error = "scheduled request could not start before the 120s measurement window ended"
		return sample
	}
	sample.ActualStartNS = actualStart.Sub(scenarioStart).Nanoseconds()
	sample.SendLagNS = actualStart.Sub(scenarioStart.Add(scheduled)).Nanoseconds()
	sample.ActualPhase = p5WeakPhaseForOffset(actualStart.Sub(scenarioStart))

	deadline := actualStart.Add(p5WeakRequestLimit)
	if scenarioEnd := scenarioStart.Add(p5WeakScenarioTime); deadline.After(scenarioEnd) {
		deadline = scenarioEnd
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		sample.ErrorStage, sample.Error = "local_listen", err.Error()
		return sample
	}
	type acceptResult struct {
		conn net.Conn
		err  error
	}
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
		t.Fatalf("weak request %d TLS state version=0x%x resumed=%v", ordinal, state.Version, state.DidResume)
	}
	certificate.verifyState(t, state)

	requestStart := time.Now()
	sample.RequestStartNS = requestStart.Sub(scenarioStart).Nanoseconds()
	reqURL := &url.URL{
		Scheme:   "https",
		Host:     certificate.targetAddr.String(),
		Path:     "/weak",
		RawQuery: fmt.Sprintf("ordinal=%d&size=%d", ordinal, responseSize),
	}
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
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, expected) {
		t.Fatalf("weak HTTPS integrity ordinal=%d status=%d bytes=%d expected=%d", ordinal, resp.StatusCode, len(body), len(expected))
	}
	sum := sha256.Sum256(body)
	sample.BodySHA256 = fmt.Sprintf("%x", sum[:])
	sample.Success = true
	return sample
}
