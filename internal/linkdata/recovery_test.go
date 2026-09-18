package linkdata

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

func testStreamingBlock(t *testing.T, blockID uint32) ([][]byte, [][]byte) {
	t.Helper()
	codec := fec.NewFastReedSolomon20x20()
	enc, err := fec.NewFastBlockEncoder(codec, 1400, time.Millisecond, blockID)
	if err != nil {
		t.Fatal(err)
	}
	want := make([][]byte, fec.DataShards)
	sources := make([][]byte, 0, fec.DataShards)
	base := time.Unix(1, 0)
	for i := 0; i < fec.DataShards; i++ {
		want[i] = bytes.Repeat([]byte{byte(i + 1)}, 80+i)
		out, err := enc.Add(want[i], base.Add(time.Duration(i)*time.Microsecond))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) == 0 {
			t.Fatalf("source %d emitted no systematic datagram", i)
		}
		sources = append(sources, append([]byte(nil), out[0]...))
	}
	return want, sources
}

func testStreamingBlockWithParity(t *testing.T, blockID uint32) ([][]byte, [][]byte) {
	t.Helper()
	codec := fec.NewFastReedSolomon20x20()
	enc, err := fec.NewFastBlockEncoder(codec, 1400, time.Millisecond, blockID)
	if err != nil {
		t.Fatal(err)
	}
	want := make([][]byte, fec.DataShards)
	wire := make([][]byte, 0, fec.TotalShards)
	base := time.Unix(1, 0)
	for i := 0; i < fec.DataShards; i++ {
		want[i] = bytes.Repeat([]byte{byte(i + 1)}, 80+i)
		out, err := enc.Add(want[i], base.Add(time.Duration(i)*time.Microsecond))
		if err != nil {
			t.Fatal(err)
		}
		for _, datagram := range out {
			wire = append(wire, append([]byte(nil), datagram...))
		}
	}
	if len(wire) != fec.TotalShards {
		t.Fatalf("wire=%d want=%d", len(wire), fec.TotalShards)
	}
	return want, wire
}

func TestHighLatencyRecoveryWindowAllowsLateParityAtTwoPointFiveSeconds(t *testing.T) {
	p, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	want, wire := testStreamingBlockWithParity(t, 66)
	t0 := time.Unix(90, 0)

	got, err := p.decodeAt(wire[0], t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], want[0]) {
		t.Fatalf("first delivery=%q", got)
	}

	late := t0.Add(2500 * time.Millisecond)
	recovered := make([][]byte, 0, fec.DataShards-1)
	for i := fec.DataShards; i < fec.DataShards+fec.DataShards-1; i++ {
		got, err = p.decodeAt(wire[i], late.Add(time.Duration(i-fec.DataShards)*time.Microsecond))
		if err != nil {
			t.Fatalf("late parity %d: %v", i-fec.DataShards, err)
		}
		recovered = append(recovered, got...)
	}
	if len(recovered) != fec.DataShards-1 {
		t.Fatalf("late parity recovered=%d want=%d", len(recovered), fec.DataShards-1)
	}
	for i, packet := range recovered {
		if !bytes.Equal(packet, want[i+1]) {
			t.Fatalf("recovered source %d mismatch", i+1)
		}
	}
	st := p.FECObserveStats()
	if st.Recovery.ExpireEvents != 0 || st.Decoder.InFlight != 0 {
		t.Fatalf("late parity recovery stats=%+v", st)
	}
}

func TestBoundedRecoveryExpiresWithoutInputAndLateSystematicDeliversOnce(t *testing.T) {
	p, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	want, sources := testStreamingBlock(t, 77)
	t0 := time.Unix(100, 0)

	got, err := p.decodeAt(sources[0], t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], want[0]) {
		t.Fatalf("first delivery=%q", got)
	}
	if st := p.FECObserveStats(); st.Decoder.InFlight != 1 || st.Recovery.PendingDeadlines != 1 {
		t.Fatalf("before expiry stats=%+v", st)
	}

	if _, err := p.FlushDue(t0.Add(candidateFECRecoveryHorizon - time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if st := p.FECObserveStats(); st.Decoder.InFlight != 1 || st.Recovery.ExpireEvents != 0 {
		t.Fatalf("expired early stats=%+v", st)
	}

	// No inbound shard arrives here. The existing FlushDue timer drives expiry.
	if _, err := p.FlushDue(t0.Add(candidateFECRecoveryHorizon)); err != nil {
		t.Fatal(err)
	}
	st := p.FECObserveStats()
	if st.Decoder.InFlight != 0 || st.Decoder.Retired != 1 ||
		st.Recovery.ExpireEvents != 1 || st.Recovery.ExpiredIncomplete != 1 ||
		st.Recovery.ExpiredMissingSources != fec.DataShards-1 || st.Recovery.PendingDeadlines != 0 {
		t.Fatalf("after expiry stats=%+v", st)
	}

	late := t0.Add(candidateFECRecoveryHorizon + time.Millisecond)
	got, err = p.decodeAt(sources[1], late)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !bytes.Equal(got[0], want[1]) {
		t.Fatalf("late first delivery=%q", got)
	}
	got, err = p.decodeAt(sources[1], late.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("late duplicate delivered %d packets", len(got))
	}
	if st := p.FECObserveStats(); st.Decoder.InFlight != 0 {
		t.Fatalf("expired block resurrected heavy: %+v", st)
	}
}

func TestBoundedRecoveryAbsoluteDeadlineDoesNotRefreshOnProgress(t *testing.T) {
	p, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	_, sources := testStreamingBlock(t, 88)
	t0 := time.Unix(200, 0)
	if _, err := p.decodeAt(sources[0], t0); err != nil {
		t.Fatal(err)
	}
	if _, err := p.decodeAt(sources[0], t0.Add(1500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.decodeAt(sources[1], t0.Add(candidateFECRecoveryHorizon-10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.FlushDue(t0.Add(candidateFECRecoveryHorizon)); err != nil {
		t.Fatal(err)
	}
	st := p.FECObserveStats()
	if st.Decoder.InFlight != 0 || st.Recovery.ExpireEvents != 1 {
		t.Fatalf("progress refreshed absolute deadline: %+v", st)
	}
}

func TestBoundedRecoveryBlockIDWrap(t *testing.T) {
	p, err := New(fixedConfig(), 64)
	if err != nil {
		t.Fatal(err)
	}
	_, maxSources := testStreamingBlock(t, ^uint32(0))
	_, zeroSources := testStreamingBlock(t, 0)
	t0 := time.Unix(300, 0)
	if _, err := p.decodeAt(maxSources[0], t0); err != nil {
		t.Fatal(err)
	}
	if _, err := p.decodeAt(zeroSources[0], t0.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.FlushDue(t0.Add(candidateFECRecoveryHorizon + 2*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	st := p.FECObserveStats()
	if st.Decoder.InFlight != 0 || st.Recovery.ExpireEvents != 2 || st.Recovery.PendingDeadlines != 0 {
		t.Fatalf("wrap expiry stats=%+v", st)
	}
}

func TestRecoveryDeadlineQueueCompactsAfterExpiry(t *testing.T) {
	r := newFECRecoveryTracker(time.Second)
	base := time.Unix(400, 0)
	for i := 0; i < 4096; i++ {
		r.observeHeavy(uint32(i), base.Add(time.Duration(i)*time.Microsecond))
	}
	dec, err := fec.NewBlockDecoder(fec.NewFastReedSolomon20x20(), 1400, 1)
	if err != nil {
		t.Fatal(err)
	}
	r.expire(dec, base.Add(2*time.Second))
	if len(r.seen) != 0 || r.head != 0 || len(r.queue) != 0 {
		t.Fatalf("deadline queue retained stale history seen=%d head=%d queue=%d", len(r.seen), r.head, len(r.queue))
	}
}
