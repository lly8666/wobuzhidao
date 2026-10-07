//go:build linux

package faketcp

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestRawWriteTimingDefaultOffToggleAndFailedCallAccounting(t *testing.T) {
	for _, path := range []string{"single", "native-batch", "fallback-batch"} {
		t.Run(path, func(t *testing.T) {
			e := &RawIPv4Endpoint{sendFD: -1, ipID: 1, persona: PacketPersonaLegacy,
				sendBatchDisabled: path == "fallback-batch"}
			e.SetIODiagnostics(true)
			seg := Segment{SrcIP: [4]byte{127, 0, 0, 2}, DstIP: [4]byte{127, 0, 0, 1},
				SrcPort: 40000, DstPort: 443, Flags: FlagACK, Payload: []byte("owned")}
			for stage := 0; stage < 3; stage++ {
				// First call uses the actual default. The final disabled call must
				// preserve previous evidence without recording further durations.
				if stage > 0 {
					e.SetWriteTimingDiagnostics(stage == 1)
				}
				var err error
				if path == "single" {
					var packet []byte
					packet, err = e.WriteSegment(seg)
					if packet != nil {
						t.Fatal("failed send unexpectedly returned wire bytes")
					}
				} else {
					var sent int
					sent, err = e.WriteSegments([]Segment{seg, seg})
					if sent != 0 {
						t.Fatalf("failed batch returned sent prefix=%d", sent)
					}
				}
				if !errors.Is(err, syscall.EBADF) {
					t.Fatalf("original syscall failure must remain unchanged: %v", err)
				}
				stats := e.IODiagnostic()
				timing := stats.WriteTiming
				var want uint64
				if stage > 0 {
					want = 1
				}
				if timing.Enabled != (stage == 1) || timing.LockWait.Samples != want ||
					timing.LockHold.Samples != want || timing.Marshal.Samples != want || timing.Syscall.Samples != want {
					t.Fatalf("stage=%d unmatched timing or default-off activity: %+v", stage, timing)
				}
				if stats.SendCalls != uint64(stage+1) || stats.SendMessages != 0 {
					t.Fatalf("timing changed existing attempt accounting: %+v", stats)
				}
				if stage == 0 && timing != (RawWriteTimingDiagnostic{}) {
					t.Fatalf("disabled diagnostics accumulated durations: %+v", timing)
				}
			}
		})
	}
}

func TestRawWriteDurationKeepsUnitsAndBoundedTotals(t *testing.T) {
	var d rawWriteDuration
	d.observe(12 * time.Millisecond)
	d.observe(3 * time.Millisecond)
	d.observe(-time.Millisecond)
	got := d.snapshot()
	if got.Samples != 3 || got.TotalNS != uint64(15*time.Millisecond) ||
		got.MaxNS != uint64(12*time.Millisecond) || got.Over10MS != 1 {
		t.Fatalf("bad nanosecond accounting: %+v", got)
	}
}
