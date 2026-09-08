package faketcp

import "testing"

func TestIsKeepaliveProbe(t *testing.T) {
	next := uint32(9000)
	base := Segment{Flags: FlagACK, Seq: next - 1, Ack: 7000}
	if !IsKeepaliveProbe(base, next) {
		t.Fatal("classic ACK-only RCV.NXT-1 segment was not recognized as keepalive")
	}

	cases := []struct {
		name string
		seg  Segment
	}{
		{"normal-ack", Segment{Flags: FlagACK, Seq: next, Ack: 7000}},
		{"payload", Segment{Flags: FlagACK, Seq: next - 1, Ack: 7000, Payload: []byte{1}}},
		{"fin", Segment{Flags: FlagFIN | FlagACK, Seq: next - 1, Ack: 7000}},
		{"rst", Segment{Flags: FlagRST | FlagACK, Seq: next - 1, Ack: 7000}},
		{"syn", Segment{Flags: FlagSYN | FlagACK, Seq: next - 1, Ack: 7000}},
		{"psh", Segment{Flags: FlagPSH | FlagACK, Seq: next - 1, Ack: 7000}},
		{"no-ack", Segment{Seq: next - 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsKeepaliveProbe(tc.seg, next) {
				t.Fatalf("%s incorrectly recognized as keepalive", tc.name)
			}
		})
	}
}

func TestIsKeepaliveProbeSequenceWrap(t *testing.T) {
	if !IsKeepaliveProbe(Segment{Flags: FlagACK, Seq: ^uint32(0)}, 0) {
		t.Fatal("keepalive recognition must preserve uint32 TCP sequence wrap")
	}
}
