package fec

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func ownSizeClassWire(dst [][]byte, wire [][]byte) [][]byte {
	for _, packet := range wire {
		dst = append(dst, append([]byte(nil), packet...))
	}
	return dst
}

// Interleaved large/small sources exercise the production cause: parity is
// sized by the largest source IN ITS GROUP, rather than in the entire lane.
func TestSizeClassEncoderMixedProfilesRecoveryAndByteBudget(t *testing.T) {
	for _, parity := range []int{20} {
		t.Run(itoaSmall(parity), func(t *testing.T) {
			enc, err := NewSizeClassEncoder(NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, 1, parity)
			if err != nil {
				t.Fatal(err)
			}
			legacy, _ := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1200, time.Second, 1, parity)
			var wire [][]byte
			want := make(map[byte][]byte)
			for i := 0; i < DataShards; i++ {
				for c, size := range [...]int{128, 320, 1100} {
					id := byte(i*3 + c + 1)
					packet := bytes.Repeat([]byte{id}, size)
					want[id] = packet
					out, err := enc.Add(packet, time.Unix(1, 0))
					if err != nil || len(out) == 0 || !bytes.Equal(out[0][HeaderSize:], packet) {
						t.Fatalf("source must leave immediately: id=%d err=%v", id, err)
					}
					wire = ownSizeClassWire(wire, out)
					if _, err := legacy.Add(packet, time.Unix(1, 0)); err != nil {
						t.Fatal(err)
					}
				}
			}
			stats := enc.Stats()
			wantParityBytes := uint64(parity * (3*HeaderSize + 128 + 320 + 1100))
			if stats.FullBlocks != 3 || stats.PartialBlocks != 0 || stats.PendingSources != 0 || stats.SizeClasses != 3 || stats.ParityBytes != wantParityBytes {
				t.Fatalf("stats=%+v expected parity bytes=%d", stats, wantParityBytes)
			}
			if stats.ParityBytes*100 >= legacy.Stats().ParityBytes*55 {
				t.Fatalf("mixed parity inflation not removed: grouped=%d mixed=%d", stats.ParityBytes, legacy.Stats().ParityBytes)
			}
			var sourceBytes, parityBytes uint64
			dec, _ := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1200, 8, parity)
			got := make(map[byte][]byte)
			ids := make(map[uint32]bool)
			// Parity first, reverse order; erase exactly R sources from each
			// class. An unmodified v1 decoder must reconstruct all 60 packets.
			for pass := 0; pass < 2; pass++ {
				for i := len(wire) - 1; i >= 0; i-- {
					packet := wire[i]
					h, err := ParseBlockHeader(packet)
					if err != nil {
						t.Fatal(err)
					}
					isParity := int(h.ShardIndex) >= DataShards
					if pass == 0 {
						ids[h.BlockID] = true
						if isParity {
							parityBytes += uint64(len(packet))
						} else {
							sourceBytes += uint64(len(packet))
						}
					}
					if (pass == 0) != isParity || (!isParity && int(h.ShardIndex) < parity) {
						continue
					}
					packets, _, err := dec.Add(packet)
					if err != nil {
						t.Fatal(err)
					}
					for _, recovered := range packets {
						if _, exists := got[recovered[0]]; exists {
							t.Fatal("duplicate first delivery")
						}
						got[recovered[0]] = append([]byte(nil), recovered...)
					}
				}
			}
			if len(ids) != 3 || !ids[1] || !ids[2] || !ids[3] || len(got) != len(want) || sourceBytes != stats.SourceBytes || parityBytes != stats.ParityBytes {
				t.Fatalf("ids=%v recovered=%d/%d bytes=%d/%d stats=%+v", ids, len(got), len(want), sourceBytes, parityBytes, stats)
			}
			for id, packet := range want {
				if !bytes.Equal(got[id], packet) {
					t.Fatalf("corrupt recovered source %d", id)
				}
			}
		})
	}
}

func TestSizeClassEncoderLowerProfilesPreserveWireAndPartialPolicy(t *testing.T) {
	for _, parity := range SupportedParityShards() {
		if parity == 20 {
			continue
		}
		enc, _ := NewSizeClassEncoder(NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, 1, parity)
		legacy, _ := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, 1, parity)
		if enc.Stats().SizeClasses != 1 {
			t.Fatalf("20:%d changed grouping", parity)
		}
		compare := func(a, b [][]byte) {
			t.Helper()
			if len(a) != len(b) {
				t.Fatalf("20:%d wire count changed: %d/%d", parity, len(a), len(b))
			}
			for i := range a {
				if !bytes.Equal(a[i], b[i]) {
					t.Fatalf("20:%d wire changed at %d", parity, i)
				}
			}
		}
		now := time.Unix(1, 0)
		for i := 0; i < 27; i++ {
			packet := bytes.Repeat([]byte{byte(i)}, [...]int{128, 320, 1100}[i%3])
			a, err := enc.Add(packet, now)
			if err != nil {
				t.Fatal(err)
			}
			b, err := legacy.Add(packet, now)
			if err != nil {
				t.Fatal(err)
			}
			compare(a, b)
		}
		a, _ := enc.FlushDue(now.Add(8 * time.Millisecond))
		b, _ := legacy.FlushDue(now.Add(8 * time.Millisecond))
		compare(a, b)
	}
}

func TestSizeClassEncoderIndependentDeadlineWrapAndLateSource(t *testing.T) {
	enc, _ := NewSizeClassEncoder(NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, ^uint32(0), 20)
	now := time.Unix(1, 0)
	var sources [][]byte
	for i, size := range [...]int{100, 300, 1000} {
		out, err := enc.Add(bytes.Repeat([]byte{byte(i + 1)}, size), now.Add(time.Duration(i)*time.Millisecond))
		if err != nil || len(out) != 1 {
			t.Fatalf("source %d err=%v", i, err)
		}
		h, _ := ParseBlockHeader(out[0])
		if h.BlockID != ^uint32(0)+uint32(i) {
			t.Fatalf("wrap allocation id=%d", h.BlockID)
		}
		sources = ownSizeClassWire(sources, out)
	}
	if out, err := enc.FlushDue(now.Add(7 * time.Millisecond)); err != nil || len(out) != 0 {
		t.Fatalf("early flush out=%d err=%v", len(out), err)
	}
	parity, err := enc.FlushDue(now.Add(8 * time.Millisecond))
	if err != nil || len(parity) != 1 || enc.Stats().PendingBlocks != 2 {
		t.Fatalf("first deadline out=%d stats=%+v err=%v", len(parity), enc.Stats(), err)
	}
	dec, _ := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1200, 8, 20)
	got, _, err := dec.Add(parity[0])
	if err != nil || len(got) != 1 || !bytes.Equal(got[0], sources[0][HeaderSize:]) {
		t.Fatalf("one-source partial recovery err=%v", err)
	}
	if got, _, err := dec.Add(sources[0]); err != nil || len(got) != 0 {
		t.Fatalf("late original duplicate out=%d err=%v", len(got), err)
	}
	// Unresolved classes do not hold up a first-arriving source in another.
	if got, _, err := dec.Add(sources[2]); err != nil || len(got) != 1 {
		t.Fatalf("cross-class HOL out=%d err=%v", len(got), err)
	}
	parity, err = enc.FlushDue(now.Add(10 * time.Millisecond))
	if err != nil || len(parity) != 2 || enc.Pending() != 0 {
		t.Fatalf("independent deadlines out=%d pending=%d err=%v", len(parity), enc.Pending(), err)
	}
	out, _ := enc.Add([]byte("next"), now.Add(11*time.Millisecond))
	h, _ := ParseBlockHeader(out[0])
	if h.BlockID != 2 {
		t.Fatalf("class reused global id: %d", h.BlockID)
	}
}

func TestSizeClassEncoderBoundsAndForcedFlush(t *testing.T) {
	for _, tc := range []struct{ mtu, classes int }{{64, 1}, {256, 1}, {300, 2}, {512, 2}, {513, 3}} {
		enc, err := NewSizeClassEncoder(NewFastReedSolomon20x20(), tc.mtu, time.Second, 1, 20)
		if err != nil || enc.Stats().SizeClasses != tc.classes {
			t.Fatalf("mtu=%d err=%v", tc.mtu, err)
		}
		for _, invalid := range [][]byte{nil, make([]byte, tc.mtu+1)} {
			if _, err := enc.Add(invalid, time.Unix(1, 0)); !errors.Is(err, ErrPacketTooLarge) {
				t.Fatalf("invalid packet accepted: %v", err)
			}
		}
		for _, group := range enc.groups[:enc.count] {
			for i := 0; i < DataShards-1; i++ {
				if _, err := enc.Add(make([]byte, group.maxPacketSize), time.Unix(1, 0)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if enc.Pending() != tc.classes*(DataShards-1) {
			t.Fatalf("unbounded or incorrect pending=%d", enc.Pending())
		}
		out, err := enc.Flush()
		if err != nil || len(out) != tc.classes*(DataShards-1) || enc.Pending() != 0 || enc.Stats().PartialBlocks != uint64(tc.classes) {
			t.Fatalf("forced flush len=%d stats=%+v err=%v", len(out), enc.Stats(), err)
		}
	}
}
