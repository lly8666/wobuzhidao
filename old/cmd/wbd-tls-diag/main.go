package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

type attempt struct {
	Index             int     `json:"index"`
	OK                bool    `json:"ok"`
	Error             string  `json:"error,omitempty"`
	TCPConnectMS      float64 `json:"tcp_connect_ms,omitempty"`
	TLSHandshakeMS    float64 `json:"tls_handshake_ms,omitempty"`
	TLSVersion        string  `json:"tls_version,omitempty"`
	CipherSuite       string  `json:"cipher_suite,omitempty"`
	ALPN              string  `json:"alpn,omitempty"`
	ClientHelloSHA256 string  `json:"client_hello_sha256,omitempty"`
	LeafCertSHA256    string  `json:"leaf_cert_sha256,omitempty"`
	LeafSPKISHA256    string  `json:"leaf_spki_sha256,omitempty"`
	LeafSubject       string  `json:"leaf_subject,omitempty"`
	LeafIssuer        string  `json:"leaf_issuer,omitempty"`
	LeafNotAfter      string  `json:"leaf_not_after,omitempty"`
}

type summary struct {
	Addr                string    `json:"addr"`
	ServerName          string    `json:"server_name"`
	Profile             string    `json:"profile"`
	Attempts            int       `json:"attempts"`
	Successes           int       `json:"successes"`
	Failures            int       `json:"failures"`
	TCPConnectP50MS     float64   `json:"tcp_connect_p50_ms,omitempty"`
	TCPConnectP95MS     float64   `json:"tcp_connect_p95_ms,omitempty"`
	TLSHandshakeP50MS   float64   `json:"tls_handshake_p50_ms,omitempty"`
	TLSHandshakeP95MS   float64   `json:"tls_handshake_p95_ms,omitempty"`
	Results             []attempt `json:"results"`
}

func main() {
	addr := flag.String("addr", "", "TCP endpoint host:port to connect to genuinely")
	serverName := flag.String("server-name", "", "TLS hostname to validate and send as SNI")
	count := flag.Int("count", 10, "number of independent TCP+TLS handshakes")
	timeout := flag.Duration("timeout", 5*time.Second, "per-attempt timeout")
	interval := flag.Duration("interval", 100*time.Millisecond, "delay between attempts")
	caFile := flag.String("ca-file", "", "optional PEM CA bundle for an operator-controlled endpoint")
	profile := flag.String("profile", "native", "ClientHello profile; currently only native is implemented")
	witnessOut := flag.String("witness-out", "", "demo only: write the last successful ClientHello SHA-256 as a 0600 file")
	flag.Parse()

	if *addr == "" || *serverName == "" || *count <= 0 || *count > 1000 || *timeout <= 0 || *interval < 0 {
		fatal(errors.New("-addr, -server-name and positive sane count/timeout are required"))
	}
	if *profile != "native" {
		fatal(fmt.Errorf("profile %q is not implemented in this diagnostic build; use native until a pinned browser profile is pcap-qualified", *profile))
	}
	roots, err := rootsFromFile(*caFile)
	if err != nil {
		fatal(err)
	}

	out := summary{Addr: *addr, ServerName: *serverName, Profile: *profile, Attempts: *count}
	var tcpTimes, tlsTimes []float64
	lastWitness := ""
	for i := 0; i < *count; i++ {
		r := runAttempt(i, *addr, *serverName, *timeout, roots)
		out.Results = append(out.Results, r)
		if r.OK {
			out.Successes++
			tcpTimes = append(tcpTimes, r.TCPConnectMS)
			tlsTimes = append(tlsTimes, r.TLSHandshakeMS)
			if r.ClientHelloSHA256 != "" {
				lastWitness = r.ClientHelloSHA256
			}
		} else {
			out.Failures++
		}
		if i+1 != *count && *interval != 0 {
			time.Sleep(*interval)
		}
	}
	if len(tcpTimes) != 0 {
		out.TCPConnectP50MS = percentile(tcpTimes, 0.50)
		out.TCPConnectP95MS = percentile(tcpTimes, 0.95)
		out.TLSHandshakeP50MS = percentile(tlsTimes, 0.50)
		out.TLSHandshakeP95MS = percentile(tlsTimes, 0.95)
	}
	if strings.TrimSpace(*witnessOut) != "" {
		if lastWitness == "" {
			fatal(errors.New("-witness-out requested but no successful TLS ClientHello was captured"))
		}
		if err := os.WriteFile(*witnessOut, []byte(lastWitness+"\n"), 0o600); err != nil {
			fatal(err)
		}
		if err := os.Chmod(*witnessOut, 0o600); err != nil {
			fatal(err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fatal(err)
	}
}

func runAttempt(index int, addr, serverName string, timeout time.Duration, roots *x509.CertPool) attempt {
	r := attempt{Index: index}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	d := net.Dialer{}
	t0 := time.Now()
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.TCPConnectMS = float64(time.Since(t0)) / float64(time.Millisecond)
	defer raw.Close()

	cfg := &tls.Config{
		ServerName: serverName,
		RootCAs: roots,
		MinVersion: tls.VersionTLS12,
	}
	captured := newHelloCaptureConn(raw)
	conn := tls.Client(captured, cfg)
	t1 := time.Now()
	if err := conn.HandshakeContext(ctx); err != nil {
		r.ClientHelloSHA256 = captured.SHA256Hex()
		r.Error = err.Error()
		return r
	}
	r.TLSHandshakeMS = float64(time.Since(t1)) / float64(time.Millisecond)
	r.ClientHelloSHA256 = captured.SHA256Hex()
	st := conn.ConnectionState()
	r.OK = true
	r.TLSVersion = tlsVersion(st.Version)
	r.CipherSuite = tls.CipherSuiteName(st.CipherSuite)
	r.ALPN = st.NegotiatedProtocol
	if len(st.PeerCertificates) != 0 {
		leaf := st.PeerCertificates[0]
		certHash := sha256.Sum256(leaf.Raw)
		spkiHash := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		r.LeafCertSHA256 = hex.EncodeToString(certHash[:])
		r.LeafSPKISHA256 = hex.EncodeToString(spkiHash[:])
		r.LeafSubject = leaf.Subject.String()
		r.LeafIssuer = leaf.Issuer.String()
		r.LeafNotAfter = leaf.NotAfter.UTC().Format(time.RFC3339)
	}
	_ = conn.Close()
	return r
}

func rootsFromFile(path string) (*x509.CertPool, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil // platform roots
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("no certificates parsed from -ca-file")
	}
	return pool, nil
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS1.0"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}

func percentile(in []float64, q float64) float64 {
	if len(in) == 0 {
		return 0
	}
	v := append([]float64(nil), in...)
	sort.Float64s(v)
	if q <= 0 {
		return v[0]
	}
	if q >= 1 {
		return v[len(v)-1]
	}
	pos := q * float64(len(v)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return v[lo]
	}
	return v[lo] + (v[hi]-v[lo])*(pos-float64(lo))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "WBD_TLS_DIAG_FAIL", err)
	os.Exit(1)
}
