package main

import (
	"fmt"
	"testing"
)

func TestClientLinkConfigFixedParityMatrix(t *testing.T) {
	for _, parity := range []int{4, 8, 10, 12, 16, 20} {
		name := fmt.Sprintf("20:%d", parity)
		cfg, err := clientLinkConfig(options{fec: name, mtu: 1400, flushMS: 8, lanes: 1})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if int(cfg.DataShards) != 20 || int(cfg.ParityShards) != parity {
			t.Fatalf("%s -> %d:%d", name, cfg.DataShards, cfg.ParityShards)
		}
	}
}
