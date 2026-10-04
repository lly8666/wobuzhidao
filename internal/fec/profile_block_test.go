package fec

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestProfileStreamingWireRecoversExactlyRMissingSources(t *testing.T) {
	for _, parity := range []int{4, 8, 12, 16, 20} {
		t.Run(fmt.Sprintf("20x%d", parity), func(t *testing.T) {
			enc, err := NewProfileFastBlockEncoder(parity, 1400, 8*time.Millisecond, 77)
			if err != nil {
				t.Fatal(err)
			}
			dec, err := NewProfileBlockDecoder(1400, 8)
			if err != nil {
				t.Fatal(err)
			}

			want := make([][]byte, DataShards)
			var wire [][]byte
			now := time.Unix(10, 0)
			for i := 0; i < DataShards; i++ {
				p := bytes.Repeat([]byte{byte(i + 1)}, 100+i)
				want[i] = append([]byte(nil), p...)
				out, err := enc.Add(p, now.Add(time.Duration(i)*time.Microsecond))
				if err != nil {
					t.Fatal(err)
				}
				for _, d := range out {
					wire = append(wire, append([]byte(nil), d...))
				}
			}
			if got := len(wire); got != DataShards+parity {
				t.Fatalf("wire=%d want=%d", got, DataShards+parity)
			}

			// Drop source indices [0,R). Keep every parity shard, yielding exactly
			// K=20 received equations. Feed in reverse order to force parity-first
			// metadata and arbitrary reorder handling.
			kept := make([][]byte, 0, DataShards)
			for _, d := range wire {
				h, err := ParseProfileBlockHeader(d[:HeaderSize])
				if err != nil {
					t.Fatal(err)
				}
				if h.Streaming && int(h.ShardIndex) < parity {
					continue
				}
				kept = append(kept, d)
			}
			if len(kept) != DataShards {
				t.Fatalf("kept=%d want=%d", len(kept), DataShards)
			}
			got := make(map[byte][]byte)
			for i := len(kept) - 1; i >= 0; i-- {
				packets, _, err := dec.Add(kept[i])
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range packets {
					got[p[0]-1] = p
				}
			}
			if dec.InFlight() != 0 {
				t.Fatalf("decoder retained completed block: %d", dec.InFlight())
			}
			for i, p := range want {
				if !bytes.Equal(got[byte(i)], p) {
					t.Fatalf("packet %d mismatch/missing", i)
				}
			}
		})
	}
}

func TestProfilePartialFlushImmediateSourcesAndRepair(t *testing.T) {
	for _, parity := range []int{4, 8, 12, 16, 20} {
		enc, err := NewProfileFastBlockEncoder(parity, 1400, 8*time.Millisecond, 1)
		if err != nil {
			t.Fatal(err)
		}
		dec, _ := NewProfileBlockDecoder(1400, 8)
		now := time.Unix(20, 0)
		packets := [][]byte{[]byte("alpha"), []byte("bravo-bravo"), []byte("charlie")}
		var sources [][]byte
		for i, p := range packets {
			out, err := enc.Add(p, now.Add(time.Duration(i)*time.Millisecond))
			if err != nil || len(out) != 1 {
				t.Fatalf("R=%d add=%d out=%d err=%v", parity, i, len(out), err)
			}
			sources = append(sources, append([]byte(nil), out[0]...))
		}
		if out, err := enc.FlushDue(now.Add(7 * time.Millisecond)); err != nil || len(out) != 0 {
			t.Fatalf("R=%d early flush out=%d err=%v", parity, len(out), err)
		}
		repairs, err := enc.FlushDue(now.Add(8 * time.Millisecond))
		if err != nil || len(repairs) != parity {
			t.Fatalf("R=%d repairs=%d err=%v", parity, len(repairs), err)
		}

		// Deliver source 0 and 2 immediately, omit source 1, then provide repair.
		var got [][]byte
		for _, d := range [][]byte{sources[0], sources[2]} {
			out, _, err := dec.Add(d)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, out...)
		}
		if len(got) != 2 || !bytes.Equal(got[0], packets[0]) || !bytes.Equal(got[1], packets[2]) {
			t.Fatalf("R=%d systematic first delivery=%q", parity, got)
		}
		for _, d := range repairs {
			out, _, err := dec.Add(d)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, out...)
		}
		found := false
		for _, p := range got {
			if bytes.Equal(p, packets[1]) {
				found = true
			}
		}
		if !found || dec.InFlight() != 0 {
			t.Fatalf("R=%d missing repaired packet/inflight=%d", parity, dec.InFlight())
		}
	}
}

func TestProfileWireVersionAndParityMismatchRejected(t *testing.T) {
	enc4, _ := NewProfileFastBlockEncoder(4, 1400, 8*time.Millisecond, 9)
	enc8, _ := NewProfileFastBlockEncoder(8, 1400, 8*time.Millisecond, 9)
	now := time.Unix(30, 0)
	a, _ := enc4.Add([]byte("same-block-source"), now)
	b, _ := enc8.Add([]byte("same-block-source"), now)
	dec, _ := NewProfileBlockDecoder(1400, 8)
	if _, _, err := dec.Add(a[0]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := dec.Add(b[0]); !errors.Is(err, ErrHeaderMismatch) {
		t.Fatalf("mixed parity geometry err=%v", err)
	}

	if _, err := ParseBlockHeader(a[0][:HeaderSize]); err == nil {
		t.Fatal("legacy v1 parser accepted variable-parity v2 header")
	}
	legacy := BlockHeader{BlockID: 1, ShardIndex: 0, DataCount: 1, ShardSize: 3, OriginalLengths: [DataShards]uint16{3}}
	lb, err := legacy.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProfileBlockHeader(lb); err == nil {
		t.Fatal("profile v2 parser accepted legacy v1 header")
	}
}
