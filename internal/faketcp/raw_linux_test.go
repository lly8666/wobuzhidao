//go:build linux

package faketcp

import (
	"bytes"
	"os"
	"reflect"
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


// E4 hot-path CPU candidate: after the first validated parse, ownership copy
// must yield the exact same Segment as parsing the owned buffer again.
// This includes TCP options, zero payload, and Ethernet/IP tail padding,
// whose bytes must NOT become Segment.Payload.
func TestRawOwnedParsedIPv4TCPOneParseMatchesOwnedReparse(t *testing.T) {
	cases := []struct {
		name    string
		segment Segment
		padding int
	}{
		{name: "plain-payload", segment: Segment{
			SrcIP: [4]byte{198, 51, 100, 1}, DstIP: [4]byte{192, 0, 2, 2},
			SrcPort: 40001, DstPort: 443, Seq: 100, Ack: 200,
			Flags: FlagACK | FlagPSH, Window: 65535, Payload: []byte("immediate-delivery"),
		}},
		{name: "ethernet-padding-excluded", padding: 21, segment: Segment{
			SrcIP: [4]byte{198, 51, 100, 3}, DstIP: [4]byte{192, 0, 2, 4},
			SrcPort: 40002, DstPort: 8443, Seq: 300, Ack: 400,
			Flags: FlagACK | FlagPSH, Window: 65000, Payload: []byte("body"),
		}},
		{name: "syn-options-zero-payload", padding: 12, segment: Segment{
			SrcIP: [4]byte{203, 0, 113, 5}, DstIP: [4]byte{192, 0, 2, 6},
			SrcPort: 40003, DstPort: 443, Seq: 500, Window: 64000,
			Flags: FlagSYN, MSSSet: true, MSS: 1360,
			SACKPermitted: true, WindowScaleSet: true, WindowScale: 8,
		}},
		{name: "sack-options-payload", segment: Segment{
			SrcIP: [4]byte{203, 0, 113, 7}, DstIP: [4]byte{192, 0, 2, 8},
			SrcPort: 40004, DstPort: 443, Seq: 700, Ack: 800,
			Flags: FlagACK, SACKN: 2,
			SACK: [MaxSACKBlocks]SACKBlock{{Start: 1, End: 3}, {Start: 8, End: 10}},
			Window: 63000, Payload: []byte("sack-body"),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packet := MarshalSegment(tc.segment, 9, PacketPersonaLegacy)
			if tc.padding > 0 {
				packet = append(packet, bytes.Repeat([]byte{0xa5}, tc.padding)...)
			}
			borrowed, err := ParseIPv4TCP(packet)
			if err != nil {
				t.Fatal(err)
			}
			got, owned, err := rawOwnedParsedIPv4TCP(packet, borrowed)
			if err != nil {
				t.Fatal(err)
			}
			want, oldOwned, err := rawOwnedIPv4TCP(packet)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || !bytes.Equal(owned, oldOwned) {
				t.Fatalf("single-parse ownership changed metadata/wire: got=%+v want=%+v", got, want)
			}
			if !bytes.Equal(got.Payload, tc.segment.Payload) {
				t.Fatalf("padding/options leaked into payload: got=%x want=%x", got.Payload, tc.segment.Payload)
			}
			for i := range packet {
				packet[i] ^= 0xff
			}
			if !reflect.DeepEqual(got, want) || !bytes.Equal(owned, oldOwned) {
				t.Fatal("returned Segment/wire aliases reused AF_PACKET receive scratch")
			}
		})
	}
}
