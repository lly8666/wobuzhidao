package tlsrecord

import (
	"bytes"
	"errors"
	"testing"
)

func TestSealOpenRoundTripAndPadding(t *testing.T) {
	keys := testKeys(t)
	sealer, err := NewSealer(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}
	opener, err := NewOpener(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte{0, 1, 2, 0, 255}
	wire, pn, err := sealer.Seal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if pn != 0 {
		t.Fatalf("pn = %d, want 0", pn)
	}
	record, err := opener.OpenRecord(wire)
	if err != nil {
		t.Fatal(err)
	}
	if record.PN != pn || !bytes.Equal(record.Payload, payload) {
		t.Fatalf("opened = pn %d payload %x", record.PN, record.Payload)
	}

	padded, err := sealRecord(keys, sealer.aead, sealer.maxBody, 42, KindLINK, payload, 7)
	if err != nil {
		t.Fatal(err)
	}
	record, err = opener.OpenRecord(padded)
	if err != nil {
		t.Fatal(err)
	}
	if record.PN != 42 || !bytes.Equal(record.Payload, payload) {
		t.Fatalf("padded open = pn %d payload %x", record.PN, record.Payload)
	}
}

func TestDecoderNoHOLAndMultiRecord(t *testing.T) {
	keys := testKeys(t)
	sealer, err := NewSealer(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(keys, MaxWireLen)
	if err != nil {
		t.Fatal(err)
	}

	a, _, err := sealer.Seal([]byte("A"))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := sealer.Seal([]byte("B"))
	if err != nil {
		t.Fatal(err)
	}

	got := decoder.OpenPayload(b)
	if len(got) != 1 || got[0].Err != nil || got[0].PN != 1 || string(got[0].Payload) != "B" {
		t.Fatalf("B result = %#v", got)
	}
	got = decoder.OpenPayload(a)
	if len(got) != 1 || got[0].Err != nil || got[0].PN != 0 || string(got[0].Payload) != "A" {
		t.Fatalf("A result = %#v", got)
	}
	if decoder.Stats().Late != 1 {
		t.Fatalf("late = %d, want 1", decoder.Stats().Late)
	}

	c, _, err := sealer.Seal([]byte("C"))
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := sealer.Seal([]byte("D"))
	if err != nil {
		t.Fatal(err)
	}
	combined := append(append([]byte(nil), c...), d...)
	got = decoder.OpenPayload(combined)
	if len(got) != 2 || got[0].Err != nil || got[1].Err != nil || got[0].PN != 2 || got[1].PN != 3 {
		t.Fatalf("combined result = %#v", got)
	}
}

func TestDecoderSkipsBadRecordWithTrustedLength(t *testing.T) {
	keys := testKeys(t)
	sealer, _ := NewSealer(keys, MaxWireLen)
	decoder, _ := NewDecoder(keys, MaxWireLen)

	bad, _, err := sealer.Seal([]byte("bad"))
	if err != nil {
		t.Fatal(err)
	}
	good, _, err := sealer.Seal([]byte("good"))
	if err != nil {
		t.Fatal(err)
	}
	bad = append([]byte(nil), bad...)
	bad[len(bad)-1] ^= 0x80
	combined := append(bad, good...)

	got := decoder.OpenPayload(combined)
	if len(got) != 2 {
		t.Fatalf("results = %d, want 2", len(got))
	}
	if !errors.Is(got[0].Err, ErrAuthentication) {
		t.Fatalf("first error = %v", got[0].Err)
	}
	if got[1].Err != nil || got[1].PN != 1 || string(got[1].Payload) != "good" {
		t.Fatalf("second = %#v", got[1])
	}
}

func TestDecoderDropsRemainderOnUntrustedLength(t *testing.T) {
	keys := testKeys(t)
	sealer, _ := NewSealer(keys, MaxWireLen)
	decoder, _ := NewDecoder(keys, MaxWireLen)
	good, _, err := sealer.Seal([]byte("good"))
	if err != nil {
		t.Fatal(err)
	}
	invalid := []byte{OuterType, 0x03, 0x03, 0x00, 0x01, 0x00}
	payload := append(invalid, good...)
	got := decoder.OpenPayload(payload)
	if len(got) != 1 || !errors.Is(got[0].Err, ErrInvalidLength) {
		t.Fatalf("result = %#v", got)
	}
	if decoder.Stats().Delivered != 0 {
		t.Fatalf("delivered = %d, want 0", decoder.Stats().Delivered)
	}
}

func TestDuplicateAndEvictionSemantics(t *testing.T) {
	keys := testKeys(t)
	sealer, _ := NewSealer(keys, MaxWireLen)
	decoder, _ := NewDecoder(keys, MaxWireLen)
	wire, _, err := sealer.Seal([]byte("once"))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoder.OpenPayload(wire); len(got) != 1 || got[0].Err != nil {
		t.Fatalf("first = %#v", got)
	}
	if got := decoder.OpenPayload(wire); len(got) != 1 || !errors.Is(got[0].Err, ErrDuplicate) {
		t.Fatalf("duplicate = %#v", got)
	}

	recent := newRecentPNSet()
	for pn := uint64(0); pn < uint64(RecentPNCapacity); pn++ {
		duplicate, _, _ := recent.observe(pn)
		if duplicate {
			t.Fatalf("unexpected duplicate at %d", pn)
		}
	}
	duplicate, evicted, _ := recent.observe(uint64(RecentPNCapacity))
	if duplicate || !evicted {
		t.Fatalf("first eviction duplicate=%v evicted=%v", duplicate, evicted)
	}
	duplicate, evicted, late := recent.observe(0)
	if duplicate || !evicted || !late {
		t.Fatalf("evicted old PN duplicate=%v evicted=%v late=%v", duplicate, evicted, late)
	}
}

func TestPNExhaustionAndFailedEncodingNeverReusePN(t *testing.T) {
	keys := testKeys(t)
	max := ^uint64(0)
	sealer, err := newSealerAtPN(keys, MaxWireLen, max-1)
	if err != nil {
		t.Fatal(err)
	}
	opener, _ := NewOpener(keys, MaxWireLen)
	for _, want := range []uint64{max - 1, max} {
		wire, pn, err := sealer.Seal(nil)
		if err != nil {
			t.Fatal(err)
		}
		if pn != want {
			t.Fatalf("pn = %d, want %d", pn, want)
		}
		record, err := opener.OpenRecord(wire)
		if err != nil || record.PN != want {
			t.Fatalf("open pn=%d err=%v, want %d", record.PN, err, want)
		}
	}
	if _, _, err := sealer.Seal(nil); !errors.Is(err, ErrPNExhausted) {
		t.Fatalf("exhausted error = %v", err)
	}

	tiny, err := NewSealer(keys, FixedWireOverhead)
	if err != nil {
		t.Fatal(err)
	}
	if _, pn, err := tiny.Seal([]byte{1}); !errors.Is(err, ErrPayloadTooLarge) || pn != 0 {
		t.Fatalf("oversize pn=%d err=%v", pn, err)
	}
	wire, pn, err := tiny.Seal(nil)
	if err != nil {
		t.Fatal(err)
	}
	if pn != 1 {
		t.Fatalf("next pn = %d, want 1", pn)
	}
	if len(wire) != FixedWireOverhead {
		t.Fatalf("empty record len = %d, want %d", len(wire), FixedWireOverhead)
	}
}

func TestUnknownKindDoesNotPoisonFollowingRecord(t *testing.T) {
	keys := testKeys(t)
	sealer, _ := NewSealer(keys, MaxWireLen)
	decoder, _ := NewDecoder(keys, MaxWireLen)
	bad, err := sealRecord(keys, sealer.aead, sealer.maxBody, 7, 0x7f, []byte("ignored"), 0)
	if err != nil {
		t.Fatal(err)
	}
	good, err := sealRecord(keys, sealer.aead, sealer.maxBody, 8, KindLINK, []byte("good"), 0)
	if err != nil {
		t.Fatal(err)
	}
	got := decoder.OpenPayload(append(bad, good...))
	if len(got) != 2 || !errors.Is(got[0].Err, ErrUnknownKind) || got[1].Err != nil || got[1].PN != 8 {
		t.Fatalf("result = %#v", got)
	}
}

func TestRecordNonceUniqueAcrossBoundaries(t *testing.T) {
	iv := testKeys(t).IV
	pns := []uint64{0, 1, (1 << 32) - 1, 1 << 32, (1 << 32) + 1, ^uint64(0) - 1, ^uint64(0)}
	seen := map[[12]byte]uint64{}
	for _, pn := range pns {
		nonce := recordNonce(iv, pn)
		if prior, ok := seen[nonce]; ok {
			t.Fatalf("nonce reused by pn %d and %d", prior, pn)
		}
		seen[nonce] = pn
	}
}
