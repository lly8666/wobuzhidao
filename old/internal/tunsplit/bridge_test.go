package tunsplit

import (
	"net/netip"
	"testing"
)

func testIPv4Packet(dst string, protocol byte) []byte {
	packet := make([]byte, 20)
	packet[0] = 0x45
	packet[9] = protocol
	addr := netip.MustParseAddr(dst).As4()
	copy(packet[16:20], addr[:])
	return packet
}

func TestBridgeDirectsOnlyTCPAndUDPForDirectDestinations(t *testing.T) {
	cn4 := writeTestCN4(t, "1.2.0.0/16\n")
	c, err := NewClassifier(Policy{ProxyLAN: false, ProxyChina: false, ProxyOther: false}, cn4)
	if err != nil {
		t.Fatal(err)
	}
	b := &Bridge{Classifier: c}

	for _, protocol := range []byte{6, 17} {
		if !b.shouldDirect(testIPv4Packet("8.8.8.8", protocol)) {
			t.Fatalf("protocol=%d direct destination did not enter userspace direct path", protocol)
		}
	}
	for _, protocol := range []byte{1, 2, 47, 50} {
		if b.shouldDirect(testIPv4Packet("8.8.8.8", protocol)) {
			t.Fatalf("protocol=%d bypassed fail-closed proxy path", protocol)
		}
	}
	if b.shouldDirect([]byte{0x45}) {
		t.Fatal("malformed packet bypassed fail-closed proxy path")
	}
}

func TestBridgeHonorsProxyDecisionBeforeTransportProtocol(t *testing.T) {
	cn4 := writeTestCN4(t, "1.2.0.0/16\n")
	c, err := NewClassifier(Policy{ProxyOther: true}, cn4)
	if err != nil {
		t.Fatal(err)
	}
	b := &Bridge{Classifier: c}
	for _, protocol := range []byte{6, 17} {
		if b.shouldDirect(testIPv4Packet("8.8.8.8", protocol)) {
			t.Fatalf("protocol=%d ignored proxy_other policy", protocol)
		}
	}
}
