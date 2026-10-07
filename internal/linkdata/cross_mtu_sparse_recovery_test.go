package linkdata

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

// Models the product-facing result of a 9001-byte IPv4 datagram crossing an
// inner MTU 9000: the OS supplies one near-MTU IP fragment and one tiny trailing
// IP fragment as independent datagrams. This test then exercises the real
// LINK/FEC path. It proves the tiny tail can occupy the small FEC size class,
// while a later independent 96-byte datagram in the same FEC block is still
// first-delivered immediately. The OS fragmentation shape itself is covered by
// the privileged Linux TUN fixture.
func TestCrossInnerMTUTinyTailFECRecoveryDoesNotHOLSmallDatagram(t *testing.T) {
	tx, err := NewFECPath(FECPathConfig{SourceMTU: 1200, ParityShards: 20, FlushAfter: 8 * time.Millisecond, MaxBlocks: 8})
	if err != nil {
		t.Fatal(err)
	}
	rx, err := NewFECPath(FECPathConfig{SourceMTU: 1200, ParityShards: 20, FlushAfter: 8 * time.Millisecond, MaxBlocks: 8})
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(13000, 0)

	firstOSFragment := bytes.Repeat([]byte{0xa1}, 8996)
	tinyTail := bytes.Repeat([]byte{0xb2}, 25)
	small := bytes.Repeat([]byte{0xc3}, 96)

	firstWire, err := tx.Encode(firstOSFragment, t0)
	if err != nil {
		t.Fatal(err)
	}
	tailWire, err := tx.Encode(tinyTail, t0.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	smallWire, err := tx.Encode(small, t0.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(tailWire) != 1 || len(smallWire) != 1 {
		t.Fatalf("tiny systematic counts tail=%d small=%d", len(tailWire), len(smallWire))
	}
	tailHeader, err := fec.ParseBlockHeader(tailWire[0])
	if err != nil {
		t.Fatal(err)
	}
	smallHeader, err := fec.ParseBlockHeader(smallWire[0])
	if err != nil {
		t.Fatal(err)
	}
	if tailHeader.BlockID != smallHeader.BlockID || tailHeader.ShardIndex == smallHeader.ShardIndex {
		t.Fatalf("tail/small not sharing independent small-size FEC block: tail=%+v small=%+v", tailHeader, smallHeader)
	}

	var gotFirst [][]byte
	for _, w := range firstWire {
		out, err := rx.Decode(w, t0.Add(3*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		gotFirst = append(gotFirst, out...)
	}
	if len(gotFirst) != 1 || !bytes.Equal(gotFirst[0], firstOSFragment) {
		t.Fatalf("near-MTU OS fragment did not LINK-reassemble immediately: outputs=%d", len(gotFirst))
	}

	// Deliberately drop the tiny tail systematic source. The independent small
	// source must not wait for that missing shard, block, or OS datagram.
	smallOut, err := rx.Decode(smallWire[0], t0.Add(4*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(smallOut) != 1 || !bytes.Equal(smallOut[0], small) {
		t.Fatalf("independent small datagram HOL'd by missing tail: outputs=%d", len(smallOut))
	}

	parity, err := tx.FlushDue(t0.Add(100 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	var recovered [][]byte
	paritySeen := 0
	for _, w := range parity {
		h, err := fec.ParseBlockHeader(w)
		if err != nil {
			t.Fatal(err)
		}
		if h.BlockID != tailHeader.BlockID {
			continue
		}
		paritySeen++
		out, err := rx.Decode(w, t0.Add(101*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		recovered = append(recovered, out...)
		if len(recovered) != 0 {
			break
		}
	}
	if paritySeen == 0 || len(recovered) != 1 || !bytes.Equal(recovered[0], tinyTail) {
		t.Fatalf("tiny tail FEC recovery failed: parity_seen=%d outputs=%d", paritySeen, len(recovered))
	}
	state := rx.State()
	if state.Reassembly.Assemblies != 0 || state.Decoder.RecoveredSources == 0 {
		t.Fatalf("post recovery state=%+v", state)
	}
	t.Logf("WBD_CROSS_MTU_FEC_PREFX first_ip_fragment_bytes=8996 tiny_tail_bytes=25 small_bytes=96 small_first_delivery_ms=4 tail_fec_recovery_ms=101 recovered_sources=%d",
		state.Decoder.RecoveredSources)
}
