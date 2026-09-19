package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type p2WireCapture struct {
	mu     sync.Mutex
	client []faketcp.Segment
	server []faketcp.Segment
}

func (c *p2WireCapture) addClient(seg faketcp.Segment) {
	c.mu.Lock()
	c.client = append(c.client, cloneWireSegment(seg))
	c.mu.Unlock()
}

func (c *p2WireCapture) addServer(seg faketcp.Segment) {
	c.mu.Lock()
	c.server = append(c.server, cloneWireSegment(seg))
	c.mu.Unlock()
}

func (c *p2WireCapture) snapshot() (client, server []faketcp.Segment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client = append([]faketcp.Segment(nil), c.client...)
	server = append([]faketcp.Segment(nil), c.server...)
	return client, server
}

func cloneWireSegment(seg faketcp.Segment) faketcp.Segment {
	seg.Payload = append([]byte(nil), seg.Payload...)
	return seg
}

func TestP2HostedWireQualificationRealTLSAdmission(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer assoc.Close()
	defer peer.Close()

	var capture p2WireCapture
	peer.onClientPayload = capture.addClient
	peer.onServerPayload = capture.addServer

	nonce := [16]byte{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
	type serverResult struct {
		session *ServerAdmissionSession
		err     error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		session, err := EstablishServer(context.Background(), assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    3 * time.Second,
			},
			ExpectedUsername: "wire-user-7c9f",
			ExpectedPassword: "wire-password-b41e",
			ServerLimit:      1360,
			Random:           bytes.NewReader(nonce[:]),
		})
		serverDone <- serverResult{session: session, err: err}
	}()

	client, err := EstablishClient(context.Background(), peer, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test",
			RouteKey:   routeKey,
			Timeout:    3 * time.Second,
		},
		Username:    "wire-user-7c9f",
		Password:    "wire-password-b41e",
		TunnelID:    []byte("wire-tunnel-16xx"),
		ClientLimit: 1360,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := <-serverDone
	if server.err != nil {
		t.Fatal(server.err)
	}
	if client.Negotiated.Keys != server.session.Negotiated.Keys {
		t.Fatal("real TLS admission keys differ")
	}

	clientSegs, serverSegs := capture.snapshot()
	if len(clientSegs) == 0 || len(serverSegs) == 0 {
		t.Fatalf("capture client=%d server=%d", len(clientSegs), len(serverSegs))
	}
	clientTLS := assertHostedWireDirection(t, "client", clientSegs)
	serverTLS := assertHostedWireDirection(t, "server", serverSegs)
	assertTLSRecordStream(t, "client", clientTLS)
	assertTLSRecordStream(t, "server", serverTLS)

	for _, secret := range [][]byte{
		[]byte(admissionMagic),
		[]byte("wire-user-7c9f"),
		[]byte("wire-password-b41e"),
		[]byte("wire-tunnel-16xx"),
	} {
		if bytes.Contains(clientTLS, secret) || bytes.Contains(serverTLS, secret) {
			t.Fatalf("protected admission value leaked in clear TLS-shaped wire payload: %q", secret)
		}
	}

	// The public SYN continues to expose only the standard TCP option profile.
	synPacket := faketcp.MarshalSegment(peer.base, 1, faketcp.PacketPersonaLegacy)
	syn, err := faketcp.ParseIPv4TCP(synPacket)
	if err != nil {
		t.Fatal(err)
	}
	if !faketcp.IsWBDHandshakeSegment(syn) ||
		!syn.MSSSet || syn.MSS != faketcp.DefaultMSS ||
		!syn.SACKPermitted ||
		!syn.WindowScaleSet || syn.WindowScale != faketcp.DefaultWindowScale {
		t.Fatalf("unexpected SYN wire profile: %#v", syn)
	}
	if got := synPacket[20+12] >> 4; got != 8 {
		t.Fatalf("SYN tcp data offset=%d want=8", got)
	}
}

func assertHostedWireDirection(t *testing.T, name string, segs []faketcp.Segment) []byte {
	t.Helper()
	var stream []byte
	var wantSeq uint32
	for i, seg := range segs {
		if len(seg.Payload) == 0 {
			t.Fatalf("%s segment %d unexpectedly has no payload", name, i)
		}
		if len(seg.Payload) > faketcp.DefaultBootstrapChunk ||
			len(seg.Payload) > faketcp.DefaultMSS {
			t.Fatalf("%s segment %d payload=%d exceeds bootstrap/MSS bound", name, i, len(seg.Payload))
		}
		if i != 0 && seg.Seq != wantSeq {
			t.Fatalf("%s segment %d seq=%d want=%d", name, i, seg.Seq, wantSeq)
		}
		wantSeq = seg.Seq + uint32(len(seg.Payload))

		packet := faketcp.MarshalSegment(seg, uint16(i+10), faketcp.PacketPersonaLegacy)
		if len(packet) != 40+len(seg.Payload) {
			t.Fatalf("%s segment %d packet len=%d payload=%d", name, i, len(packet), len(seg.Payload))
		}
		if got := int(binary.BigEndian.Uint16(packet[2:4])); got != len(packet) {
			t.Fatalf("%s segment %d ipv4 total=%d want=%d", name, i, got, len(packet))
		}
		if frag := binary.BigEndian.Uint16(packet[6:8]); frag != 0x4000 {
			t.Fatalf("%s segment %d fragment field=%#x want DF-only", name, i, frag)
		}
		if packet[20+12]>>4 != 5 {
			t.Fatalf("%s segment %d has unexpected TCP options/data offset=%d", name, i, packet[32]>>4)
		}
		if wireChecksum(packet[:20]) != 0 {
			t.Fatalf("%s segment %d bad IPv4 checksum", name, i)
		}
		if wireTCPChecksum(seg.SrcIP, seg.DstIP, packet[20:]) != 0 {
			t.Fatalf("%s segment %d bad TCP checksum", name, i)
		}
		parsed, err := faketcp.ParseIPv4TCP(packet)
		if err != nil {
			t.Fatalf("%s segment %d parse: %v", name, i, err)
		}
		if parsed.SrcIP != seg.SrcIP || parsed.DstIP != seg.DstIP ||
			parsed.SrcPort != seg.SrcPort || parsed.DstPort != seg.DstPort ||
			parsed.Seq != seg.Seq || parsed.Ack != seg.Ack ||
			parsed.Flags != seg.Flags || !bytes.Equal(parsed.Payload, seg.Payload) {
			t.Fatalf("%s segment %d packet round-trip changed segment", name, i)
		}
		stream = append(stream, seg.Payload...)
	}
	return stream
}

func assertTLSRecordStream(t *testing.T, name string, stream []byte) {
	t.Helper()
	records := 0
	applicationRecords := 0
	for off := 0; off < len(stream); {
		if len(stream)-off < 5 {
			t.Fatalf("%s TLS stream has %d trailing bytes", name, len(stream)-off)
		}
		typ := stream[off]
		if typ < 20 || typ > 23 {
			t.Fatalf("%s TLS record %d content type=%d", name, records, typ)
		}
		if stream[off+1] != 3 || stream[off+2] < 1 || stream[off+2] > 3 {
			t.Fatalf("%s TLS record %d legacy version=%02x%02x", name, records, stream[off+1], stream[off+2])
		}
		n := int(binary.BigEndian.Uint16(stream[off+3 : off+5]))
		end := off + 5 + n
		if end > len(stream) {
			t.Fatalf("%s TLS record %d length=%d exceeds remaining=%d", name, records, n, len(stream)-off-5)
		}
		if typ == 23 {
			applicationRecords++
		}
		records++
		off = end
	}
	if records == 0 || applicationRecords == 0 {
		t.Fatalf("%s TLS stream records=%d application=%d", name, records, applicationRecords)
	}
}

func wireChecksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b[:2]))
		b = b[2:]
	}
	if len(b) != 0 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func wireTCPChecksum(src, dst [4]byte, tcp []byte) uint16 {
	var pseudo []byte
	pseudo = append(pseudo, src[:]...)
	pseudo = append(pseudo, dst[:]...)
	pseudo = append(pseudo, 0, 6)
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(tcp)))
	pseudo = append(pseudo, length[:]...)
	pseudo = append(pseudo, tcp...)
	return wireChecksum(pseudo)
}
