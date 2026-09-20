package faketcp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

var (
	ErrNpcapConfig          = errors.New("faketcp: invalid Npcap configuration")
	ErrNpcapClosed          = errors.New("faketcp: Npcap endpoint is closed")
	ErrNpcapStaleGeneration = errors.New("faketcp: stale Npcap lane generation")
	ErrNpcapUnsupported     = errors.New("faketcp: Npcap is unsupported on this platform")
	ErrNpcapOutboundFlow    = errors.New("faketcp: outbound segment does not match bound Npcap flow")
)

type NpcapFlow struct {
	LocalIP   [4]byte
	PeerIP    [4]byte
	LocalPort uint16
	PeerPort  uint16
}

func (f NpcapFlow) Validate() error {
	if f.LocalIP == ([4]byte{}) || f.PeerIP == ([4]byte{}) ||
		f.LocalPort == 0 || f.PeerPort == 0 {
		return ErrNpcapConfig
	}
	return nil
}

func (f NpcapFlow) BPFFilter() (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"tcp and src host %s and dst host %s and src port %d and dst port %d and (ip[6:2] & 0x3fff = 0)",
		net.IP(f.PeerIP[:]).String(),
		net.IP(f.LocalIP[:]).String(),
		f.PeerPort,
		f.LocalPort,
	), nil
}

type NpcapConfig struct {
	Device     string
	SourceMAC  [6]byte
	NextHopMAC [6]byte
	Flow       NpcapFlow
	Persona    PacketPersona
	Generation uint64
}

func (c NpcapConfig) Validate() error {
	if !canonicalNpcapDevice(c.Device) ||
		c.SourceMAC == ([6]byte{}) ||
		c.NextHopMAC == ([6]byte{}) ||
		c.Generation == 0 {
		return ErrNpcapConfig
	}
	return c.Flow.Validate()
}

func canonicalNpcapDevice(value string) bool {
	value = strings.TrimSpace(value)
	const prefix = "\\Device\\NPF_"
	if len(value) != len(prefix)+38 || !strings.EqualFold(value[:len(prefix)], prefix) {
		return false
	}
	guid := value[len(prefix):]
	return guid[0] == '{' && guid[len(guid)-1] == '}' &&
		guid[9] == '-' && guid[14] == '-' && guid[19] == '-' && guid[24] == '-'
}

func extractEthernetIPv4(frame []byte) ([]byte, bool) {
	if len(frame) < 14 {
		return nil, false
	}
	etherType := binary.BigEndian.Uint16(frame[12:14])
	offset := 14
	for tags := 0; (etherType == 0x8100 || etherType == 0x88a8) && tags < 2; tags++ {
		if len(frame) < offset+4 {
			return nil, false
		}
		etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
		offset += 4
	}
	if etherType == 0x8100 || etherType == 0x88a8 ||
		etherType != 0x0800 || len(frame) < offset+20 {
		return nil, false
	}
	packet := frame[offset:]
	if packet[0]>>4 != 4 {
		return nil, false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl {
		return nil, false
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total < ihl || total > len(packet) {
		return nil, false
	}
	return packet[:total], true
}

func (f NpcapFlow) matchesPacket(packet []byte, inbound bool) bool {
	if len(packet) < 40 || packet[0]>>4 != 4 || packet[9] != 6 {
		return false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+20 {
		return false
	}
	if binary.BigEndian.Uint16(packet[6:8])&0x3fff != 0 {
		return false
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total < ihl+20 || total > len(packet) {
		return false
	}
	var srcIP, dstIP [4]byte
	copy(srcIP[:], packet[12:16])
	copy(dstIP[:], packet[16:20])
	srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
	dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
	if inbound {
		return srcIP == f.PeerIP && dstIP == f.LocalIP &&
			srcPort == f.PeerPort && dstPort == f.LocalPort
	}
	return srcIP == f.LocalIP && dstIP == f.PeerIP &&
		srcPort == f.LocalPort && dstPort == f.PeerPort
}

func (f NpcapFlow) matchesSegment(seg Segment, inbound bool) bool {
	if inbound {
		return seg.SrcIP == f.PeerIP && seg.DstIP == f.LocalIP &&
			seg.SrcPort == f.PeerPort && seg.DstPort == f.LocalPort
	}
	return seg.SrcIP == f.LocalIP && seg.DstIP == f.PeerIP &&
		seg.SrcPort == f.LocalPort && seg.DstPort == f.PeerPort
}

func decodeNpcapInbound(frame []byte, flow NpcapFlow) (Segment, []byte, bool, error) {
	packet, ok := extractEthernetIPv4(frame)
	if !ok || !flow.matchesPacket(packet, true) {
		return Segment{}, nil, false, nil
	}
	owned := append([]byte(nil), packet...)
	seg, err := ParseIPv4TCP(owned)
	if err != nil {
		return Segment{}, nil, false, err
	}
	if !flow.matchesSegment(seg, true) {
		return Segment{}, nil, false, ErrNpcapConfig
	}
	return seg, owned, true, nil
}

func encodeNpcapOutbound(seg Segment, cfg NpcapConfig, ipID uint16) ([]byte, []byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if !cfg.Flow.matchesSegment(seg, false) {
		return nil, nil, ErrNpcapOutboundFlow
	}
	packet := MarshalSegment(seg, ipID, cfg.Persona)
	if !cfg.Flow.matchesPacket(packet, false) {
		return nil, nil, ErrNpcapOutboundFlow
	}
	frame := make([]byte, 14+len(packet))
	copy(frame[0:6], cfg.NextHopMAC[:])
	copy(frame[6:12], cfg.SourceMAC[:])
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	copy(frame[14:], packet)
	return append([]byte(nil), packet...), frame, nil
}

type npcapCallGate struct {
	mu         sync.Mutex
	generation uint64
	closed     bool
	active     sync.WaitGroup
}

func newNpcapCallGate(generation uint64) *npcapCallGate {
	return &npcapCallGate{generation: generation}
}

func (g *npcapCallGate) begin(generation uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrNpcapClosed
	}
	if generation == 0 || generation != g.generation {
		return ErrNpcapStaleGeneration
	}
	g.active.Add(1)
	return nil
}

func (g *npcapCallGate) end() {
	g.active.Done()
}

func (g *npcapCallGate) close() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.closed = true
	return true
}

func (g *npcapCallGate) isClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

func (g *npcapCallGate) wait() {
	g.active.Wait()
}
