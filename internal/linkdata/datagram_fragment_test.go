package linkdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func TestFragmentWireFieldsPacketIDAndLengthBounds(t *testing.T) {
	f, err := NewFragmenter(64)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0xa5}, 100)
	frames, err := f.Fragment(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("frames=%d want=3", len(frames))
	}
	for i, frame := range frames {
		if len(frame) > f.MTU() {
			t.Fatalf("frame %d bytes=%d mtu=%d", i, len(frame), f.MTU())
		}
		if string(frame[:8]) != "WBDLFRG1" || frame[8] != 1 || frame[9] != 0 {
			t.Fatalf("frame %d header=%x", i, frame[:10])
		}
		if got := binary.BigEndian.Uint16(frame[10:12]); got != uint16(i) {
			t.Fatalf("frame %d index=%d", i, got)
		}
		if got := binary.BigEndian.Uint16(frame[12:14]); got != uint16(len(frames)) {
			t.Fatalf("frame %d count=%d", i, got)
		}
		if got := binary.BigEndian.Uint16(frame[14:16]); got != uint16(len(payload)) {
			t.Fatalf("frame %d total=%d", i, got)
		}
		if got := binary.BigEndian.Uint32(frame[16:20]); got != 1 {
			t.Fatalf("frame %d packet_id=%d", i, got)
		}
	}

	ordinary := []byte("ordinary")
	pass, err := f.Fragment(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if len(pass) != 1 || !bytes.Equal(pass[0], ordinary) {
		t.Fatalf("ordinary pass-through=%q", pass)
	}

	if _, err := f.Fragment(nil); !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("empty error=%v", err)
	}
	if _, err := f.Fragment(make([]byte, MaxDatagramLen+1)); !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("oversize error=%v", err)
	}
	if _, err := NewFragmenter(FragmentHeaderLen); !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("small mtu error=%v", err)
	}
	if _, err := NewFragmenter(MaxDatagramLen + 1); !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("large mtu error=%v", err)
	}
}

func TestReservedMagicEscapeAndPacketIDWrap(t *testing.T) {
	f, err := NewFragmenter(128)
	if err != nil {
		t.Fatal(err)
	}
	f.nextID = math.MaxUint32
	payload := append(append([]byte(nil), fragmentMagic[:]...), []byte("reserved-prefix")...)
	first, err := f.Fragment(payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.Fragment(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint32(first[0][16:20]); got != math.MaxUint32 {
		t.Fatalf("first packet_id=%d", got)
	}
	if got := binary.BigEndian.Uint32(second[0][16:20]); got != 1 {
		t.Fatalf("wrapped packet_id=%d want=1", got)
	}

	r, err := NewReassembler(128)
	if err != nil {
		t.Fatal(err)
	}
	got, complete, err := r.Push(first[0], time.Unix(1, 0))
	if err != nil || !complete || !bytes.Equal(got, payload) {
		t.Fatalf("escaped reassembly complete=%v err=%v got=%q", complete, err, got)
	}
}

func TestReassemblyOutOfOrderAndIdenticalDuplicate(t *testing.T) {
	f, _ := NewFragmenter(48)
	r, _ := NewReassembler(48)
	payload := bytes.Repeat([]byte("fragment-order-"), 10)
	frames, err := f.Fragment(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) < 3 {
		t.Fatalf("frames=%d want>=3", len(frames))
	}

	now := time.Unix(10, 0)
	if got, complete, err := r.Push(frames[len(frames)-1], now); err != nil || complete || got != nil {
		t.Fatalf("first out-of-order complete=%v err=%v got=%x", complete, err, got)
	}
	before := r.State()
	if got, complete, err := r.Push(frames[len(frames)-1], now); err != nil || complete || got != nil {
		t.Fatalf("duplicate complete=%v err=%v got=%x", complete, err, got)
	}
	if after := r.State(); after.BufferedFragments != before.BufferedFragments || after.BufferedBytes != before.BufferedBytes {
		t.Fatalf("duplicate changed state before=%+v after=%+v", before, after)
	}

	var got []byte
	for i := len(frames) - 2; i >= 0; i-- {
		out, complete, err := r.Push(frames[i], now)
		if err != nil {
			t.Fatal(err)
		}
		if complete {
			got = out
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("reassembled bytes=%d want=%d", len(got), len(payload))
	}
	if st := r.State(); st.Assemblies != 0 || st.BufferedFragments != 0 || st.BufferedBytes != 0 || st.RetiredPacketIDs != 1 {
		t.Fatalf("post-complete state=%+v", st)
	}
}

func TestConflictingDuplicateRejectedWithoutReplacingFirstArrival(t *testing.T) {
	f, _ := NewFragmenter(48)
	r, _ := NewReassembler(48)
	payload := bytes.Repeat([]byte{0x3c}, 100)
	frames, err := f.Fragment(payload)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(20, 0)
	if _, complete, err := r.Push(frames[0], now); err != nil || complete {
		t.Fatalf("first fragment complete=%v err=%v", complete, err)
	}
	conflict := append([]byte(nil), frames[0]...)
	conflict[len(conflict)-1] ^= 0xff
	if _, _, err := r.Push(conflict, now); !errors.Is(err, ErrConflictingFragment) {
		t.Fatalf("conflicting duplicate error=%v", err)
	}

	var got []byte
	for _, frame := range frames[1:] {
		out, complete, err := r.Push(frame, now)
		if err != nil {
			t.Fatal(err)
		}
		if complete {
			got = out
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("conflict replaced first arrival: got=%x want=%x", got, payload)
	}
}

func TestMissingDatagramFragmentDoesNotBlockLaterCompleteDatagram(t *testing.T) {
	f, _ := NewFragmenter(48)
	r, _ := NewReassembler(48)
	a := bytes.Repeat([]byte("A"), 100)
	b := bytes.Repeat([]byte("B"), 90)
	aFrames, _ := f.Fragment(a)
	bFrames, _ := f.Fragment(b)
	if len(aFrames) < 3 || len(bFrames) < 3 {
		t.Fatalf("A frames=%d B frames=%d", len(aFrames), len(bFrames))
	}
	now := time.Unix(30, 0)

	for i, frame := range aFrames {
		if i == 1 {
			continue
		}
		if _, complete, err := r.Push(frame, now); err != nil || complete {
			t.Fatalf("A fragment %d complete=%v err=%v", i, complete, err)
		}
	}

	var gotB []byte
	for _, frame := range bFrames {
		out, complete, err := r.Push(frame, now)
		if err != nil {
			t.Fatal(err)
		}
		if complete {
			gotB = out
		}
	}
	if !bytes.Equal(gotB, b) {
		t.Fatalf("B did not complete while A had a permanent hole: bytes=%d", len(gotB))
	}
	if st := r.State(); st.Assemblies != 1 {
		t.Fatalf("A should remain incomplete while B is delivered: %+v", st)
	}

	gotA, complete, err := r.Push(aFrames[1], now)
	if err != nil || !complete || !bytes.Equal(gotA, a) {
		t.Fatalf("A later completion complete=%v err=%v bytes=%d", complete, err, len(gotA))
	}
}

func TestExplicitExpiryRetiresIncompleteDatagramWithoutNewTraffic(t *testing.T) {
	f, _ := NewFragmenter(48)
	r, _ := NewReassembler(48)
	payload := bytes.Repeat([]byte{0x44}, 100)
	frames, _ := f.Fragment(payload)
	t0 := time.Unix(40, 0)
	if _, complete, err := r.Push(frames[0], t0); err != nil || complete {
		t.Fatalf("initial complete=%v err=%v", complete, err)
	}

	r.Expire(t0.Add(AssemblyTTL))
	if st := r.State(); st.Assemblies != 0 || st.BufferedFragments != 0 || st.BufferedBytes != 0 || st.RetiredPacketIDs != 1 {
		t.Fatalf("expired state=%+v", st)
	}
	if got, complete, err := r.Push(frames[1], t0.Add(AssemblyTTL+time.Millisecond)); err != nil || complete || got != nil {
		t.Fatalf("stale retired fragment complete=%v err=%v got=%x", complete, err, got)
	}
	if got, complete, err := r.Push([]byte("later-ok"), t0.Add(AssemblyTTL+time.Millisecond)); err != nil || !complete || string(got) != "later-ok" {
		t.Fatalf("later ordinary complete=%v err=%v got=%q", complete, err, got)
	}
}

func TestReassemblyStateAndMalformedInputAreBoundedAndIsolated(t *testing.T) {
	f, _ := NewFragmenter(48)
	r, _ := NewReassembler(48)
	payload := bytes.Repeat([]byte{0x77}, 100)
	frames, _ := f.Fragment(payload)
	base := frames[0]
	t0 := time.Unix(50, 0)

	for id := uint32(1); id <= MaxReassemblyAssemblies+8; id++ {
		frame := append([]byte(nil), base...)
		binary.BigEndian.PutUint32(frame[16:20], id)
		if _, complete, err := r.Push(frame, t0.Add(time.Duration(id)*time.Nanosecond)); err != nil || complete {
			t.Fatalf("id=%d complete=%v err=%v", id, complete, err)
		}
		st := r.State()
		if st.Assemblies > MaxReassemblyAssemblies || st.BufferedFragments > MaxBufferedFragments || st.BufferedBytes > MaxBufferedBytes || st.RetiredPacketIDs > MaxRetiredPacketIDs {
			t.Fatalf("unbounded state id=%d: %+v", id, st)
		}
	}

	oversize := make([]byte, 49)
	copy(oversize, fragmentMagic[:])
	if _, _, err := r.Push(oversize, t0); !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("oversize frame error=%v", err)
	}
	bad := append([]byte(nil), base...)
	binary.BigEndian.PutUint16(bad[12:14], 0)
	if _, _, err := r.Push(bad, t0); !errors.Is(err, ErrInvalidFragment) {
		t.Fatalf("bad count error=%v", err)
	}
	if got, complete, err := r.Push([]byte("healthy-after-bad"), t0); err != nil || !complete || string(got) != "healthy-after-bad" {
		t.Fatalf("healthy after malformed complete=%v err=%v got=%q", complete, err, got)
	}
}

func TestMaximumFragmentCountMetadataDoesNotPreallocateDeclaredCount(t *testing.T) {
	r, _ := NewReassembler(64)
	frame := make([]byte, FragmentHeaderLen+1)
	copy(frame[:8], fragmentMagic[:])
	frame[8] = 1
	binary.BigEndian.PutUint16(frame[10:12], uint16(MaxFragments-1))
	binary.BigEndian.PutUint16(frame[12:14], uint16(MaxFragments))
	binary.BigEndian.PutUint16(frame[14:16], uint16(MaxDatagramLen))
	binary.BigEndian.PutUint32(frame[16:20], 99)
	frame[20] = 0x01
	if _, complete, err := r.Push(frame, time.Unix(60, 0)); err != nil || complete {
		t.Fatalf("max-count metadata complete=%v err=%v", complete, err)
	}
	if st := r.State(); st.Assemblies != 1 || st.BufferedFragments != 1 || st.BufferedBytes != 1 {
		t.Fatalf("max-count metadata allocated unexpected state: %+v", st)
	}
}
