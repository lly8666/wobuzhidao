//go:build linux

package faketcp

import (
	"bytes"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func rawBatchPair(t *testing.T, size int) (*RawIPv4Endpoint, int) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { unix.Close(fds[0]); unix.Close(fds[1]) })
	e := &RawIPv4Endpoint{recvFD: fds[0], recvBuf: make([]byte, size), localIP: [4]byte{198, 51, 100, 20}}
	e.SetIODiagnostics(true)
	return e, fds[1]
}

func rawBatchPacket(payload []byte, id uint16) []byte {
	return MarshalSegment(Segment{SrcIP: [4]byte{198, 51, 100, 10}, DstIP: [4]byte{198, 51, 100, 20},
		SrcPort: 40000, DstPort: 443, Seq: uint32(id)*1000, Flags: FlagACK | FlagPSH, Payload: payload}, id, PacketPersonaLegacy)
}

func TestRawReceiveBatchPreservesOrderAndOwnedBuffers(t *testing.T) {
	e, sender := rawBatchPair(t, 65536+64)
	for i := 1; i <= 3; i++ {
		if err := unix.Sendto(sender, rawBatchPacket([]byte{byte(i)}, uint16(i)), 0, nil); err != nil { t.Fatal(err) }
	}
	var held [][]byte
	for i := 1; i <= 3; i++ {
		seg, packet, err := e.ReadSegment()
		if err != nil || !bytes.Equal(seg.Payload, []byte{byte(i)}) { t.Fatalf("i=%d seg=%+v err=%v", i, seg, err) }
		held = append(held, packet)
	}
	stats := e.IODiagnostic()
	if stats.ReceiveCalls != 1 || stats.ReceiveMessages != 3 || stats.ReceiveMulti != 1 {
		t.Fatalf("batch syscall not exercised: %+v", stats)
	}
	if err := unix.Sendto(sender, rawBatchPacket([]byte("reuse"), 4), 0, nil); err != nil { t.Fatal(err) }
	if _, _, err := e.ReadSegment(); err != nil { t.Fatal(err) }
	for i, wire := range held {
		seg, err := ParseIPv4TCP(wire)
		if err != nil || !bytes.Equal(seg.Payload, []byte{byte(i+1)}) { t.Fatal("refill overwrote retained packet") }
	}
}

func TestRawReceiveBatchSparseDoesNotWaitForEight(t *testing.T) {
	e, sender := rawBatchPair(t, 65536+64)
	if err := unix.Sendto(sender, rawBatchPacket([]byte("one"), 1), 0, nil); err != nil { t.Fatal(err) }
	done := make(chan error, 1)
	go func() { _, _, err := e.ReadSegment(); done <- err }()
	select {
	case err := <-done:
		if err != nil { t.Fatal(err) }
	case <-time.After(time.Second):
		t.Fatal("single packet blocked waiting for batch fill")
	}
}

func TestRawReceiveBatchRejectsTruncationAndLegacyFallbackWorks(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		e, sender := rawBatchPair(t, 64)
		e.recvBatchDisabled = fallback
		for _, payload := range [][]byte{bytes.Repeat([]byte{0xa5}, 400), []byte("valid")} {
			if err := unix.Sendto(sender, rawBatchPacket(payload, 1), 0, nil); err != nil { t.Fatal(err) }
		}
		seg, _, err := e.ReadSegment()
		if err != nil || string(seg.Payload) != "valid" { t.Fatalf("fallback=%v seg=%+v err=%v", fallback, seg, err) }
	}
}
