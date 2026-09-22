package platformflow

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

type startupHelloConn struct{ bytes.Buffer }

func (c *startupHelloConn) Read([]byte) (int, error)       { return 0, io.EOF }
func (*startupHelloConn) Close() error                     { return nil }
func (*startupHelloConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*startupHelloConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*startupHelloConn) SetDeadline(time.Time) error      { return nil }
func (*startupHelloConn) SetReadDeadline(time.Time) error  { return nil }
func (*startupHelloConn) SetWriteDeadline(time.Time) error { return nil }

func TestStartupPaddingRealClientHelloPlatformEnvelope(t *testing.T) {
	// Exercise an independently produced ClientHello and the production frame
	// serializer, rather than accepting only the detector's synthetic fixture.
	conn := &startupHelloConn{}
	client := tls.Client(conn, &tls.Config{ServerName: "example.com", MinVersion: tls.VersionTLS13})
	if err := client.Handshake(); err == nil {
		t.Fatal("fixture must terminate at EOF")
	}
	hello := append([]byte(nil), conn.Bytes()...)
	if len(hello) < 50 {
		t.Fatal("no real ClientHello")
	}
	newOwner := func(role datapath.Role) (*datapath.TunnelOwner, logicaltunnel.LaneRef) {
		o, err := datapath.NewTunnelOwner(1, 4)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(o.Close)
		p := datapath.TLSStartupPaddingPolicy(true)
		p.BytesPerRecord = 1
		p.MaxPaddingRatioPPM = datapath.PaddingRatioScalePPM
		if err = o.ConfigurePadding(p); err != nil {
			t.Fatal(err)
		}
		mtu := pathmtu.Config{ConnectionMTU: 1500, IPv4HeaderLen: 20, TCPHeaderLen: 20, RecordWireLimit: 1250}
		l, err := datapath.NewLane(datapath.LaneConfig{Role: role, TunnelID: []byte("0123456789abcdef"), ClientRecordLimit: 1250, ServerRecordLimit: 1250, TxMTU: mtu, RxMTU: mtu})
		if err != nil {
			t.Fatal(err)
		}
		snap, err := o.AttachInitial(1, l)
		if err != nil {
			t.Fatal(err)
		}
		return o, snap.Ref
	}
	c, cref := newOwner(datapath.RoleClient)
	s, sref := newOwner(datapath.RoleServer)
	lease := netip.MustParseAddr("10.66.0.2")
	now := time.Unix(509, 0)
	for _, part := range []struct {
		off  int
		data []byte
	}{{0, hello[:7]}, {7, hello[7:]}} {
		packet, err := MarshalPacket(lease, Frame{Kind: KindTCPData, FlowID: 7, Offset: uint64(part.off), Payload: part.data})
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.NormalOutbound(packet, now)
		if err != nil || len(r) == 0 {
			t.Fatalf("no immediate output: %v", err)
		}
		var got [][]byte
		for _, w := range r {
			in, err := s.InboundPayload(sref, w.Wire, now)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, in.Datagrams...)
		}
		if len(got) != 1 || !bytes.Equal(got[0], packet) {
			t.Fatal("service bytes changed/held")
		}
	}
	if c.Stats().Padding.StartupDetected != 1 || s.Stats().Padding.StartupDetected != 1 {
		t.Fatal("real TLS or platform envelope detection missing")
	}
	reply, err := MarshalPacket(lease, Frame{Kind: KindTCPData, FlowID: 7, Payload: []byte("server flight")})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.NormalOutbound(reply, now)
	if err != nil || len(r) != 1 || r[0].PaddingBytes != 1 {
		t.Fatalf("server startup not padded: %v %v", r, err)
	}
	in, err := c.InboundPayload(cref, r[0].Wire, now)
	if err != nil || len(in.Datagrams) != 1 || !bytes.Equal(in.Datagrams[0], reply) {
		t.Fatal("reply damaged")
	}
	// A different FlowID has no inherited TLS eligibility.
	other, _ := MarshalPacket(lease, Frame{Kind: KindTCPData, FlowID: 8, Payload: []byte("ordinary")})
	r, err = s.NormalOutbound(other, now)
	if err != nil || r[0].PaddingBytes != 0 {
		t.Fatal("eligibility leaked across flow IDs")
	}
}
