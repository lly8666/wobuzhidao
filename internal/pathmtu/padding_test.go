package pathmtu

import (
	"errors"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func TestPaddingHeadroomUsesUnifiedRecordBudget(t *testing.T) {
	for _, mtu := range []int{576, 1280, 1400, 1500, 1600, 9000} {
		cfg := baseConfig(mtu, 20)
		b, err := Derive(cfg)
		if err != nil {
			t.Fatalf("mtu=%d: %v", mtu, err)
		}
		for _, used := range []int{0, 1, b.RecordPayloadMTU / 2, b.RecordPayloadMTU} {
			got, err := b.PaddingHeadroom(used)
			if err != nil {
				t.Fatalf("mtu=%d used=%d: %v", mtu, used, err)
			}
			want := b.RecordWireMTU - tlsrecord.FixedWireOverhead - used
			if got != want {
				t.Fatalf("mtu=%d used=%d headroom=%d want=%d", mtu, used, got, want)
			}
		}
		if got, err := b.PaddingHeadroom(b.RecordPayloadMTU); err != nil || got != 0 {
			t.Fatalf("mtu=%d full headroom=%d err=%v", mtu, got, err)
		}
	}
}

func TestPaddingHeadroomRejectsPayloadOutsideBudget(t *testing.T) {
	b, err := Derive(baseConfig(1500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.PaddingHeadroom(-1); !errors.Is(err, ErrPayloadBudget) {
		t.Fatalf("negative payload err=%v", err)
	}
	if _, err := b.PaddingHeadroom(b.RecordPayloadMTU + 1); !errors.Is(err, ErrPayloadBudget) {
		t.Fatalf("oversize payload err=%v", err)
	}

	cfg := baseConfig(1500, 0)
	cfg.PeerMSS = 900
	cfg.RecordWireLimit = 800
	b, err = Derive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.RecordWireMTU != 800 {
		t.Fatalf("record wire=%d want=800", b.RecordWireMTU)
	}
	if got, err := b.PaddingHeadroom(700); err != nil || got != 69 {
		t.Fatalf("headroom=%d err=%v want=69", got, err)
	}
}
