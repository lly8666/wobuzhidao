package linkdata

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

func TestFECSizeClassesFragmentRecoveryOwnershipAndNoHOL(t *testing.T) {
	cfg := fecPathTestConfig(20, 1100)
	tx, _ := NewFECPath(cfg)
	rx, _ := NewFECPath(cfg)
	now := time.Unix(1, 0)
	large := bytes.Repeat([]byte{7}, 1250)
	small := bytes.Repeat([]byte{9}, 128)
	sources, err := tx.Encode(large, now)
	if err != nil || len(sources) != 2 {
		t.Fatalf("fragmented source len=%d err=%v", len(sources), err)
	}
	wire, err := tx.Encode(small, now)
	if err != nil || len(wire) != 1 {
		t.Fatalf("small source len=%d err=%v", len(wire), err)
	}
	// Lose the large fragment; deliver its short tail. The unrelated small
	// datagram must complete immediately while that assembly is unresolved.
	if got, err := rx.Decode(sources[1], now); err != nil || len(got) != 0 {
		t.Fatalf("partial assembly out=%d err=%v", len(got), err)
	}
	if got, err := rx.Decode(wire[0], now); err != nil || len(got) != 1 || !bytes.Equal(got[0], small) {
		t.Fatalf("cross-datagram HOL err=%v", err)
	}
	// The truly independent 128B datagram keeps its 8ms parity.
	// The 1250B datagram's two LINK fragments (including its short tail)
	// stay together until the bounded 16ms largest-group deadline.
	smallRepair, err := tx.FlushDue(now.Add(cfg.FlushAfter))
	if err != nil || len(smallRepair) != 1 {
		t.Fatalf("small 8ms parity len=%d err=%v", len(smallRepair), err)
	}
	largeRepair, err := tx.FlushDue(now.Add(2 * cfg.FlushAfter))
	if err != nil || len(largeRepair) != 2 {
		t.Fatalf("fragmented large 16ms parity len=%d err=%v", len(largeRepair), err)
	}
	repairs := append(smallRepair, largeRepair...)
	var completed [][]byte
	for i := len(repairs) - 1; i >= 0; i-- {
		got, err := rx.Decode(repairs[i], now.Add(cfg.FlushAfter))
		if err != nil {
			t.Fatal(err)
		}
		completed = append(completed, got...)
	}
	if len(completed) != 1 || !bytes.Equal(completed[0], large) {
		t.Fatalf("large recovered=%d", len(completed))
	}
	// Reuse every encoder class repeatedly; previously returned source and
	// parity must still be valid owned snapshots, including LINK tail frames.
	snapshots := append(append(append([][]byte(nil), sources...), wire...), repairs...)
	var copies [][]byte
	for _, packet := range snapshots {
		copies = append(copies, append([]byte(nil), packet...))
	}
	for i := 0; i < 60; i++ {
		if _, err := tx.Encode(bytes.Repeat([]byte{byte(i)}, [...]int{100, 300, 1000}[i%3]), now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Flush(); err != nil {
		t.Fatal(err)
	}
	for i, packet := range snapshots {
		if !bytes.Equal(packet, copies[i]) {
			t.Fatalf("owned wire mutated: %d", i)
		}
		if _, err := fec.ParseBlockHeader(packet); err != nil {
			t.Fatal(err)
		}
	}
}
