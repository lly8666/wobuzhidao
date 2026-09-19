package control

import (
	"errors"
	"testing"
)

func TestCurrentLinkPolicyAdmitsFixedParityMatrix(t *testing.T) {
	p := CurrentLinkPolicy()
	for _, parity := range []uint8{4, 8, 10, 12, 16, 20} {
		cfg := fixed20x20Link()
		cfg.ParityShards = parity
		if err := p.Validate(cfg); err != nil {
			t.Fatalf("20:%d rejected: %v", parity, err)
		}
	}
	cfg := fixed20x20Link()
	cfg.ParityShards = 9
	if err := p.Validate(cfg); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("20:9 err=%v want ErrUnsupported", err)
	}
}
