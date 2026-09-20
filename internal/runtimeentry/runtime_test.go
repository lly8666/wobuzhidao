package runtimeentry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/netip"
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

type memorySegmentEndpoint struct {
	in   <-chan faketcp.Segment
	out  chan<- faketcp.Segment
	done chan struct{}
	once sync.Once
}

func (e *memorySegmentEndpoint) io() SegmentIO {
	return SegmentIO{
		Read: func() (faketcp.Segment, error) {
			select {
			case <-e.done:
				return faketcp.Segment{}, net.ErrClosed
			case seg := <-e.in:
				return seg, nil
			}
		},
		Emit: func(seg faketcp.Segment) error {
			copySeg := seg
			copySeg.Payload = append([]byte(nil), seg.Payload...)
			select {
			case <-e.done:
				return net.ErrClosed
			case e.out <- copySeg:
				return nil
			}
		},
		Close: func() error {
			e.once.Do(func() { close(e.done) })
			return nil
		},
	}
}

func memorySegmentPair() (*memorySegmentEndpoint, *memorySegmentEndpoint) {
	c2s := make(chan faketcp.Segment, 1024)
	s2c := make(chan faketcp.Segment, 1024)
	client := &memorySegmentEndpoint{in: s2c, out: c2s, done: make(chan struct{})}
	server := &memorySegmentEndpoint{in: c2s, out: s2c, done: make(chan struct{})}
	return client, server
}

type memoryPacketWriter struct {
	mu sync.Mutex
	packets [][]byte
	notify chan struct{}
}

func newMemoryPacketWriter() *memoryPacketWriter {
	return &memoryPacketWriter{notify: make(chan struct{}, 16)}
}

func (w *memoryPacketWriter) WritePacket(packet []byte) (int, error) {
	w.mu.Lock()
	w.packets = append(w.packets, append([]byte(nil), packet...))
	w.mu.Unlock()
	select {
	case w.notify <- struct{}{}:
	default:
	}
	return len(packet), nil
}

func (w *memoryPacketWriter) waitPacket(t *testing.T, timeout time.Duration) []byte {
	t.Helper()
	select {
	case <-w.notify:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for packet")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.packets[len(w.packets)-1]...)
}

func TestSingleProcessClientServerEntryCarriesBidirectionalIPv4(t *testing.T) {
	cert := runtimeCertificate(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	var tunnelID logicaltunnel.TunnelID
	copy(tunnelID[:], []byte("runtime-entry-v1"))
	var installation logicaltunnel.InstallationID
	installation[0] = 1
	lease := logicaltunnel.Lease{
		Account: "solo",
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: "10.66.0.2/32",
			Routes4: []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}

	clientEP, serverEP := memorySegmentPair()
	defer clientEP.io().close()
	defer serverEP.io().close()

	serverTUN := newMemoryPacketWriter()
	router, err := linuxserver.NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 8, serverTUN)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{
		IO: serverEP.io(),
		ListenPort: 443,
		MaxAssociations: 16,
		InitialRTO: time.Second,
		TickInterval: 10 * time.Millisecond,
		MaxFlows: 64,
		Admission: realityfront.ServerAdmissionConfig{
			TLS: realityfront.ServerConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout: 3 * time.Second,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit: 1250,
		},
		LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
			if id != tunnelID {
				return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
			}
			return lease.Clone(), nil
		},
		Lane: datapath.ServerLaneParams{
			ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		},
		Router: router,
		Service: platformflow.DefaultServerConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()

	clientDeliver := make(chan []byte, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := DialClient(ctx, ClientConfig{
		IO: clientEP.io(),
		Flow: faketcp.ClientFlow{
			LocalIP: [4]byte{192, 0, 2, 10}, PeerIP: [4]byte{192, 0, 2, 20},
			LocalPort: 41000, PeerPort: 443,
		},
		ClientISN: 1000,
		InitialRTO: time.Second,
		Lease: lease,
		MaxFlows: 64,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second,
			},
			Username: "solo", Password: "correct-password",
			TunnelID: tunnelID.Bytes(), ClientLimit: 1300,
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU: 1500, TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		},
		Deliver: func(packets [][]byte, _ time.Time) error {
			for _, packet := range packets {
				clientDeliver <- append([]byte(nil), packet...)
			}
			return nil
		},
		TickInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	forward := ipv4Packet([4]byte{10, 66, 0, 2}, [4]byte{8, 8, 8, 8}, 17)
	if err := client.SendPacket(forward, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := serverTUN.waitPacket(t, 3*time.Second); string(got) != string(forward) {
		t.Fatalf("server TUN packet mismatch got=%x want=%x", got, forward)
	}

	reverse := ipv4Packet([4]byte{8, 8, 8, 8}, [4]byte{10, 66, 0, 2}, 6)
	if err := server.RoutePacket(reverse, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clientDeliver:
		if string(got) != string(reverse) {
			t.Fatalf("client packet mismatch got=%x want=%x", got, reverse)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reverse packet")
	}

	serverCancel()
	if err := <-serverDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func ipv4Packet(src, dst [4]byte, proto byte) []byte {
	packet := make([]byte, 20)
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	packet[8] = 64
	packet[9] = proto
	copy(packet[12:16], src[:])
	copy(packet[16:20], dst[:])
	return packet
}

func runtimeCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{CommonName: "target.test"},
		DNSNames: []string{"target.test"},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
