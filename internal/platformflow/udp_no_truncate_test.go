package platformflow

import (
    "bytes"
    "net"
    "testing"
    "time"
)

// This is a *socket* boundary test, not a tunnel performance qualification.
// Legal IPv4 UDP > platformflow.MaxPayload must be rejected in one piece:
// never forward a shortened datagram or poison the next small datagram.
func TestUDPMappingReadRejectsOversizeWithoutTruncatingNext(t *testing.T) {
    server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
    if err != nil { t.Fatal(err) }
    defer server.Close()
    writer, err := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
    if err != nil { t.Fatal(err) }
    defer writer.Close()

    cases := []struct {
        payloadSize int
        rejected bool
    }{
        {96, false},
        {MaxPayload, false},
        {MaxPayload + 1, true},
        {8972, true}, // legal IPv4 UDP at 9000-byte inner TUN MTU
        {8973, true}, // legal IPv4 UDP, inner IP fragmentation needed
        {65507, true}, // maximum legal ordinary IPv4 UDP payload
        {256, false},
    }
    buffer := make([]byte, MaxPayload+1)
    for _, tc := range cases {
        data := bytes.Repeat([]byte{byte(tc.payloadSize % 251)}, tc.payloadSize)
        if _, err := writer.Write(data); err != nil { t.Fatalf("send len %d: %v", tc.payloadSize, err) }
        if err := server.SetReadDeadline(time.Now().Add(3*time.Second)); err != nil { t.Fatal(err) }
        n, peer, rejected, err := readBoundedUDPMapping(server, buffer)
        if err != nil { t.Fatalf("read len %d: %v", tc.payloadSize, err) }
        if !peer.IsValid() { t.Fatalf("invalid peer at len %d", tc.payloadSize) }
        if rejected != tc.rejected {
            t.Fatalf("len %d rejected=%v wanted=%v received=%d", tc.payloadSize, rejected, tc.rejected, n)
        }
        if !rejected {
            if n != tc.payloadSize || !bytes.Equal(buffer[:n], data) {
                t.Fatalf("len %d accepted data changed or truncated (actual %d)", tc.payloadSize, n)
            }
        } else if n <= MaxPayload {
            t.Fatalf("oversize datagram silently shortened into accepted range: n=%d", n)
        }
    }
}

func TestUDPMappingOversizeCounterIsInSnapshot(t *testing.T) {
    server := &UDPServer{sendQueue: newUDPServerSendQueue(1)}
    server.upstreamOversizeDrops.Add(3)
    got := server.Diagnostic()
    if got.UpstreamOversizeDrops != 3 {
        t.Fatalf("upstream oversized drops snapshot=%d want 3", got.UpstreamOversizeDrops)
    }
    if got.QueueCapacityRecords != 1 {
        t.Fatalf("counter changed queue capacity: %d", got.QueueCapacityRecords)
    }
}
