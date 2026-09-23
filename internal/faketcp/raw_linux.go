//go:build linux

package faketcp

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"syscall"
)

const (
	rawEthPIPv4       = 0x0800
	rawPacketOutgoing = 4
)

// RawIPv4Endpoint is the minimal Linux raw-packet adapter required for hosted
// P2 qualification. It is deliberately narrower than the later production
// platform layer: one interface, one local IPv4 address, TCP-shaped packets,
// and the existing Segment parser/marshaler. It does not create TUN devices,
// routing policy, DTLS, or a kernel TCP business channel.
type RawIPv4Endpoint struct {
	recvFD  int
	sendFD  int
	localIP [4]byte
	persona PacketPersona

	recvMu  sync.Mutex
	recvBuf []byte

	mu     sync.Mutex
	ipID   uint16
	closed bool
	once   sync.Once
}

func OpenRawIPv4Endpoint(interfaceName string, localIP [4]byte, persona PacketPersona) (*RawIPv4Endpoint, error) {
	if interfaceName == "" || localIP == ([4]byte{}) {
		return nil, errors.New("faketcp: invalid raw IPv4 endpoint config")
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, err
	}

	recvFD, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(rawHTONS(rawEthPIPv4)))
	if err != nil {
		return nil, err
	}
	closeRecv := true
	defer func() {
		if closeRecv {
			_ = syscall.Close(recvFD)
		}
	}()
	if err := syscall.SetsockoptTimeval(recvFD, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Usec: 200000}); err != nil {
		return nil, err
	}
	if err := syscall.Bind(recvFD, &syscall.SockaddrLinklayer{
		Protocol: rawHTONS(rawEthPIPv4),
		Ifindex:  iface.Index,
	}); err != nil {
		return nil, err
	}

	sendFD, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return nil, err
	}
	closeSend := true
	defer func() {
		if closeSend {
			_ = syscall.Close(sendFD)
		}
	}()
	if err := syscall.SetsockoptInt(sendFD, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		return nil, err
	}

	closeRecv = false
	closeSend = false
	return &RawIPv4Endpoint{
		recvFD: recvFD, sendFD: sendFD,
		localIP: localIP, persona: persona,
		recvBuf: make([]byte, 65536+64),
		ipID:    1,
	}, nil
}

// ReadSegment returns the next inbound IPv4/TCP segment addressed to localIP.
// AF_PACKET exposes both loopback directions; PACKET_OUTGOING is ignored so a
// local hosted test does not process the same kernel packet twice.
func (e *RawIPv4Endpoint) ReadSegment() (Segment, []byte, error) {
	if e == nil {
		return Segment{}, nil, errors.New("faketcp: nil raw IPv4 endpoint")
	}
	e.recvMu.Lock()
	defer e.recvMu.Unlock()
	for {
		n, from, err := syscall.Recvfrom(e.recvFD, e.recvBuf, 0)
		if err != nil {
			// recvfrom(2) may be interrupted by a signal before any packet is
			// consumed. EINTR is not an endpoint failure; retry the same blocking
			// receive and preserve all other error handling unchanged.
			if rawIOInterrupted(err) {
				continue
			}
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				e.mu.Lock()
				closed := e.closed
				e.mu.Unlock()
				if closed {
					return Segment{}, nil, net.ErrClosed
				}
				continue
			}
			return Segment{}, nil, err
		}
		if ll, ok := from.(*syscall.SockaddrLinklayer); ok && ll.Pkttype == rawPacketOutgoing {
			continue
		}
		ip := rawExtractIPv4(e.recvBuf[:n])
		if len(ip) == 0 {
			continue
		}
		seg, err := ParseIPv4TCP(ip)
		if err != nil {
			continue
		}
		if seg.DstIP != e.localIP {
			continue
		}
		return rawOwnedIPv4TCP(ip)
	}
}

// rawOwnedIPv4TCP transfers a borrowed packet view into exact-sized owned
// storage. The returned Segment.Payload aliases owned, never the reusable
// AF_PACKET receive scratch.
func rawOwnedIPv4TCP(ip []byte) (Segment, []byte, error) {
	owned := append([]byte(nil), ip...)
	seg, err := ParseIPv4TCP(owned)
	if err != nil {
		return Segment{}, nil, err
	}
	return seg, owned, nil
}

// WriteSegment serializes a Segment through the production checksum/options
// path and injects the complete IPv4 packet. The returned bytes are an owned
// copy of exactly what was submitted to the kernel raw socket.
func (e *RawIPv4Endpoint) WriteSegment(seg Segment) ([]byte, error) {
	if e == nil {
		return nil, errors.New("faketcp: nil raw IPv4 endpoint")
	}
	e.mu.Lock()
	id := e.ipID
	e.ipID++
	pkt := MarshalSegment(seg, id, e.persona)
	var err error
	for {
		err = syscall.Sendto(e.sendFD, pkt, 0, &syscall.SockaddrInet4{
			Port: int(seg.DstPort),
			Addr: seg.DstIP,
		})
		if rawIOInterrupted(err) {
			continue
		}
		break
	}
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// MarshalSegment already returns a fresh owned packet. Sendto has completed
	// before this point, so returning that packet preserves the ownership contract
	// without cloning every emitted data/ACK segment a second time.
	return pkt, nil
}

func (e *RawIPv4Endpoint) Close() error {
	if e == nil {
		return nil
	}
	var first error
	e.once.Do(func() {
		e.mu.Lock()
		e.closed = true
		e.mu.Unlock()
		if err := syscall.Close(e.recvFD); err != nil {
			first = err
		}
		if err := syscall.Close(e.sendFD); err != nil && first == nil {
			first = err
		}
	})
	return first
}

func rawIOInterrupted(err error) bool {
	return errors.Is(err, syscall.EINTR)
}

func rawHTONS(v uint16) uint16 {
	return v<<8 | v>>8
}

func rawExtractIPv4(frame []byte) []byte {
	// AF_PACKET/SOCK_RAW on Linux loopback presents an Ethernet-style header,
	// while alternate capture devices may expose raw/SLL-shaped prefixes.
	for _, off := range []int{14, 16, 20, 0, 4} {
		if off < 0 || len(frame) < off+20 {
			continue
		}
		if frame[off]>>4 != 4 || frame[off+9] != 6 {
			continue
		}
		ihl := int(frame[off]&0x0f) * 4
		if ihl < 20 || len(frame) < off+ihl {
			continue
		}
		total := int(binary.BigEndian.Uint16(frame[off+2 : off+4]))
		if total < ihl+20 || off+total > len(frame) {
			continue
		}
		return frame[off : off+total]
	}
	return nil
}
