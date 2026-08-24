package benchmark

import (
	"context"
	"testing"
	"time"
)

func TestKernelSocketFaultSmokeShowsTCPHOLAndUDPBypass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	delays := []time.Duration{1 * time.Millisecond, 35 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond}
	tcp, err := RunSocketFaultSmoke(ctx, SocketTCP, delays, 256)
	if err != nil {
		t.Fatal(err)
	}
	udp, err := RunSocketFaultSmoke(ctx, SocketUDP, delays, 256)
	if err != nil {
		t.Fatal(err)
	}
	if len(tcp.Arrival) != 4 || len(udp.Arrival) != 4 {
		t.Fatalf("bad observations tcp=%v udp=%v", tcp.Arrival, udp.Arrival)
	}
	if tcp.Arrival[2] < tcp.Arrival[1] {
		t.Fatalf("TCP bypassed serial stall: %v", tcp.Arrival)
	}
	if !(udp.Arrival[2]+10*time.Millisecond < udp.Arrival[1]) {
		t.Fatalf("UDP did not bypass stall: %v", udp.Arrival)
	}
	t.Logf("tcp arrivals=%v", tcp.Arrival)
	t.Logf("udp arrivals=%v", udp.Arrival)
}
