package linkdata

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

func TestSizeFailureIdentifiesDirectionAndPreservesSentinel(t *testing.T) {
	p, err := New(offConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Encode(make([]byte, 1401), time.Now())
	if !errors.Is(err, fec.ErrPacketTooLarge) || !strings.Contains(err.Error(), "encode: input_bytes=1401") {
		t.Fatalf("missing encode context: %v", err)
	}
	_, err = p.Decode(make([]byte, 1401))
	if !errors.Is(err, fec.ErrPacketTooLarge) || !strings.Contains(err.Error(), "decode off: wire_bytes=1401") {
		t.Fatalf("missing decode context: %v", err)
	}
}

func TestFECDecodeSizeFailureIncludesDeclaredAndActualLengths(t *testing.T) {
	p, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := p.Encode([]byte("source"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), wire[0]...)
	binary.BigEndian.PutUint16(bad[12:14], 1401)
	_, err = p.Decode(bad)
	if !errors.Is(err, fec.ErrPacketTooLarge) {
		t.Fatalf("lost error identity: %v", err)
	}
	for _, context := range []string{"linkdata decode:", "block=1", "shard=0", "wire_bytes=62", "declared_shard_bytes=1401", "expected_wire_bytes=1457"} {
		if !strings.Contains(err.Error(), context) {
			t.Fatalf("missing %q: %v", context, err)
		}
	}
}
