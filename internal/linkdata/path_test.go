package linkdata

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
)

func offConfig() control.LinkConfig {
	return control.LinkConfig{FECMode: control.FECOff, Scheduler: control.FECSchedulerNone, LaneCount: 1, MTU: 1400}
}

func fixedConfig() control.LinkConfig {
	return control.LinkConfig{
		FECMode: control.FECFixed, Scheduler: control.FECSchedulerTailRS,
		DataShards: 20, ParityShards: 20, LaneCount: 1,
		FlushMillis: 8, MTU: 1400,
	}
}

func TestOffPathIsPacketPreservingZeroWait(t *testing.T) {
	p, err := New(offConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	packet := []byte("one complete datagram")
	wire, err := p.Encode(packet, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 || !bytes.Equal(wire[0], packet) {
		t.Fatalf("wire=%q", wire)
	}
	got, err := p.Decode(wire[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], packet) {
		t.Fatalf("got=%q", got)
	}
	if flush, err := p.FlushDue(time.Unix(2, 0)); err != nil || len(flush) != 0 {
		t.Fatalf("off flush=%d err=%v", len(flush), err)
	}
	st := p.Stats()
	if st.InnerTXPackets != 1 || st.InnerTXBytes != uint64(len(packet)) ||
		st.WireTXPackets != 1 || st.WireTXBytes != uint64(len(packet)) ||
		st.FECSystematicTXPackets != 0 || st.FECRepairTXPackets != 0 ||
		st.WireRXPackets != 1 || st.WireRXBytes != uint64(len(packet)) ||
		st.InnerRXPackets != 1 || st.InnerRXBytes != uint64(len(packet)) {
		t.Fatalf("off stats=%+v", st)
	}
}

func TestFixedPathStreamsSystematicImmediately(t *testing.T) {
	p, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	packet := []byte("first source")
	wire, err := p.Encode(packet, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 1 {
		t.Fatalf("first source emitted %d datagrams, want one immediate systematic", len(wire))
	}
	st := p.Stats()
	if st.InnerTXPackets != 1 || st.WireTXPackets != 1 || st.FECSystematicTXPackets != 1 || st.FECRepairTXPackets != 0 {
		t.Fatalf("streaming stats=%+v", st)
	}
	got, err := p.Decode(wire[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], packet) {
		t.Fatalf("got=%q", got)
	}
	st = p.Stats()
	if st.WireRXPackets != 1 || st.InnerRXPackets != 1 || st.InnerRXBytes != uint64(len(packet)) {
		t.Fatalf("decode stats=%+v", st)
	}
	repair, err := p.FlushDue(time.Unix(1, int64(8*time.Millisecond)))
	if err != nil {
		t.Fatal(err)
	}
	if len(repair) != 20 {
		t.Fatalf("partial repair=%d want=20", len(repair))
	}
	st = p.Stats()
	if st.WireTXPackets != 21 || st.FECSystematicTXPackets != 1 || st.FECRepairTXPackets != 20 {
		t.Fatalf("partial flush stats=%+v", st)
	}
}

func TestFixedPathRecoversMissingSourceFromParity(t *testing.T) {
	enc, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1, 0)
	var wire [][]byte
	var want [][]byte
	for i := 0; i < 20; i++ {
		packet := bytes.Repeat([]byte{byte(i + 1)}, 80+i)
		want = append(want, packet)
		out, err := enc.Encode(packet, now.Add(time.Duration(i)*time.Microsecond))
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, out...)
	}
	if len(wire) != 40 {
		t.Fatalf("wire=%d want=40", len(wire))
	}
	encStats := enc.Stats()
	if encStats.InnerTXPackets != 20 || encStats.WireTXPackets != 40 ||
		encStats.FECSystematicTXPackets != 20 || encStats.FECRepairTXPackets != 20 {
		t.Fatalf("encoder stats=%+v", encStats)
	}

	// Drop source shard 7. Feed every other source and enough parity. Surviving
	// sources may be returned immediately; the missing original must appear once
	// the decoder has 20 independent shards.
	seen := make(map[string]bool)
	for i, shard := range wire {
		if i == 7 {
			continue
		}
		got, err := dec.Decode(shard)
		if err != nil {
			t.Fatal(err)
		}
		for _, packet := range got {
			seen[string(packet)] = true
		}
	}
	if !seen[string(want[7])] {
		t.Fatal("missing systematic source was not recovered from repair shards")
	}
	decStats := dec.Stats()
	if decStats.WireRXPackets != 39 || decStats.InnerRXPackets != 20 {
		t.Fatalf("decoder stats=%+v", decStats)
	}
}

func TestResearchProfileCannotBecomeLivePath(t *testing.T) {
	cfg := fixedConfig()
	cfg.Scheduler = control.FECSchedulerCausal
	if _, err := New(cfg, 64); err == nil {
		t.Fatal("causal research profile unexpectedly admitted as live path")
	}
}
