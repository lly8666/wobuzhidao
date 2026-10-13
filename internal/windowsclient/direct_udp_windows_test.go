//go:build windows

package windowsclient

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"errors"
	"net/netip"
	"testing"
)

func TestN4NativeUDPWindowsConstructorAndGenerationCloseFence(t *testing.T) {
	lease:=netip.MustParseAddr("10.66.0.2")
	physical:=netip.MustParseAddr("192.0.2.2")
	writer:=&fakeWriter{}
	for _,tc:=range []struct{name string;lease,physical netip.Addr;iface uint32;generation uint64;mtu int}{
		{"no iface",lease,physical,0,5,1249},
		{"no generation",lease,physical,12,0,1249},
		{"source tunnel leak",lease,lease,12,5,1249},
		{"MTU oversize",lease,physical,12,5,9001},
		{"MTU missing",lease,physical,12,5,0},
	}{
		t.Run(tc.name,func(t *testing.T){
			if _,err:=NewNativeUDPDirect(tc.lease,tc.physical,tc.iface,tc.generation,tc.mtu,writer);!errors.Is(err,ErrInvalidBinding) {
				t.Fatalf("unsafe source/interface accepted: %v",err)
			}
		})
	}
	backend,err:=NewNativeUDPDirect(lease,physical,12,5,1249,writer)
	if err!=nil {t.Fatal(err)}
	p,_,_:=n4NativeUDPTestRequest(t)
	if err:=backend.ForwardDirectIPv4(p,11,5);!errors.Is(err,ErrTUNFlowScope) {
		t.Fatalf("wrong physical interface accepted: %v",err)
	}
	if err:=backend.ForwardDirectIPv4(p,12,6);!errors.Is(err,ErrTUNFlowScope) {
		t.Fatalf("retired generation accepted: %v",err)
	}
	if err:=backend.Close();err!=nil {t.Fatal(err)}
	if err:=backend.Close();err!=nil {t.Fatal(err)}
	if err:=backend.ForwardDirectIPv4(p,12,5);!errors.Is(err,ErrTUNDirectBackendUnavailable) {
		t.Fatalf("closed backend accepted packet: %v",err)
	}
	if backend.FlowCount()!=0 {t.Fatalf("owned flow count after Close=%d",backend.FlowCount())}
}

type n4NativeSocketCapture struct{ packets chan []byte }

func (w *n4NativeSocketCapture) WritePacket(p []byte) (int,error) {
	select {
	case w.packets <- append([]byte(nil),p...):
		return len(p),nil
	default:
		return 0,fmt.Errorf("native UDP return backpressure")
	}
}

// This uses a real Windows kernel UDP socket with a bound NIC interface/source,
// but the return PacketWriter is deliberately simulated: never call it Wintun
// or true physical internet qualification.
func TestN4NativeUDPWindowsBoundSourceSameHostEcho(t *testing.T) {
	ifaces,err:=net.Interfaces()
	if err!=nil {t.Fatal(err)}
	var source netip.Addr
	var ifaceIndex uint32
	for _,iface:=range ifaces {
		if iface.Index<=0 || iface.Flags&net.FlagUp==0 || iface.Flags&net.FlagLoopback!=0 {continue}
		addrs,e:=iface.Addrs()
		if e!=nil {continue}
		for _,a:=range addrs {
			ipnet,ok:=a.(*net.IPNet)
			if !ok {continue}
			v4:=ipnet.IP.To4()
			if v4==nil {continue}
			addr,ok:=netip.AddrFromSlice(v4)
			if ok && addr.IsGlobalUnicast() && !addr.IsLinkLocalUnicast() {
				source=addr.Unmap()
				ifaceIndex=uint32(iface.Index)
				break
			}
		}
		if source.IsValid() {break}
	}
	if !source.IsValid() {
		t.Skip("UNSUPPORTED: hosted Windows runner has no physical IPv4 NIC/source")
	}
	srv,err:=net.ListenUDP("udp4",&net.UDPAddr{IP:net.IP(source.AsSlice())})
	if err!=nil {t.Fatalf("native local physical source listener: %v",err)}
	defer srv.Close()
	serverPort:=uint16(srv.LocalAddr().(*net.UDPAddr).Port)
	lease:=netip.MustParseAddr("10.66.0.2")
	if source==lease {t.Skip("UNSUPPORTED: same host test NIC overlaps synthetic lease")}
	writer:=&n4NativeSocketCapture{packets:make(chan []byte,2)}
	backend,err:=NewNativeUDPDirect(lease,source,ifaceIndex,7,1249,writer)
	if err!=nil {t.Fatal(err)}
	defer backend.Close()
	done:=make(chan error,1)
	go func(){
		b:=make([]byte,1024)
		_ = srv.SetReadDeadline(time.Now().Add(8*time.Second))
		n,peer,e:=srv.ReadFromUDPAddrPort(b)
		if e==nil && !bytes.Equal(b[:n],[]byte("native-direct-ping")) {
			e=fmt.Errorf("native responder payload mismatch")
		}
		if e==nil {_,e=srv.WriteToUDPAddrPort([]byte("native-direct-pong"),peer)}
		done<-e
	}()
	packet,err:=makeDirectUDPResponse(lease,source,41001,serverPort,[]byte("native-direct-ping"),11,1249)
	if err!=nil {t.Fatal(err)}
	if err:=backend.ForwardDirectIPv4(packet,ifaceIndex,7);err!=nil {
		t.Fatalf("mandatory Windows physical NIC binding/send failed iface=%d: %v",ifaceIndex,err)
	}
	select {
	case e:=<-done:
		if e!=nil {t.Fatal(e)}
	case <-time.After(10*time.Second):
		t.Fatal("native physical source UDP responder timed out")
	}
	select {
	case reply:=<-writer.packets:
		if len(reply)!=28+len("native-direct-pong") ||
			packetChecksum(reply[:20])!=0 ||
			udpChecksum(source,lease,reply[20:])!=0 ||
			binary.BigEndian.Uint16(reply[20:22])!=serverPort ||
			binary.BigEndian.Uint16(reply[22:24])!=41001 ||
			!bytes.Equal(reply[28:],[]byte("native-direct-pong")) {
			t.Fatalf("native reverse IPv4/UDP re-injection format corrupted")
		}
	case <-time.After(10*time.Second):
		t.Fatal("native return UDP packet not delivered to sink")
	}
	if n:=backend.FlowCount();n!=1 {t.Fatalf("expected one real owned native UDP mapping, got %d",n)}
}
