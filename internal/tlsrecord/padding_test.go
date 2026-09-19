package tlsrecord

import (
	"bytes"
	"errors"
	"testing"
)

func TestSealWithPaddingPreservesDefaultZeroWire(t *testing.T) {
	keys := testKeys(t)
	payload := []byte{0x41, 0x00, 0x42, 0x00, 0x00}

	def, err := NewSealer(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := NewSealer(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}
	want, wantPN, err := def.Seal(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, gotPN, err := explicit.SealWithPadding(payload, 0)
	if err != nil {
		t.Fatal(err)
	}
	if wantPN != 0 || gotPN != 0 || !bytes.Equal(got, want) {
		t.Fatalf("zero padding changed wire: default pn=%d wire=%x explicit pn=%d wire=%x", wantPN, want, gotPN, got)
	}
}

func TestSealWithPaddingTailZeroPayloadRoundTripAndStats(t *testing.T) {
	keys := testKeys(t)
	sealer, _ := NewSealer(keys, MaxWireLen)
	opener, _ := NewOpener(keys, MaxWireLen)
	payload := []byte{0x41, 0x00, 0x42, 0x00, 0x00}

	wire, pn, err := sealer.SealWithPadding(payload, 9)
	if err != nil {
		t.Fatal(err)
	}
	if pn != 0 || len(wire) != FixedWireOverhead+len(payload)+9 {
		t.Fatalf("pn=%d wire=%d want=%d", pn, len(wire), FixedWireOverhead+len(payload)+9)
	}
	record, err := opener.OpenRecord(wire)
	if err != nil {
		t.Fatal(err)
	}
	if record.PN != pn || !bytes.Equal(record.Payload, payload) {
		t.Fatalf("opened pn=%d payload=%x want=%x", record.PN, record.Payload, payload)
	}
	stats := sealer.Stats()
	if stats.Records != 1 || stats.Failed != 0 || stats.PaddingRequests != 1 ||
		stats.RequestedPaddingBytes != 9 || stats.PaddedRecords != 1 || stats.PaddingBytes != 9 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestSealWithPaddingInvalidLengthsAreExplicit(t *testing.T) {
	keys := testKeys(t)
	sealer, err := NewSealer(keys, FixedWireOverhead+5)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sealer.SealWithPadding([]byte("abc"), -1); !errors.Is(err, ErrInvalidPadding) {
		t.Fatalf("negative padding err=%v", err)
	}
	if _, _, err := sealer.SealWithPadding([]byte("abc"), 3); !errors.Is(err, ErrPaddingTooLarge) {
		t.Fatalf("over-headroom padding err=%v", err)
	}
	if _, _, err := sealer.SealWithPadding([]byte("abcdef"), 0); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("base payload overflow err=%v", err)
	}
	stats := sealer.Stats()
	if stats.Failed != 3 || stats.PaddingRequests != 3 || stats.Records != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestPaddedAuthenticationFailureDoesNotPoisonPN(t *testing.T) {
	keys := testKeys(t)
	sealer, _ := NewSealer(keys, MaxWireLen)
	decoder, _ := NewDecoder(keys, MaxWireLen)
	wire, _, err := sealer.SealWithPadding([]byte{0x10, 0x00, 0x20, 0x00}, 11)
	if err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), wire...)
	bad[len(bad)-1] ^= 0x80
	got := decoder.OpenPayload(bad)
	if len(got) != 1 || !errors.Is(got[0].Err, ErrAuthentication) {
		t.Fatalf("bad result=%#v", got)
	}
	got = decoder.OpenPayload(wire)
	if len(got) != 1 || got[0].Err != nil || got[0].PN != 0 ||
		!bytes.Equal(got[0].Payload, []byte{0x10, 0x00, 0x20, 0x00}) {
		t.Fatalf("valid result=%#v", got)
	}
}
