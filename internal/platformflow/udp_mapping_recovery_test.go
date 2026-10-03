package platformflow

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
)

func TestUDPUpstreamMappingSurvivesTransportSendFailure(t *testing.T) {
	lease := testLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil { t.Fatal(err) }
	defer owner.Close()
	if _, err := owner.AttachInitial(1, testLane(t, datapath.RoleServer, lease, 41)); err != nil { t.Fatal(err) }
	var attempts atomic.Uint64
	delivered := make(chan struct{}, 1)
	channel, err := NewTunnelChannel(owner, func(out Outbound) error {
		if attempts.Add(1) == 1 { return errors.New("synthetic retired transport") }
		if len(out.Normal) == 0 { return ErrNoWireOutput }
		delivered <- struct{}{}
		return nil
	})
	if err != nil { t.Fatal(err) }
	server, err := NewUDPServer(channel, time.Minute, 4)
	if err != nil { t.Fatal(err) }
	defer server.Close()
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1)})
	if err != nil { t.Fatal(err) }
	defer peer.Close()
	if err := peer.SetDeadline(time.Now().Add(3*time.Second)); err != nil { t.Fatal(err) }
	frame := Frame{Kind: KindUDPDatagram, FlowID: 1, Peer: peer.LocalAddr().(*net.UDPAddr).AddrPort(), Payload: []byte("first-request")}
	if err := server.Handle(frame, time.Now()); err != nil { t.Fatal(err) }
	buf := make([]byte, 128)
	_, mappingAddr, err := peer.ReadFromUDPAddrPort(buf)
	if err != nil { t.Fatal(err) }
	if _, err := peer.WriteToUDPAddrPort([]byte("lost-reply"), mappingAddr); err != nil { t.Fatal(err) }
	deadline := time.Now().Add(3*time.Second)
	for server.Diagnostic().SendErrors == 0 && time.Now().Before(deadline) { time.Sleep(time.Millisecond) }
	if d := server.Diagnostic(); d.SendErrors != 1 { t.Fatalf("missing send failure: %+v",d) }
	if server.Len() != 1 { t.Fatal("one transport failure destroyed live UDP mapping") }
	if _, err := peer.WriteToUDPAddrPort([]byte("next-reply"), mappingAddr); err != nil { t.Fatal(err) }
	select { case <-delivered: case <-time.After(3*time.Second): t.Fatal("same-port next reply did not resume") }
	frame.Payload=[]byte("next-request")
	if err := server.Handle(frame,time.Now()); err != nil { t.Fatal(err) }
	_, nextAddr, err := peer.ReadFromUDPAddrPort(buf)
	if err != nil { t.Fatal(err) }
	if nextAddr != mappingAddr { t.Fatalf("mapping changed after transport loss: %v -> %v",mappingAddr,nextAddr) }
	server.Tick(time.Now().Add(2*time.Minute))
	if server.Len()!=0 { t.Fatal("preserved mapping did not expire at idle bound") }
}
