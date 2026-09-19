package tlsrecord

import (
	"bytes"
	"errors"
	"testing"
)

func testNonce() [16]byte {
	var nonce [16]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	return nonce
}

func testMaster() []byte {
	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i)
	}
	return master
}

func testKeys(t testing.TB) Keys {
	t.Helper()
	pair, err := DeriveKeys(testMaster(), testNonce())
	if err != nil {
		t.Fatal(err)
	}
	return pair.C2S
}

func TestExporterContextHashFieldSensitive(t *testing.T) {
	base, err := ExporterContextHash(1, testNonce(), []byte("tunnel-vector-01"), 1500, 1600)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ExporterContextHash(1, testNonce(), []byte("tunnel-vector-01"), 1500, 1600)
	if err != nil {
		t.Fatal(err)
	}
	if base != again {
		t.Fatal("same context produced different hash")
	}
	changed, err := ExporterContextHash(1, testNonce(), []byte("tunnel-vector-02"), 1500, 1600)
	if err != nil {
		t.Fatal(err)
	}
	if base == changed {
		t.Fatal("TunnelID change did not affect exporter context hash")
	}
	tooLong := bytes.Repeat([]byte{1}, 65536)
	if _, err := ExporterContextHash(1, testNonce(), tooLong, 1500, 1600); !errors.Is(err, ErrTunnelIDTooLong) {
		t.Fatalf("oversize TunnelID error = %v", err)
	}
}

func TestDeriveKeysDirectionSeparated(t *testing.T) {
	pair, err := DeriveKeys(testMaster(), testNonce())
	if err != nil {
		t.Fatal(err)
	}
	if pair.C2S == pair.S2C {
		t.Fatal("direction keys unexpectedly equal")
	}
	if _, err := DeriveKeys(testMaster()[:31], testNonce()); !errors.Is(err, ErrInvalidMasterLength) {
		t.Fatalf("short master error = %v", err)
	}
}
