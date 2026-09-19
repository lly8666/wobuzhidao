package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

const (
	vectorSchema       = "wbd-winlin-wire-vector/v1"
	vectorServerName   = "winlin.test"
	vectorRouteKey     = "0123456789abcdef0123456789abcdef"
	vectorUsername     = "winlin-user"
	vectorPassword     = "winlin-password"
	vectorInstallation = "00112233445566778899aabbccddeeff"
	vectorClientPort   = uint16(41001)
	vectorServerPort   = uint16(443)
	vectorClientISN    = uint32(0x10203040)
	vectorServerISN    = uint32(0x55667788)
	vectorChunk        = 1200
)

var (
	vectorClientIP = [4]byte{192, 0, 2, 10}
	vectorServerIP = [4]byte{198, 51, 100, 20}
	postBarrierProbe = []byte("WBD_POST_BOOTSTRAP_PACKET_MODE_V1")
)

type packetVector struct {
	Kind  string `json:"kind"`
	Bytes string `json:"bytes_b64"`
}

type wireVector struct {
	Schema           string                     `json:"schema"`
	SourceSHA        string                     `json:"source_sha"`
	ProducerGOOS     string                     `json:"producer_goos"`
	PacketPersona    string                     `json:"packet_persona"`
	ServerName       string                     `json:"server_name"`
	ClientIP         string                     `json:"client_ip"`
	ServerIP         string                     `json:"server_ip"`
	ClientPort       uint16                     `json:"client_port"`
	ServerPort       uint16                     `json:"server_port"`
	ClientISN        uint32                     `json:"client_isn"`
	ServerISN        uint32                     `json:"server_isn"`
	TLSVersion       uint16                     `json:"tls_version"`
	TunnelConfig     logicaltunnel.TunnelConfig `json:"tunnel_config"`
	ClientTLSLength  int                        `json:"client_tls_length"`
	ClientTLSSHA256  string                     `json:"client_tls_sha256"`
	PostBarrierProbe string                     `json:"post_barrier_probe_sha256"`
	Packets          []packetVector             `json:"packets"`
}

type recordingConn struct {
	net.Conn
	mu     sync.Mutex
	writes []byte
}

func (c *recordingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.mu.Lock()
		c.writes = append(c.writes, p[:n]...)
		c.mu.Unlock()
	}
	return n, err
}

func (c *recordingConn) Snapshot() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.writes...)
}

type readOnlyConn struct {
	*bytes.Reader
}

func (c *readOnlyConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *readOnlyConn) Close() error                { return nil }
func (c *readOnlyConn) LocalAddr() net.Addr         { return vectorAddr("vector-local") }
func (c *readOnlyConn) RemoteAddr() net.Addr        { return vectorAddr("vector-remote") }
func (c *readOnlyConn) SetDeadline(time.Time) error      { return nil }
func (c *readOnlyConn) SetReadDeadline(time.Time) error  { return nil }
func (c *readOnlyConn) SetWriteDeadline(time.Time) error { return nil }

type vectorAddr string

func (a vectorAddr) Network() string { return "wbd-vector" }
func (a vectorAddr) String() string  { return string(a) }

type serverBootstrapResult struct {
	auth realityfront.AuthenticatedTunnel
	err  error
}

func main() {
	mode := flag.String("mode", "", "generate or verify")
	file := flag.String("file", "", "wire-vector JSON path")
	sourceSHA := flag.String("source-sha", "", "exact Git source SHA")
	flag.Parse()

	if strings.TrimSpace(*file) == "" || strings.TrimSpace(*sourceSHA) == "" {
		fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector: -file and -source-sha are required")
		os.Exit(2)
	}

	switch *mode {
	case "generate":
		if runtime.GOOS != "windows" {
			fmt.Fprintf(os.Stderr, "wbd-winlin-wire-vector: native Windows generation required; got %s\n", runtime.GOOS)
			os.Exit(2)
		}
		if faketcp.DefaultPacketPersona != faketcp.PacketPersonaWindows11 {
			fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector: Windows build did not select Windows11 packet persona")
			os.Exit(1)
		}
		v, err := generateVector(strings.TrimSpace(*sourceSHA), faketcp.DefaultPacketPersona, runtime.GOOS)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector generate:", err)
			os.Exit(1)
		}
		if err := writeVector(*file, v); err != nil {
			fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector write:", err)
			os.Exit(1)
		}
		fmt.Printf("WBD_WINLIN_VECTOR_GENERATE_PASS source_sha=%s producer=windows persona=%s tls13=1 packets=%d same_flow=1 physical_npcap=0\n", v.SourceSHA, v.PacketPersona, len(v.Packets))
	case "verify":
		v, err := readVector(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector read:", err)
			os.Exit(1)
		}
		if err := verifyVector(v, strings.TrimSpace(*sourceSHA)); err != nil {
			fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector verify:", err)
			os.Exit(1)
		}
		fmt.Printf("WBD_WINLIN_VECTOR_VERIFY_PASS source_sha=%s producer=windows persona=windows11 server_association=1 reality_marker=1 no_second_syn=1 same_sequence_space=1 physical_npcap=0\n", v.SourceSHA)
	default:
		fmt.Fprintln(os.Stderr, "wbd-winlin-wire-vector: -mode must be generate or verify")
		os.Exit(2)
	}
}

func generateVector(sourceSHA string, persona faketcp.PacketPersona, producerGOOS string) (wireVector, error) {
	var zero wireVector
	if strings.TrimSpace(sourceSHA) == "" {
		return zero, errors.New("source SHA is required")
	}
	if producerGOOS != "windows" {
		return zero, fmt.Errorf("producer GOOS=%q want windows", producerGOOS)
	}
	if persona != faketcp.PacketPersonaWindows11 {
		return zero, fmt.Errorf("packet persona=%s want windows11", persona)
	}

	clientTLS, auth, tlsVersion, err := captureWindowsBootstrap()
	if err != nil {
		return zero, err
	}
	if len(clientTLS) == 0 || tlsVersion != tls.VersionTLS13 {
		return zero, errors.New("captured bootstrap is not TLS 1.3")
	}

	v := wireVector{
		Schema: vectorSchema, SourceSHA: sourceSHA, ProducerGOOS: producerGOOS,
		PacketPersona: persona.String(), ServerName: vectorServerName,
		ClientIP: net.IP(vectorClientIP[:]).String(), ServerIP: net.IP(vectorServerIP[:]).String(),
		ClientPort: vectorClientPort, ServerPort: vectorServerPort,
		ClientISN: vectorClientISN, ServerISN: vectorServerISN,
		TLSVersion: tlsVersion, TunnelConfig: auth.Config,
		ClientTLSLength: len(clientTLS), ClientTLSSHA256: sha256Hex(clientTLS),
		PostBarrierProbe: sha256Hex(postBarrierProbe),
	}

	appendPacket := func(kind string, pkt []byte) {
		v.Packets = append(v.Packets, packetVector{Kind: kind, Bytes: base64.StdEncoding.EncodeToString(pkt)})
	}
	appendPacket("syn", marshalClientPacket(persona, vectorClientISN, 0, faketcp.FlagSYN, nil, 1))
	appendPacket("final_ack", marshalClientPacket(persona, vectorClientISN+1, vectorServerISN+1, faketcp.FlagACK, nil, 2))

	seq := vectorClientISN + 1
	ipID := uint16(3)
	for off := 0; off < len(clientTLS); {
		n := len(clientTLS) - off
		if n > vectorChunk {
			n = vectorChunk
		}
		payload := clientTLS[off : off+n]
		appendPacket("bootstrap", marshalClientPacket(persona, seq, vectorServerISN+1, faketcp.FlagACK|faketcp.FlagPSH, payload, ipID))
		seq += uint32(n)
		ipID++
		off += n
	}
	appendPacket("post_barrier", marshalClientPacket(persona, seq, vectorServerISN+1, faketcp.FlagACK|faketcp.FlagPSH, postBarrierProbe, ipID))
	return v, nil
}

func captureWindowsBootstrap() ([]byte, realityfront.AuthenticatedTunnel, uint16, error) {
	var zero realityfront.AuthenticatedTunnel
	cert, err := vectorCertificate()
	if err != nil {
		return nil, zero, 0, err
	}
	manager, err := logicaltunnel.ParseManager("10.66.0.0/24", []string{"0.0.0.0/0"})
	if err != nil {
		return nil, zero, 0, err
	}
	installation, err := logicaltunnel.ParseInstallationID(vectorInstallation)
	if err != nil {
		return nil, zero, 0, err
	}
	ticketDir, err := os.MkdirTemp("", "wbd-winlin-vector-ticket-")
	if err != nil {
		return nil, zero, 0, err
	}
	defer os.RemoveAll(ticketDir)

	clientRaw, serverRaw := net.Pipe()
	clientConn := &recordingConn{Conn: clientRaw}
	defer clientConn.Close()
	defer serverRaw.Close()

	serverDone := make(chan serverBootstrapResult, 1)
	go func() {
		auth, serverErr := realityfront.BootstrapServerSingleFlowV2(context.Background(), serverRaw, realityfront.SingleFlowServerV2Config{
			SingleFlowServerConfig: realityfront.SingleFlowServerConfig{
				ServerName: vectorServerName, RouteKey: []byte(vectorRouteKey),
				ExpectedUsername: vectorUsername, ExpectedPassword: vectorPassword,
				TicketDir: ticketDir, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, Timeout: 5 * time.Second,
			},
			LeaseProvider: manager,
		})
		serverDone <- serverBootstrapResult{auth: auth, err: serverErr}
	}()

	clientAuth, err := realityfront.BootstrapClientSingleFlowV2(context.Background(), clientConn, realityfront.SingleFlowClientV2Config{
		SingleFlowClientConfig: realityfront.SingleFlowClientConfig{
			ServerName: vectorServerName, RouteKey: []byte(vectorRouteKey), Username: vectorUsername, Password: vectorPassword,
			VerifyServer: false, Timeout: 5 * time.Second,
		},
		InstallationID: installation,
	})
	if err != nil {
		_ = clientConn.Close()
		_ = serverRaw.Close()
		s := <-serverDone
		return nil, zero, 0, fmt.Errorf("client V2 bootstrap: %w; server=%v", err, s.err)
	}
	s := <-serverDone
	if s.err != nil {
		return nil, zero, 0, fmt.Errorf("server V2 bootstrap: %w", s.err)
	}
	if clientAuth.Config.TunnelID != s.auth.Config.TunnelID || clientAuth.Config.Address4 != s.auth.Config.Address4 {
		return nil, zero, 0, errors.New("client/server authenticated tunnel config differs")
	}
	return clientConn.Snapshot(), clientAuth.AuthenticatedTunnel, clientAuth.TLSState.Version, nil
}

func vectorCertificate() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: vectorServerName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), DNSNames: []string{vectorServerName},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, nil
}

func marshalClientPacket(persona faketcp.PacketPersona, seq, ack uint32, flags uint8, payload []byte, ipID uint16) []byte {
	buf := make([]byte, faketcp.PacketLen(flags, len(payload)))
	pkt := faketcp.MarshalIPv4TCPSACKPersonaInto(buf, vectorClientIP, vectorServerIP, vectorClientPort, vectorServerPort, seq, ack, flags, 65535, nil, payload, ipID, persona)
	return append([]byte(nil), pkt...)
}

func verifyVector(v wireVector, sourceSHA string) error {
	if v.Schema != vectorSchema {
		return fmt.Errorf("schema=%q want %q", v.Schema, vectorSchema)
	}
	if v.SourceSHA != sourceSHA {
		return fmt.Errorf("source SHA=%q want %q", v.SourceSHA, sourceSHA)
	}
	if v.ProducerGOOS != "windows" || v.PacketPersona != "windows11" {
		return fmt.Errorf("producer/persona=%s/%s want windows/windows11", v.ProducerGOOS, v.PacketPersona)
	}
	if v.ServerName != vectorServerName || v.ClientPort != vectorClientPort || v.ServerPort != vectorServerPort || v.ClientISN != vectorClientISN || v.ServerISN != vectorServerISN {
		return errors.New("vector flow identity differs from contract")
	}
	if v.TLSVersion != tls.VersionTLS13 || v.ClientTLSLength <= 0 {
		return errors.New("vector does not contain a TLS 1.3 bootstrap")
	}
	if err := v.TunnelConfig.Validate(); err != nil {
		return fmt.Errorf("tunnel config: %w", err)
	}
	if len(v.Packets) < 4 || v.Packets[0].Kind != "syn" || v.Packets[1].Kind != "final_ack" || v.Packets[len(v.Packets)-1].Kind != "post_barrier" {
		return errors.New("vector packet ordering is incomplete")
	}

	decoded := make([]faketcp.Segment, 0, len(v.Packets))
	for i, p := range v.Packets {
		raw, err := base64.StdEncoding.DecodeString(p.Bytes)
		if err != nil {
			return fmt.Errorf("packet %d base64: %w", i, err)
		}
		if len(raw) < 20 || raw[8] != 128 {
			return fmt.Errorf("packet %d does not retain Windows IPv4 TTL persona", i)
		}
		seg, err := faketcp.ParseIPv4TCP(raw)
		if err != nil {
			return fmt.Errorf("packet %d parse: %w", i, err)
		}
		if seg.SrcIP != vectorClientIP || seg.DstIP != vectorServerIP || seg.SrcPort != vectorClientPort || seg.DstPort != vectorServerPort {
			return fmt.Errorf("packet %d changed four-tuple", i)
		}
		if i > 0 && seg.Flags&faketcp.FlagSYN != 0 {
			return fmt.Errorf("packet %d contains a second SYN", i)
		}
		decoded = append(decoded, seg)
	}

	syn := decoded[0]
	if !faketcp.IsWBDHandshakeSegment(syn) || syn.Seq != vectorClientISN {
		return errors.New("Windows SYN does not match WBD handshake persona")
	}
	assoc, err := faketcp.NewServerAssociation(syn, vectorServerISN, faketcp.RecoveryLegacy, time.Second)
	if err != nil {
		return fmt.Errorf("server association: %w", err)
	}
	if err := assoc.HandleHandshakeACK(decoded[1]); err != nil {
		return fmt.Errorf("final ACK: %w", err)
	}

	var delivered []byte
	for i := 2; i < len(decoded); i++ {
		result, err := assoc.HandleSegment(decoded[i], time.Unix(1, int64(i)))
		if err != nil {
			return fmt.Errorf("packet %d server handle: %w", i, err)
		}
		if len(decoded[i].Payload) != 0 && len(result.Deliver) == 0 {
			return fmt.Errorf("packet %d payload was not first-arrival deliverable", i)
		}
		delivered = append(delivered, result.Deliver...)
	}
	if len(delivered) != v.ClientTLSLength+len(postBarrierProbe) {
		return fmt.Errorf("delivered bytes=%d want %d", len(delivered), v.ClientTLSLength+len(postBarrierProbe))
	}
	clientTLS := delivered[:v.ClientTLSLength]
	probe := delivered[v.ClientTLSLength:]
	if sha256Hex(clientTLS) != v.ClientTLSSHA256 {
		return errors.New("recovered TLS bootstrap digest differs from Windows producer")
	}
	if !bytes.Equal(probe, postBarrierProbe) || sha256Hex(probe) != v.PostBarrierProbe {
		return errors.New("post-bootstrap packet-mode payload did not preserve same sequence space")
	}
	hello, err := realityfront.ReadSingleFlowHello(&readOnlyConn{Reader: bytes.NewReader(clientTLS)}, vectorServerName, []byte(vectorRouteKey), time.Second)
	if err != nil {
		return fmt.Errorf("Reality-like ClientHello parse: %w", err)
	}
	if !hello.Recognized || hello.Info.ServerName != vectorServerName {
		return errors.New("Windows-produced ClientHello lost Reality-like marker/SNI on Linux consume")
	}
	return nil
}

func writeVector(path string, v wireVector) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o600)
}

func readVector(path string) (wireVector, error) {
	var v wireVector
	body, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return v, err
	}
	return v, nil
}

func sha256Hex(p []byte) string {
	sum := sha256.Sum256(p)
	return hex.EncodeToString(sum[:])
}

var _ io.Reader = (*readOnlyConn)(nil)
