//go:build windows

package windowsclient

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// NativeUDPDirect implements actual UDP socket forwarding, not raw-IP egress.
// Windows IP_UNICAST_IF and a physical IPv4 source bind are mandatory; failure
// never falls back to an unbound socket or to the protected tunnel.
type NativeUDPDirect struct {
	mu sync.Mutex
	lease,source netip.Addr
	iface uint32
	generation uint64
	maxIP int
	writer PacketWriter
	flows map[nativeUDPKey]*nativeUDPFlow
	id atomic.Uint32
	closed bool
}

type nativeUDPKey struct {
	remote netip.AddrPort
	clientPort uint16
	df bool
}
type nativeUDPFlow struct {
	conn *net.UDPConn
	lastUse time.Time
}

// NewNativeUDPDirect only creates owned state, not a route or background task.
// The caller must use the same physical interface/source as the tunnel's
// underlay and close this object before retiring its generation.
func NewNativeUDPDirect(lease4,physicalSource4 netip.Addr,interfaceIndex uint32,generation uint64,
	maxIPv4 int,writer PacketWriter) (*NativeUDPDirect,error) {
	if !lease4.Is4() || !physicalSource4.Is4() || lease4==physicalSource4 ||
		!physicalSource4.IsGlobalUnicast() || interfaceIndex==0 || generation==0 ||
		maxIPv4<28 || maxIPv4>9000 || writer==nil {
		return nil,ErrInvalidBinding
	}
	return &NativeUDPDirect{lease:lease4,source:physicalSource4,iface:interfaceIndex,
		generation:generation,maxIP:maxIPv4,writer:writer,
		flows:make(map[nativeUDPKey]*nativeUDPFlow)},nil
}

func (b *NativeUDPDirect) ForwardDirectIPv4(packet []byte,interfaceIndex uint32,generation uint64) error {
	if b==nil || interfaceIndex==0 || generation==0 ||
		interfaceIndex!=b.iface || generation!=b.generation {
		return ErrTUNFlowScope
	}
	d,err:=parseDirectUDPIPv4(packet,b.lease,b.maxIP)
	if err!=nil {return err}
	key:=nativeUDPKey{remote:d.remote,clientPort:d.sourcePort,df:d.dontFragment}
	b.mu.Lock()
	if b.closed {b.mu.Unlock();return ErrTUNDirectBackendUnavailable}
	flow:=b.flows[key]
	if flow==nil {
		// Creation is serialized, so the cap can never be exceeded even under
		// many simultaneous first packets. Only UDP state is locked here.
		conn,openErr:=openNativeUDPBound(b.source,b.iface,key.df)
		if openErr!=nil {b.mu.Unlock();return fmt.Errorf("%w: UDP physical bind: %v",ErrTUNDirectBackendUnavailable,openErr)}
		if len(b.flows)>=N4MaxFlowAssociations {
			var victim nativeUDPKey
			var earliest time.Time
			first:=true
			for k,v:=range b.flows {
				if first||v.lastUse.Before(earliest) {first=false;victim=k;earliest=v.lastUse}
			}
			if !first {
				b.flows[victim].conn.Close()
				delete(b.flows,victim)
			}
		}
		flow=&nativeUDPFlow{conn:conn}
		b.flows[key]=flow
		go b.readResponses(key,flow)
	}
	flow.lastUse=time.Now()
	b.mu.Unlock()
	// The packet is validated and the borrowed TUN ring was already copied
	// by TUNPacketDispatcher. Write only the UDP payload on a bound socket.
	n,err:=flow.conn.WriteToUDPAddrPort(d.payload,key.remote)
	if err!=nil {return err}
	if n!=len(d.payload) {return fmt.Errorf("%w: short UDP direct write",ErrN4UDPUnsupported)}
	return nil
}

func (b *NativeUDPDirect) readResponses(key nativeUDPKey,flow *nativeUDPFlow) {
	// One extra byte detects an oversized datagram even if ReadFrom truncates it.
	buf:=make([]byte,b.maxIP-27)
	for {
		if err:=flow.conn.SetReadDeadline(time.Now().Add(2*time.Second));err!=nil {return}
		n,from,err:=flow.conn.ReadFromUDPAddrPort(buf)
		if err!=nil {
			var nerr net.Error
			if errors.As(err,&nerr)&&nerr.Timeout() {
				b.mu.Lock()
				existing:=b.flows[key]
				if existing!=flow || b.closed {b.mu.Unlock();return}
				if time.Since(flow.lastUse)>=N4FlowTTL {
					delete(b.flows,key)
					flow.conn.Close()
					b.mu.Unlock()
					return
				}
				b.mu.Unlock()
				continue
			}
			b.mu.Lock()
			if b.flows[key]==flow {delete(b.flows,key)}
			b.mu.Unlock()
			flow.conn.Close()
			return
		}
		if from!=key.remote || n>b.maxIP-28 {continue} // no off-path or truncated reply
		b.mu.Lock()
		allowed:=!b.closed&&b.flows[key]==flow
		if allowed {flow.lastUse=time.Now()}
		b.mu.Unlock()
		if !allowed {return}
		p,err:=makeDirectUDPResponse(from.Addr(),b.lease,from.Port(),key.clientPort,
			buf[:n],uint16(b.id.Add(1)),b.maxIP)
		if err!=nil {continue} // oversized response is not silently truncated
		// The callback is supplied by the owner/lease-bound Wintun Router.
		// A short write is a hard failure; never retry with an unbound path.
		written,err:=b.writer.WritePacket(p)
		if err!=nil || written!=len(p) {
			// A dead Wintun must not leave owned UDP sockets spinning.
			b.mu.Lock()
			if b.flows[key]==flow {delete(b.flows,key)}
			b.mu.Unlock()
			flow.conn.Close()
			return
		}
	}
}

// Close owns only these UDP sockets. It is idempotent and does not wait for
// an OS/driver WritePacket blocked elsewhere; the caller closes the Wintun
// after this to unblock outstanding writes.
func (b *NativeUDPDirect) Close() error {
	if b==nil {return nil}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {return nil}
	b.closed=true
	for k,v:=range b.flows {v.conn.Close();delete(b.flows,k)}
	return nil
}

func (b *NativeUDPDirect) FlowCount() int {
	if b==nil {return 0}
	b.mu.Lock();defer b.mu.Unlock()
	return len(b.flows)
}

func openNativeUDPBound(source netip.Addr,iface uint32,df bool) (*net.UDPConn,error) {
	// IPv4 IP_UNICAST_IF (31) requires network-byte-order interface index.
	// IP_DONTFRAGMENT (14) makes DF original-packet intent explicit on egress.
	const ipUnicastIf=31
	const ipDontFragment=14
	lc:=net.ListenConfig{Control:func(_, _ string,raw syscall.RawConn) error {
		var sockErr error
		err:=raw.Control(func(fd uintptr) {
			s:=windows.Handle(fd)
			if e:=windows.SetsockoptInt(s,0,ipUnicastIf,int(bits.ReverseBytes32(iface)));e!=nil {
				sockErr=e;return
			}
			val:=0
			if df {val=1}
			sockErr=windows.SetsockoptInt(s,0,ipDontFragment,val)
		})
		if err!=nil {return err}
		return sockErr
	}}
	local:=net.JoinHostPort(source.String(),"0")
	conn,err:=lc.ListenPacket(context.Background(),"udp4",local)
	if err!=nil {return nil,err}
	udp,ok:=conn.(*net.UDPConn)
	if !ok {conn.Close();return nil,ErrTUNDirectBackendUnavailable}
	// Refuse a surprise family/source fallback, even when the setsockopt calls
	// succeeded. Binding is not proof of physical Wintun exclusion.
	addr,ok:=udp.LocalAddr().(*net.UDPAddr)
	if !ok || !addr.IP.Equal(net.IP(source.AsSlice())) {
		conn.Close();return nil,ErrTUNDirectBackendUnavailable
	}
	return udp,nil
}

