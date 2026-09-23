//go:build linux

package faketcp

import (
	"bytes"
	"os"
	"syscall"
	"testing"
)

func TestRawIOInterruptedOnlyRetriesEINTR(t *testing.T) {
	if !rawIOInterrupted(syscall.EINTR) {
		t.Fatal("plain EINTR must be retryable")
	}
	if !rawIOInterrupted(&os.SyscallError{Syscall: "recvfrom", Err: syscall.EINTR}) {
		t.Fatal("wrapped EINTR must be retryable")
	}
	for _, err := range []error{syscall.EAGAIN, syscall.EWOULDBLOCK, syscall.EBADF, syscall.ENETDOWN} {
		if rawIOInterrupted(err) {
			t.Fatalf("unexpected retryable raw I/O error: %v", err)
		}
	}
}


func TestRawOwnedIPv4TCPPreservesPayloadAfterScratchReuse(t *testing.T) {
	originalPayload := []byte("owned-payload")
	packet := MarshalSegment(Segment{
		SrcIP:   [4]byte{198, 51, 100, 10},
		DstIP:   [4]byte{198, 51, 100, 20},
		SrcPort: 40000,
		DstPort: 443,
		Seq:     1000,
		Ack:     2000,
		Flags:   FlagACK | FlagPSH,
		Window:  65535,
		Payload: originalPayload,
	}, 1, PacketPersonaLegacy)

	seg, owned, err := rawOwnedIPv4TCP(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(seg.Payload, originalPayload) {
		t.Fatalf("payload=%q want=%q", seg.Payload, originalPayload)
	}
	if len(owned) != len(packet) {
		t.Fatalf("owned packet len=%d want=%d", len(owned), len(packet))
	}

	for i := range packet {
		packet[i] ^= 0xff
	}
	if !bytes.Equal(seg.Payload, originalPayload) {
		t.Fatalf("payload changed after source scratch mutation: %q", seg.Payload)
	}
}
