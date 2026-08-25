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
	got, err := p.Decode(wire[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], packet) {
		t.Fatalf("got=%q", got)
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
}

func TestResearchProfileCannotBecomeLivePath(t *testing.T) {
	cfg := fixedConfig()
	cfg.Scheduler = control.FECSchedulerCausal
	if _, err := New(cfg, 64); err == nil {
		t.Fatal("causal research profile unexpectedly admitted as live path")
	}
}
