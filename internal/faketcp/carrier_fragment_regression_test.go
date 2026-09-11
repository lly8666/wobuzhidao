package faketcp

import (
	"bytes"
	"testing"
)

func deterministicCarrierPayload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte((i*131 + 17) & 0xff)
	}
	return p
}

func TestCarrierPayloadBudgetForIPv4TCPPathMTU(t *testing.T) {
	got, err := CarrierPayloadBudget(1500)
	if err != nil {
		t.Fatalf("CarrierPayloadBudget(1500): %v", err)
	}
	if got != 1460 {
		t.Fatalf("IPv4/TCP carrier payload budget=%d want=1460", got)
	}
	if _, err := CarrierPayloadBudget(40); err == nil {
		t.Fatal("path MTU with no room for carrier payload unexpectedly accepted")
	}
}

func TestCarrierFragmentationRoundTripsOversizeDTLSDatagram(t *testing.T) {
	// The pinned wolfSSL 5.9.2 diagnostic probe observed a 1456-byte plaintext
	// datagram as a 1478-byte DTLS transport datagram. A 1500-byte IPv4 path can
	// carry only 1460 bytes after the outer FakeTCP IPv4+TCP headers, so this exact
	// case must be split below FakeTCP and restored before wolfSSL sees it.
	const pathMTU = 1500
	original := deterministicCarrierPayload(1478)
	fragmenter, err := NewCarrierFragmenter(pathMTU)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := fragmenter.Fragment(original)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) < 2 {
		t.Fatalf("oversize DTLS datagram produced %d carrier frame(s), want fragmentation", len(frames))
	}
	budget, _ := CarrierPayloadBudget(pathMTU)
	for i, frame := range frames {
		if len(frame) > budget {
			t.Fatalf("frame %d len=%d exceeds carrier payload budget=%d", i, len(frame), budget)
		}
	}

	// FakeTCP intentionally accepts out-of-order first-arrival payloads and uses
	// SACK/RTO only for background recovery. Carrier reassembly therefore cannot
	// assume fragment arrival order.
	reassembler := NewCarrierReassembler()
	var got []byte
	complete := false
	for i := len(frames) - 1; i >= 0; i-- {
		out, done, err := reassembler.Push(frames[i])
		if err != nil {
			t.Fatalf("push frame %d: %v", i, err)
		}
		if done {
			if complete {
				t.Fatal("same carrier datagram completed more than once")
			}
			complete = true
			got = out
		}
	}
	if !complete {
		t.Fatal("fragmented carrier datagram never completed")
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("reassembled datagram mismatch: got=%d bytes want=%d", len(got), len(original))
	}
}

func TestCarrierFramingPreservesSmallDatagram(t *testing.T) {
	original := deterministicCarrierPayload(1200)
	fragmenter, err := NewCarrierFragmenter(1500)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := fragmenter.Fragment(original)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("small datagram produced %d frames, want 1", len(frames))
	}
	reassembler := NewCarrierReassembler()
	got, done, err := reassembler.Push(frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("single-frame datagram did not complete immediately")
	}
	if !bytes.Equal(got, original) {
		t.Fatal("single-frame carrier round trip changed payload")
	}
}
