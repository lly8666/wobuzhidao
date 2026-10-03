package fec

import (
	"errors"
	"testing"
	"time"
)

func TestLivePressureAdmitsParityFirstAndKeepsLateFirstDelivery(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	old, _ := NewFastBlockEncoder(codec, 256, time.Millisecond, 1)
	fresh, _ := NewFastBlockEncoder(codec, 256, time.Millisecond, 2)
	now := time.Unix(100, 0)
	a, _ := old.Add([]byte("old-first"), now)
	a0 := append([]byte(nil), a[0]...)
	a, _ = old.Add([]byte("old-late"), now)
	a1 := append([]byte(nil), a[0]...)
	fresh.Add([]byte("fresh-recovered"), now)
	parity, _ := fresh.Flush()
	d, _ := NewBlockDecoder(codec, 256, 1)
	if out, _, err := d.AddLive(a0); err != nil || len(out) != 1 {
		t.Fatalf("initial source: out=%q err=%v", out, err)
	}
	// Missing every systematic source of the fresh partial block must not turn
	// a normal pressure decision into ErrDecoderFull or delay it behind block1.
	if out, _, err := d.AddLive(parity[0]); err != nil || len(out) != 1 || string(out[0]) != "fresh-recovered" {
		t.Fatalf("fresh parity recovery: out=%q err=%v", out, err)
	}
	if d.PressureCounts().PressureRetirements != 1 || d.LastPressureRetiredBlock() != 1 || d.InFlight() != 0 {
		t.Fatalf("pressure state=%+v retired=%d", d.PressureCounts(), d.LastPressureRetiredBlock())
	}
	if out, _, err := d.AddLive(a1); err != nil || len(out) != 1 || string(out[0]) != "old-late" {
		t.Fatalf("late first source: out=%q err=%v", out, err)
	}
	if out, _, err := d.AddLive(a1); err != nil || len(out) != 0 {
		t.Fatalf("late duplicate: out=%q err=%v", out, err)
	}
	// The reference API keeps its existing full-window contract.
	ref, _ := NewBlockDecoder(codec, 256, 1)
	ref.Add(a0)
	if _, _, err := ref.Add(parity[0]); !errors.Is(err, ErrDecoderFull) {
		t.Fatalf("reference capacity error=%v", err)
	}
}

func TestLivePressureLateParityDoesNotEvictNewerBlock(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	now := time.Unix(100, 0)
	newer, _ := NewFastBlockEncoder(codec, 256, time.Millisecond, 20)
	late, _ := NewFastBlockEncoder(codec, 256, time.Millisecond, 10)
	a, _ := newer.Add([]byte("newer"), now)
	b, _ := late.Add([]byte("late"), now)
	b0 := append([]byte(nil), b[0]...)
	parity, _ := late.Flush()
	d, _ := NewBlockDecoder(codec, 256, 1)
	d.AddLive(a[0])
	if out, _, err := d.AddLive(parity[0]); err != nil || len(out) != 0 || !d.IsHeavyBlock(20) || d.IsHeavyBlock(10) {
		t.Fatalf("late parity: out=%q err=%v state=%+v", out, err, d.PressureCounts())
	}
	if out, _, err := d.AddLive(b0); err != nil || len(out) != 1 || string(out[0]) != "late" {
		t.Fatalf("late systematic: out=%q err=%v", out, err)
	}
	bad := append([]byte(nil), parity[0]...)
	bad[0] = 'X'
	if _, _, err := d.AddLive(bad); err == nil || !d.IsHeavyBlock(20) {
		t.Fatal("invalid wire was accepted or evicted fresh recovery")
	}
}
