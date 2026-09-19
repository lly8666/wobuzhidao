package faketcp

import (
	"bytes"
	"testing"
	"time"
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

func TestCarrierReassemblyExpiryDropsLateFragmentsWithoutResurrection(t *testing.T) {
	fragmenter, err := NewCarrierFragmenter(1500)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := fragmenter.Fragment(deterministicCarrierPayload(3000))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) < 3 {
		t.Fatalf("frames=%d want at least 3", len(frames))
	}

	r := NewCarrierReassembler()
	t0 := time.Unix(1000, 0)
	if out, done, err := r.pushAt(frames[0], t0); err != nil || done || out != nil {
		t.Fatalf("first partial: out=%d done=%t err=%v", len(out), done, err)
	}
	if len(r.assemblies) != 1 {
		t.Fatalf("assemblies=%d want=1", len(r.assemblies))
	}

	// At the partial-reliability horizon the old assembly is retired first; the
	// arriving sibling fragment for the same datagram ID must be ignored rather
	// than resurrecting payload state.
	if out, done, err := r.pushAt(frames[1], t0.Add(carrierAssemblyTTL)); err != nil || done || out != nil {
		t.Fatalf("late retired fragment: out=%d done=%t err=%v", len(out), done, err)
	}
	if len(r.assemblies) != 0 {
		t.Fatalf("late fragment resurrected %d assembly(s)", len(r.assemblies))
	}
	if len(r.retired) != 1 {
		t.Fatalf("retired tombstones=%d want=1", len(r.retired))
	}
	if out, done, err := r.pushAt(frames[2], t0.Add(carrierAssemblyTTL+time.Second)); err != nil || done || out != nil {
		t.Fatalf("second late retired fragment: out=%d done=%t err=%v", len(out), done, err)
	}
	if len(r.assemblies) != 0 {
		t.Fatalf("second late fragment resurrected %d assembly(s)", len(r.assemblies))
	}
}

func TestCarrierCompletedDatagramTombstoneDropsDuplicateFragments(t *testing.T) {
	fragmenter, err := NewCarrierFragmenter(1500)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := fragmenter.Fragment(deterministicCarrierPayload(1478))
	if err != nil {
		t.Fatal(err)
	}
	r := NewCarrierReassembler()
	t0 := time.Unix(2000, 0)
	complete := false
	for _, frame := range frames {
		_, complete, err = r.pushAt(frame, t0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !complete || len(r.assemblies) != 0 || len(r.retired) != 1 {
		t.Fatalf("completed state complete=%t assemblies=%d retired=%d", complete, len(r.assemblies), len(r.retired))
	}
	if out, done, err := r.pushAt(frames[0], t0.Add(time.Second)); err != nil || done || out != nil {
		t.Fatalf("duplicate after completion: out=%d done=%t err=%v", len(out), done, err)
	}
	if len(r.assemblies) != 0 {
		t.Fatalf("duplicate completed fragment resurrected %d assembly(s)", len(r.assemblies))
	}
}
