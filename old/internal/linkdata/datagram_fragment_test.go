package linkdata

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

func TestOffPathFragmentsOversizeDatagramAndReassemblesOutOfOrder(t *testing.T) {
	enc, err := New(offConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := New(offConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("oversize-link-datagram-"), 220)
	if len(payload) <= int(enc.Config().MTU) {
		t.Fatalf("test payload=%d must exceed mtu=%d", len(payload), enc.Config().MTU)
	}
	wire, err := enc.Encode(payload, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) < 2 {
		t.Fatalf("oversize payload emitted %d frames, want fragmentation", len(wire))
	}
	for i, frame := range wire {
		if len(frame) > int(enc.Config().MTU) {
			t.Fatalf("frame %d bytes=%d exceeds LINK MTU=%d", i, len(frame), enc.Config().MTU)
		}
	}
	var got [][]byte
	for i := len(wire) - 1; i >= 0; i-- {
		out, err := dec.decodeAt(wire[i], time.Unix(1, int64(len(wire)-i)))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, out...)
	}
	if len(got) != 1 || !bytes.Equal(got[0], payload) {
		t.Fatalf("reassembled datagrams=%d bytes=%d want=%d", len(got), func() int { if len(got) == 0 { return 0 }; return len(got[0]) }(), len(payload))
	}
	st := enc.Stats()
	if st.InnerTXPackets != 1 || st.InnerTXBytes != uint64(len(payload)) || st.WireTXPackets != uint64(len(wire)) {
		t.Fatalf("encode stats=%+v wire=%d", st, len(wire))
	}
	if st.FragmentedTXDatagrams != 1 || st.FragmentTXFrames != uint64(len(wire)) || st.MaxFragmentTXBytes > uint64(enc.Config().MTU) {
		t.Fatalf("encode fragment stats=%+v wire=%d", st, len(wire))
	}
	st = dec.Stats()
	if st.InnerRXPackets != 1 || st.InnerRXBytes != uint64(len(payload)) || st.WireRXPackets != uint64(len(wire)) {
		t.Fatalf("decode stats=%+v wire=%d", st, len(wire))
	}
	if st.FragmentRXFrames != uint64(len(wire)) || st.ReassembledRXDatagrams != 1 || st.MaxFragmentRXBytes > uint64(dec.Config().MTU) {
		t.Fatalf("decode fragment stats=%+v wire=%d", st, len(wire))
	}
}

func TestFixedPathFragmentsBeforeFECAndReassembles(t *testing.T) {
	enc, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0xa5}, 4200)
	wire, err := enc.Encode(payload, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) < 2 {
		t.Fatalf("oversize FEC payload emitted %d datagrams, want multiple systematic shards", len(wire))
	}
	var got [][]byte
	for _, frame := range wire {
		if len(frame) > int(enc.Config().MTU)+fec.HeaderSize {
			t.Fatalf("FEC wire bytes=%d exceeds mtu+header=%d", len(frame), int(enc.Config().MTU)+fec.HeaderSize)
		}
		out, err := dec.Decode(frame)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, out...)
	}
	if len(got) != 1 || !bytes.Equal(got[0], payload) {
		t.Fatalf("FEC reassembled datagrams=%d", len(got))
	}
	encStats := enc.Stats()
	if encStats.FragmentedTXDatagrams != 1 || encStats.FragmentTXFrames < 2 || encStats.MaxFragmentTXBytes > uint64(enc.Config().MTU) {
		t.Fatalf("FEC encode fragment stats=%+v", encStats)
	}
	decStats := dec.Stats()
	if decStats.FragmentRXFrames < 2 || decStats.ReassembledRXDatagrams != 1 || decStats.MaxFragmentRXBytes > uint64(dec.Config().MTU) {
		t.Fatalf("FEC decode fragment stats=%+v", decStats)
	}
}

func TestFragmentMagicPrefixIsEscapedEvenBelowMTU(t *testing.T) {
	enc, err := New(offConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := New(offConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	payload := append(append([]byte(nil), linkFragmentMagic[:]...), []byte("ordinary-payload-with-reserved-prefix")...)
	wire, err := enc.Encode(payload, time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 || bytes.Equal(wire[0], payload) {
		t.Fatalf("reserved prefix was not escaped: frames=%d", len(wire))
	}
	got, err := dec.Decode(wire[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], payload) {
		t.Fatalf("escaped payload mismatch: %q", got)
	}
}
