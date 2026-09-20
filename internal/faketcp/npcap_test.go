package faketcp

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"
)

func testNpcapConfig() NpcapConfig {
	return NpcapConfig{
		Device:     "\\Device\\NPF_{11111111-2222-3333-4444-555555555555}",
		SourceMAC:  [6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55},
		NextHopMAC: [6]byte{0x02, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		Flow: NpcapFlow{
			LocalIP: [4]byte{192, 0, 2, 20},
			PeerIP: [4]byte{198, 51, 100, 10},
			LocalPort: 41001,
			PeerPort: 443,
		},
		Persona: PacketPersonaWindows11,
		Generation: 9,
	}
}

func TestNpcapConfigAndBPFBindExactInboundFlow(t *testing.T) {
	cfg := testNpcapConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	filter, err := cfg.Flow.BPFFilter()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"tcp",
		"src host 198.51.100.10",
		"dst host 192.0.2.20",
		"src port 443",
		"dst port 41001",
		"ip[6:2] & 0x3fff = 0",
	} {
		if !strings.Contains(filter, want) {
			t.Fatalf("filter %q missing %q", filter, want)
		}
	}

	bad := cfg
	bad.Generation = 0
	if !errors.Is(bad.Validate(), ErrNpcapConfig) {
		t.Fatal("zero generation must fail closed")
	}
	bad = cfg
	bad.Device = "eth0"
	if !errors.Is(bad.Validate(), ErrNpcapConfig) {
		t.Fatal("non-NPF device must fail closed")
	}
}

func makeNpcapFrame(packet []byte, vlan bool) []byte {
	if !vlan {
		frame := make([]byte, 14+len(packet))
		binary.BigEndian.PutUint16(frame[12:14], 0x0800)
		copy(frame[14:], packet)
		return frame
	}
	frame := make([]byte, 18+len(packet))
	binary.BigEndian.PutUint16(frame[12:14], 0x8100)
	binary.BigEndian.PutUint16(frame[16:18], 0x0800)
	copy(frame[18:], packet)
	return frame
}

func TestNpcapIngressOwnsOnlyExactPeerFlowAndCopiesLifetime(t *testing.T) {
	cfg := testNpcapConfig()
	seg := Segment{
		SrcIP: cfg.Flow.PeerIP,
		DstIP: cfg.Flow.LocalIP,
		SrcPort: cfg.Flow.PeerPort,
		DstPort: cfg.Flow.LocalPort,
		Seq: 10,
		Ack: 20,
		Flags: FlagACK,
		Window: 32000,
		Payload: []byte("peer-data"),
	}
	packet := MarshalSegment(seg, 7, PacketPersonaLegacy)
	frame := makeNpcapFrame(packet, true)
	got, owned, ok, err := decodeNpcapInbound(frame, cfg.Flow)
	if err != nil || !ok {
		t.Fatalf("decode exact flow ok=%v err=%v", ok, err)
	}
	if string(got.Payload) != "peer-data" {
		t.Fatalf("payload=%q", got.Payload)
	}
	before := append([]byte(nil), owned...)
	for i := range frame {
		frame[i] ^= 0xff
	}
	if string(owned) != string(before) {
		t.Fatal("returned packet aliases Npcap capture lifetime")
	}

	wrongPort := seg
	wrongPort.DstPort++
	if _, _, ok, err := decodeNpcapInbound(
		makeNpcapFrame(MarshalSegment(wrongPort, 8, PacketPersonaLegacy), false),
		cfg.Flow,
	); err != nil || ok {
		t.Fatalf("wrong destination port reached parser ok=%v err=%v", ok, err)
	}

	outboundEcho := seg
	outboundEcho.SrcIP = cfg.Flow.LocalIP
	outboundEcho.DstIP = cfg.Flow.PeerIP
	outboundEcho.SrcPort = cfg.Flow.LocalPort
	outboundEcho.DstPort = cfg.Flow.PeerPort
	if _, _, ok, err := decodeNpcapInbound(
		makeNpcapFrame(MarshalSegment(outboundEcho, 9, PacketPersonaLegacy), false),
		cfg.Flow,
	); err != nil || ok {
		t.Fatalf("outbound self-capture reached parser ok=%v err=%v", ok, err)
	}

	fragment := append([]byte(nil), packet...)
	binary.BigEndian.PutUint16(fragment[6:8], 0x2000)
	if _, _, ok, err := decodeNpcapInbound(
		makeNpcapFrame(fragment, false),
		cfg.Flow,
	); err != nil || ok {
		t.Fatalf("fragment reached parser ok=%v err=%v", ok, err)
	}
}

func TestNpcapOutboundBindsSourcePortPeerAndEthernetPath(t *testing.T) {
	cfg := testNpcapConfig()
	seg := Segment{
		SrcIP: cfg.Flow.LocalIP,
		DstIP: cfg.Flow.PeerIP,
		SrcPort: cfg.Flow.LocalPort,
		DstPort: cfg.Flow.PeerPort,
		Seq: 100,
		Ack: 200,
		Flags: FlagACK | FlagPSH,
		Window: 64000,
		Payload: []byte("wire"),
	}
	packet, frame, err := encodeNpcapOutbound(seg, cfg, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != 14+len(packet) {
		t.Fatalf("frame=%d packet=%d", len(frame), len(packet))
	}
	var dstMAC, srcMAC [6]byte
	copy(dstMAC[:], frame[0:6])
	copy(srcMAC[:], frame[6:12])
	if dstMAC != cfg.NextHopMAC {
		t.Fatalf("destination mac=%x", dstMAC)
	}
	if srcMAC != cfg.SourceMAC {
		t.Fatalf("source mac=%x", srcMAC)
	}
	parsed, err := ParseIPv4TCP(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Flow.matchesSegment(parsed, false) {
		t.Fatalf("serialized segment escaped bound flow: %+v", parsed)
	}

	bad := seg
	bad.SrcPort++
	if _, _, err := encodeNpcapOutbound(bad, cfg, 13); !errors.Is(err, ErrNpcapOutboundFlow) {
		t.Fatalf("wrong source port err=%v", err)
	}
	bad = seg
	bad.DstIP[3]++
	if _, _, err := encodeNpcapOutbound(bad, cfg, 14); !errors.Is(err, ErrNpcapOutboundFlow) {
		t.Fatalf("wrong peer err=%v", err)
	}
}

func TestNpcapGenerationGateFencesStaleAndCloseWaitsForActiveIO(t *testing.T) {
	gate := newNpcapCallGate(17)
	if err := gate.begin(16); !errors.Is(err, ErrNpcapStaleGeneration) {
		t.Fatalf("stale begin err=%v", err)
	}
	if err := gate.begin(17); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		if !gate.close() {
			t.Error("first close did not own transition")
		}
		gate.wait()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("close returned while an I/O call was still active")
	case <-time.After(20 * time.Millisecond):
	}
	gate.end()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("close did not unblock after active I/O returned")
	}
	if err := gate.begin(17); !errors.Is(err, ErrNpcapClosed) {
		t.Fatalf("post-close begin err=%v", err)
	}
}
