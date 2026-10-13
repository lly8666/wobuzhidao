package windowsclient

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var (
	ErrN4UDPInvalid = errors.New("windowsclient: invalid native direct UDP IPv4 packet")
	ErrN4UDPUnsupported = errors.New("windowsclient: native direct UDP cannot safely forward this packet")
)

type directUDPDatagram struct {
	remote netip.AddrPort
	sourcePort uint16
	payload []byte
	dontFragment bool
}

// parseDirectUDPIPv4 fails closed on source spoofing, fragmentation, malformed
// transport lengths and checksum corruption. A TUN IPv4 packet is not a UDP
// payload; never feed it whole into net.UDPConn or a physical NIC.
func parseDirectUDPIPv4(packet []byte, lease netip.Addr, maxIP int) (directUDPDatagram, error) {
	if !lease.Is4() || len(packet) < 28 || len(packet) > maxIP ||
		len(packet) > logicaltunnel.MaxLeasedIPv4PacketLen || packet[0]>>4 != 4 {
		return directUDPDatagram{}, ErrN4UDPInvalid
	}
	ihl := int(packet[0]&15)*4
	if ihl < 20 || len(packet) < ihl+8 || int(binary.BigEndian.Uint16(packet[2:4])) != len(packet) ||
		packet[9] != 17 || packetChecksum(packet[:ihl]) != 0 {
		return directUDPDatagram{}, ErrN4UDPInvalid
	}
	if binary.BigEndian.Uint16(packet[6:8])&0x3fff != 0 {
		// Multi-fragment UDP needs a bounded, independently qualified
		// reassembler; the classifier association alone is not sufficient.
		return directUDPDatagram{}, ErrN4UDPUnsupported
	}
	src := netip.AddrFrom4([4]byte{packet[12],packet[13],packet[14],packet[15]})
	dst := netip.AddrFrom4([4]byte{packet[16],packet[17],packet[18],packet[19]})
	if src != lease || !dst.IsGlobalUnicast() || dst == lease {
		return directUDPDatagram{}, ErrN4UDPInvalid
	}
	udp := packet[ihl:]
	ulen := int(binary.BigEndian.Uint16(udp[4:6]))
	sport,dport := binary.BigEndian.Uint16(udp[0:2]),binary.BigEndian.Uint16(udp[2:4])
	if ulen != len(udp) || ulen < 8 || sport == 0 || dport == 0 {
		return directUDPDatagram{}, ErrN4UDPInvalid
	}
	if binary.BigEndian.Uint16(udp[6:8]) != 0 && udpChecksum(src,dst,udp) != 0 {
		return directUDPDatagram{}, ErrN4UDPInvalid
	}
	return directUDPDatagram{remote:netip.AddrPortFrom(dst,dport),sourcePort:sport,
		payload:udp[8:],dontFragment:binary.BigEndian.Uint16(packet[6:8])&0x4000 != 0},nil
}

// makeDirectUDPResponse reconstructs the original remote IPv4/UDP 5-tuple
// for return delivery into the leased Wintun. UDP checksums are mandatory on
// generated packets. It never silently truncates or fragments beyond MTU.
func makeDirectUDPResponse(from,to netip.Addr,srcPort,dstPort uint16,payload []byte,id uint16,maxIP int) ([]byte,error) {
	if !from.Is4() || !to.Is4() || srcPort==0 || dstPort==0 ||
		maxIP < 28 || maxIP > logicaltunnel.MaxLeasedIPv4PacketLen ||
		len(payload) > maxIP-28 || len(payload)>65535-28 {
		return nil,fmt.Errorf("%w: response exceeds direct TUN MTU or has invalid tuple",ErrN4UDPUnsupported)
	}
	p := make([]byte,28+len(payload))
	p[0]=0x45
	binary.BigEndian.PutUint16(p[2:4],uint16(len(p)))
	binary.BigEndian.PutUint16(p[4:6],id)
	p[8],p[9]=64,17
	a,b:=from.As4(),to.As4()
	copy(p[12:16],a[:]);copy(p[16:20],b[:])
	binary.BigEndian.PutUint16(p[20:22],srcPort)
	binary.BigEndian.PutUint16(p[22:24],dstPort)
	binary.BigEndian.PutUint16(p[24:26],uint16(len(payload)+8))
	copy(p[28:],payload)
	cs:=udpChecksum(from,to,p[20:])
	if cs==0 {cs=0xffff}
	binary.BigEndian.PutUint16(p[26:28],cs)
	binary.BigEndian.PutUint16(p[10:12],packetChecksum(p[:20]))
	return p,nil
}

func packetChecksum(p []byte) uint16 {
	var sum uint32
	for i:=0;i+1<len(p);i+=2 {sum+=uint32(binary.BigEndian.Uint16(p[i:i+2]))}
	if len(p)&1!=0 {sum+=uint32(p[len(p)-1])<<8}
	for sum>>16!=0 {sum=(sum&0xffff)+(sum>>16)}
	return ^uint16(sum)
}

func udpChecksum(src,dst netip.Addr,udp []byte) uint16 {
	a,b:=src.As4(),dst.As4()
	var sum uint32
	add:=func(buf []byte) {
		for i:=0;i+1<len(buf);i+=2 {sum+=uint32(binary.BigEndian.Uint16(buf[i:i+2]))}
		if len(buf)&1!=0 {sum+=uint32(buf[len(buf)-1])<<8}
	}
	add(a[:]);add(b[:])
	sum+=17+uint32(len(udp))
	add(udp)
	for sum>>16!=0 {sum=(sum&0xffff)+(sum>>16)}
	return ^uint16(sum)
}
