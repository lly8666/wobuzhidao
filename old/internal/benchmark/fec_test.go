package benchmark

import (
	"bytes"
	"testing"
)

func TestFECRecover2010And2020(t *testing.T) {
	for _, parity := range []int{10, 20} {
		g, err := fecGenerator(20, parity)
		if err != nil {
			t.Fatalf("generator 20:%d: %v", parity, err)
		}
		data := make([][]byte, 20)
		for i := range data {
			data[i] = make([]byte, 64)
			for j := range data[i] {
				data[i][j] = byte(i*17 + j*13)
			}
		}
		shards, err := fecEncode(data, g, 20, parity)
		if err != nil {
			t.Fatal(err)
		}
		present := make([]bool, len(shards))
		for i := range present {
			present[i] = true
		}
		for n := 0; n < parity; n++ {
			i := (n*7 + 3) % len(shards)
			for !present[i] {
				i = (i + 1) % len(shards)
			}
			present[i] = false
			shards[i] = nil
		}
		got, err := fecRecoverData(shards, present, g, 20)
		if err != nil {
			t.Fatalf("recover 20:%d: %v", parity, err)
		}
		for i := range data {
			if !bytes.Equal(got[i], data[i]) {
				t.Fatalf("20:%d shard %d mismatch", parity, i)
			}
		}
	}
}

func TestFECShardHeaderRoundTrip(t *testing.T) {
	body := []byte("hello-fec")
	p := encodeFECShard(9, 27, 20, 20, body)
	g, s, k, m, b, err := parseFECShard(p)
	if err != nil {
		t.Fatal(err)
	}
	if g != 9 || s != 27 || k != 20 || m != 20 || !bytes.Equal(b, body) {
		t.Fatalf("bad roundtrip: %d %d %d %d %q", g, s, k, m, b)
	}
}
