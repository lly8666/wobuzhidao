package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"runtime"
	"sort"
	"time"

	quic "github.com/quic-go/quic-go"
)

const (
	schemaVersion = "wbd-quic-oracle/v1"
	sampleCount   = 64
	payloadBytes  = 256
	alpn          = "wbd-quic-oracle"
)

type summary struct {
	Count int64 `json:"count"`
	P50US int64 `json:"p50_us"`
	P95US int64 `json:"p95_us"`
	P99US int64 `json:"p99_us"`
	MaxUS int64 `json:"max_us"`
}

type report struct {
	SchemaVersion     string  `json:"schema_version"`
	Implementation    string  `json:"implementation"`
	Version           string  `json:"version"`
	ReleaseCommit     string  `json:"release_commit"`
	GoVersion         string  `json:"go_version"`
	Samples           int     `json:"samples"`
	PayloadBytes      int     `json:"payload_bytes"`
	SupportsDatagrams bool    `json:"supports_datagrams"`
	StreamRTT         summary `json:"stream_rtt"`
	DatagramRTT       summary `json:"datagram_rtt"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	serverTLS, clientTLS, err := tlsConfigs()
	if err != nil {
		return err
	}
	qconf := &quic.Config{EnableDatagrams: true, MaxIdleTimeout: 5 * time.Second}
	ln, err := quic.ListenAddr("127.0.0.1:0", serverTLS, qconf)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverErr := make(chan error, 1)
	go func() { serverErr <- serve(ctx, ln) }()

	conn, err := quic.DialAddr(ctx, ln.Addr().String(), clientTLS, qconf)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseWithError(0, "done")
	if !conn.ConnectionState().SupportsDatagrams {
		return fmt.Errorf("peer did not negotiate QUIC DATAGRAM support")
	}

	streamSamples, err := streamRTT(ctx, conn)
	if err != nil {
		return err
	}
	datagramSamples, err := datagramRTT(ctx, conn)
	if err != nil {
		return err
	}

	r := report{
		SchemaVersion:     schemaVersion,
		Implementation:    "github.com/quic-go/quic-go",
		Version:           "v0.61.0",
		ReleaseCommit:     "579ee19",
		GoVersion:         runtime.Version(),
		Samples:           sampleCount,
		PayloadBytes:      payloadBytes,
		SupportsDatagrams: true,
		StreamRTT:         summarize(streamSamples),
		DatagramRTT:       summarize(datagramSamples),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	_ = conn.CloseWithError(0, "done")
	select {
	case err := <-serverErr:
		if err != nil && ctx.Err() == nil {
			return err
		}
	case <-time.After(100 * time.Millisecond):
	}
	return nil
}

func serve(ctx context.Context, ln *quic.Listener) error {
	conn, err := ln.Accept(ctx)
	if err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() { errCh <- serveStream(ctx, conn) }()
	go func() { errCh <- serveDatagrams(ctx, conn) }()
	for range 2 {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func serveStream(ctx context.Context, conn *quic.Conn) error {
	s, err := conn.AcceptStream(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	buf := make([]byte, 8+payloadBytes)
	for i := 0; i < sampleCount; i++ {
		if _, err := io.ReadFull(s, buf); err != nil {
			return err
		}
		if _, err := s.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func serveDatagrams(ctx context.Context, conn *quic.Conn) error {
	for i := 0; i < sampleCount; i++ {
		msg, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			return err
		}
		if err := conn.SendDatagram(msg); err != nil {
			return err
		}
	}
	return nil
}

func streamRTT(ctx context.Context, conn *quic.Conn) ([]time.Duration, error) {
	s, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	out := make([]time.Duration, 0, sampleCount)
	tx := make([]byte, 8+payloadBytes)
	rx := make([]byte, len(tx))
	for i := 0; i < sampleCount; i++ {
		binary.BigEndian.PutUint64(tx[:8], uint64(i))
		if _, err := rand.Read(tx[8:]); err != nil {
			return nil, err
		}
		start := time.Now()
		if _, err := s.Write(tx); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(s, rx); err != nil {
			return nil, err
		}
		if binary.BigEndian.Uint64(rx[:8]) != uint64(i) {
			return nil, fmt.Errorf("stream sequence mismatch")
		}
		out = append(out, time.Since(start))
	}
	return out, nil
}

func datagramRTT(ctx context.Context, conn *quic.Conn) ([]time.Duration, error) {
	out := make([]time.Duration, 0, sampleCount)
	tx := make([]byte, 8+payloadBytes)
	for i := 0; i < sampleCount; i++ {
		binary.BigEndian.PutUint64(tx[:8], uint64(i))
		if _, err := rand.Read(tx[8:]); err != nil {
			return nil, err
		}
		start := time.Now()
		if err := conn.SendDatagram(tx); err != nil {
			return nil, err
		}
		rx, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			return nil, err
		}
		if len(rx) < 8 || binary.BigEndian.Uint64(rx[:8]) != uint64(i) {
			return nil, fmt.Errorf("datagram sequence mismatch")
		}
		out = append(out, time.Since(start))
	}
	return out, nil
}

func summarize(v []time.Duration) summary {
	us := make([]int64, len(v))
	for i, d := range v {
		us[i] = d.Microseconds()
	}
	sort.Slice(us, func(i, j int) bool { return us[i] < us[j] })
	q := func(p float64) int64 {
		if len(us) == 0 {
			return 0
		}
		idx := int(math.Ceil(p*float64(len(us)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(us) {
			idx = len(us) - 1
		}
		return us[idx]
	}
	return summary{Count: int64(len(us)), P50US: q(0.50), P95US: q(0.95), P99US: q(0.99), MaxUS: us[len(us)-1]}
}

func tlsConfigs() (*tls.Config, *tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}),
	)
	if err != nil {
		return nil, nil, err
	}
	server := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{alpn}, MinVersion: tls.VersionTLS13}
	client := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{alpn}, MinVersion: tls.VersionTLS13}
	return server, client, nil
}
