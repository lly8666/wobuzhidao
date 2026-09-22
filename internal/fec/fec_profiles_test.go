package fec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"time"
)

func profilePackets() [][]byte {
	out := make([][]byte, DataShards)
	for i := range out {
		out[i] = bytes.Repeat([]byte{byte(i + 1)}, 80+i*3)
	}
	return out
}

func profileWire(t *testing.T, parity int, blockID uint32) (want, sources, repairs [][]byte) {
	t.Helper()
	enc, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, time.Hour, blockID, parity)
	if err != nil {
		t.Fatal(err)
	}
	want = profilePackets()
	sources = make([][]byte, 0, DataShards)
	for i, packet := range want {
		out, err := enc.Add(packet, time.Unix(1, int64(i)))
		if err != nil {
			t.Fatalf("20:%d source %d: %v", parity, i, err)
		}
		if len(out) == 0 {
			t.Fatalf("20:%d source %d emitted nothing", parity, i)
		}
		sources = append(sources, append([]byte(nil), out[0]...))
		for _, item := range out[1:] {
			repairs = append(repairs, append([]byte(nil), item...))
		}
	}
	if len(sources) != DataShards || len(repairs) != parity {
		t.Fatalf("20:%d sources=%d repairs=%d", parity, len(sources), len(repairs))
	}
	return want, sources, repairs
}

func TestSupportedParityShardsExactLiveMatrix(t *testing.T) {
	want := []int{4, 8, 10, 12, 16, 20}
	if got := SupportedParityShards(); !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles=%v want=%v", got, want)
	}
	for _, parity := range want {
		if !IsSupportedParityShards(parity) {
			t.Fatalf("20:%d not supported", parity)
		}
	}
	for _, parity := range []int{-1, 0, 1, 9, 11, 19, 21} {
		if IsSupportedParityShards(parity) {
			t.Fatalf("unexpected profile 20:%d", parity)
		}
	}
}

func TestStreamingV1WireCarriesEveryLiveParityProfile(t *testing.T) {
	for _, parity := range SupportedParityShards() {
		t.Run("20x"+itoaSmall(parity), func(t *testing.T) {
			enc, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, time.Second, 0x01020304, parity)
			if err != nil {
				t.Fatal(err)
			}
			out, err := enc.Add([]byte("abc"), time.Unix(1, 0))
			if err != nil || len(out) != 1 {
				t.Fatalf("out=%d err=%v", len(out), err)
			}
			wire := out[0]
			if len(wire) != HeaderSize+3 {
				t.Fatalf("wire=%d", len(wire))
			}
			wantHeader := make([]byte, HeaderSize)
			wantHeader[0], wantHeader[1], wantHeader[2] = 'W', 'F', HeaderVersion
			binary.BigEndian.PutUint32(wantHeader[4:8], 0x01020304)
			wantHeader[8] = 0
			wantHeader[9] = DataShards
			wantHeader[10] = byte(parity)
			wantHeader[11] = DataShards
			binary.BigEndian.PutUint16(wantHeader[12:14], 3)
			binary.BigEndian.PutUint16(wantHeader[14:16], headerFlagStreamingSystematic)
			binary.BigEndian.PutUint16(wantHeader[16:18], 3)
			if !bytes.Equal(wire[:HeaderSize], wantHeader) {
				t.Fatalf("20:%d header=%x want=%x", parity, wire[:HeaderSize], wantHeader)
			}
			if !bytes.Equal(wire[HeaderSize:], []byte("abc")) {
				t.Fatalf("payload=%q", wire[HeaderSize:])
			}
			h, err := ParseBlockHeader(wire)
			if err != nil || h.EffectiveParityCount() != parity {
				t.Fatalf("parsed=%+v err=%v", h, err)
			}
		})
	}

	enc, _ := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, time.Second, 1, 10)
	out, _ := enc.Add([]byte("reserved"), time.Unix(1, 0))
	bad := append([]byte(nil), out[0]...)
	bad[3] = 1
	if _, err := ParseBlockHeader(bad); err == nil {
		t.Fatal("reserved header byte accepted")
	}
}

func TestEveryLiveProfileSystematicFirstArrivalAndFullRecovery(t *testing.T) {
	for _, parity := range SupportedParityShards() {
		t.Run("20x"+itoaSmall(parity), func(t *testing.T) {
			want, sources, repairs := profileWire(t, parity, 100)
			dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 8, parity)
			if err != nil {
				t.Fatal(err)
			}

			got := make(map[byte][]byte)
			done := false
			// Parity may arrive before surviving systematic sources.
			for i := len(repairs) - 1; i >= 0; i-- {
				packets, blockDone, err := dec.Add(repairs[i])
				if err != nil {
					t.Fatalf("repair %d: %v", i, err)
				}
				for _, packet := range packets {
					got[packet[0]] = packet
				}
				done = done || blockDone
			}
			// Lose exactly R systematic sources; reverse the remaining source order.
			for i := DataShards - 1; i >= parity; i-- {
				packets, blockDone, err := dec.Add(sources[i])
				if err != nil {
					t.Fatalf("source %d: %v", i, err)
				}
				for _, packet := range packets {
					got[packet[0]] = packet
				}
				done = done || blockDone
			}
			if !done || dec.InFlight() != 0 {
				t.Fatalf("20:%d done=%v inflight=%d", parity, done, dec.InFlight())
			}
			if len(got) != DataShards {
				t.Fatalf("20:%d delivered=%d want=%d", parity, len(got), DataShards)
			}
			for _, packet := range want {
				if actual := got[packet[0]]; !bytes.Equal(actual, packet) {
					t.Fatalf("20:%d packet %d mismatch", parity, packet[0])
				}
			}
		})
	}
}

func TestEveryLiveProfilePartialBlockEmitsOnlyUsefulParity(t *testing.T) {
	for _, parity := range SupportedParityShards() {
		t.Run("20x"+itoaSmall(parity), func(t *testing.T) {
			enc, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, 8*time.Millisecond, 7, parity)
			if err != nil {
				t.Fatal(err)
			}
			t0 := time.Unix(10, 0)
			for i, packet := range [][]byte{[]byte("alpha"), []byte("bravo"), []byte("charlie")} {
				out, err := enc.Add(packet, t0.Add(time.Duration(i)*time.Millisecond))
				if err != nil || len(out) != 1 {
					t.Fatalf("add %d out=%d err=%v", i, len(out), err)
				}
			}
			if out, err := enc.FlushDue(t0.Add(7 * time.Millisecond)); err != nil || len(out) != 0 {
				t.Fatalf("early flush out=%d err=%v", len(out), err)
			}
			repairs, err := enc.FlushDue(t0.Add(8 * time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			if len(repairs) != 3 {
				t.Fatalf("20:%d partial repairs=%d want=3", parity, len(repairs))
			}
			for i, wire := range repairs {
				h, err := ParseBlockHeader(wire)
				if err != nil {
					t.Fatal(err)
				}
				if h.EffectiveParityCount() != parity || h.DataCount != 3 || int(h.ShardIndex) != DataShards+i {
					t.Fatalf("20:%d repair %d header=%+v", parity, i, h)
				}
			}
		})
	}
}

func TestEveryLiveProfileMissingBlockDoesNotBlockNewSource(t *testing.T) {
	const missing = 7
	for _, parity := range SupportedParityShards() {
		t.Run("20x"+itoaSmall(parity), func(t *testing.T) {
			wantA, sourcesA, repairsA := profileWire(t, parity, 1)
			wantB, sourcesB, _ := profileWire(t, parity, 2)
			dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 1, parity)
			if err != nil {
				t.Fatal(err)
			}
			for i, wire := range sourcesA {
				if i == missing {
					continue
				}
				packets, done, err := dec.Add(wire)
				if err != nil || done || len(packets) != 1 || !bytes.Equal(packets[0], wantA[i]) {
					t.Fatalf("A source %d packets=%d done=%v err=%v", i, len(packets), done, err)
				}
			}
			if dec.InFlight() != 1 {
				t.Fatalf("A inflight=%d", dec.InFlight())
			}

			// B arrives while A still has a permanent-looking hole. It must
			// first-deliver immediately instead of waiting for A.
			packets, done, err := dec.Add(sourcesB[0])
			if err != nil || done || len(packets) != 1 || !bytes.Equal(packets[0], wantB[0]) {
				t.Fatalf("B first arrival packets=%d done=%v err=%v", len(packets), done, err)
			}
			if dec.InFlight() != 1 {
				t.Fatalf("B pressure changed heavy slots=%d", dec.InFlight())
			}

			packets, done, err = dec.Add(repairsA[0])
			if err != nil || !done || len(packets) != 1 || !bytes.Equal(packets[0], wantA[missing]) {
				t.Fatalf("A repair packets=%d done=%v err=%v", len(packets), done, err)
			}
		})
	}
}

func TestDuplicateShardIdempotentAndConflictingDuplicateRejected(t *testing.T) {
	_, sources, repairs := profileWire(t, 10, 9)
	dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 4, 10)
	if err != nil {
		t.Fatal(err)
	}
	packets, done, err := dec.Add(sources[0])
	if err != nil || done || len(packets) != 1 {
		t.Fatalf("first packets=%d done=%v err=%v", len(packets), done, err)
	}
	packets, done, err = dec.Add(sources[0])
	if err != nil || done || len(packets) != 0 {
		t.Fatalf("duplicate packets=%d done=%v err=%v", len(packets), done, err)
	}
	badSource := append([]byte(nil), sources[0]...)
	badSource[len(badSource)-1] ^= 0xff
	if _, _, err := dec.Add(badSource); !errors.Is(err, ErrShardMismatch) {
		t.Fatalf("source conflict err=%v", err)
	}

	dec2, _ := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 4, 10)
	if packets, done, err = dec2.Add(repairs[0]); err != nil || done || len(packets) != 0 {
		t.Fatalf("repair first packets=%d done=%v err=%v", len(packets), done, err)
	}
	if packets, done, err = dec2.Add(repairs[0]); err != nil || done || len(packets) != 0 {
		t.Fatalf("repair duplicate packets=%d done=%v err=%v", len(packets), done, err)
	}
	badRepair := append([]byte(nil), repairs[0]...)
	badRepair[len(badRepair)-1] ^= 0xff
	if _, _, err := dec2.Add(badRepair); !errors.Is(err, ErrShardMismatch) {
		t.Fatalf("repair conflict err=%v", err)
	}
}

func TestProfileHeaderMismatchDoesNotPoisonFollowingHealthyShard(t *testing.T) {
	_, sources8, _ := profileWire(t, 8, 12)
	want10, sources10, _ := profileWire(t, 10, 12)
	dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 4, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := dec.Add(sources8[0]); !errors.Is(err, ErrHeaderMismatch) {
		t.Fatalf("wrong profile err=%v", err)
	}
	if dec.InFlight() != 0 {
		t.Fatalf("wrong profile created state=%d", dec.InFlight())
	}
	packets, done, err := dec.Add(sources10[0])
	if err != nil || done || len(packets) != 1 || !bytes.Equal(packets[0], want10[0]) {
		t.Fatalf("healthy after mismatch packets=%d done=%v err=%v", len(packets), done, err)
	}
}

func TestRetiredStateHardBound(t *testing.T) {
	dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	const extra = 17
	for id := uint32(1); id <= uint32(maxRetiredBlocks+extra); id++ {
		dec.addRetiredState(id, retiredBlock{})
		if len(dec.retired) > maxRetiredBlocks {
			t.Fatalf("retired=%d cap=%d", len(dec.retired), maxRetiredBlocks)
		}
	}
	if got := len(dec.retired); got != maxRetiredBlocks {
		t.Fatalf("retired=%d want=%d", got, maxRetiredBlocks)
	}
	for id := uint32(1); id <= extra; id++ {
		if !dec.completed.contains(id) {
			t.Fatalf("aged retired id %d not completed", id)
		}
	}
}

func TestCompletedBlockSetCompactsSequentialHistory(t *testing.T) {
	var set completedBlockSet
	set.add(2)
	set.add(4)
	set.add(3)
	set.add(1)
	if set.through != 4 || len(set.ranges) != 0 {
		t.Fatalf("through=%d ranges=%v", set.through, set.ranges)
	}
	for id := uint32(5); id <= 100000; id++ {
		set.add(id)
	}
	if set.through != 100000 || len(set.ranges) != 0 {
		t.Fatalf("through=%d ranges=%d", set.through, len(set.ranges))
	}
}

func TestInvalidFixedProfileRejected(t *testing.T) {
	codec := NewFastReedSolomon20x20()
	for _, parity := range []int{1, 9, 11, 19, 21} {
		if _, err := NewFastBlockEncoderWithParity(codec, 1400, time.Millisecond, 1, parity); err == nil {
			t.Fatalf("encoder accepted 20:%d", parity)
		}
		if _, err := NewBlockDecoderWithParity(codec, 1400, 4, parity); err == nil {
			t.Fatalf("decoder accepted 20:%d", parity)
		}
	}
}

func TestFastCodecMatchesReferenceParity(t *testing.T) {
	ref := NewReedSolomon20x20()
	fast := NewFastReedSolomon20x20()
	a := make([][]byte, TotalShards)
	b := make([][]byte, TotalShards)
	for i := 0; i < TotalShards; i++ {
		a[i] = make([]byte, 256)
		b[i] = make([]byte, 256)
	}
	for i := 0; i < DataShards; i++ {
		for j := range a[i] {
			a[i][j] = byte((i*31 + j*7) & 0xff)
			b[i][j] = a[i][j]
		}
	}
	if err := ref.Encode(a); err != nil {
		t.Fatal(err)
	}
	if err := fast.Encode(b); err != nil {
		t.Fatal(err)
	}
	for i := DataShards; i < TotalShards; i++ {
		if !bytes.Equal(a[i], b[i]) {
			t.Fatalf("parity shard %d mismatch", i)
		}
	}
}

func itoaSmall(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string([]byte{byte('0' + n/10), byte('0' + n%10)})
}


func TestFastBlockEncoderStatsInventory(t *testing.T) {
	enc, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, 8*time.Millisecond, 1, WeakParityShards)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Unix(100, 0)
	for i := 0; i < DataShards; i++ {
		out, err := enc.Add([]byte{byte(i + 1)}, t0.Add(time.Duration(i)*time.Microsecond))
		if err != nil {
			t.Fatal(err)
		}
		if i == DataShards-1 {
			if len(out) != 1+WeakParityShards {
				t.Fatalf("full flush records=%d want=%d", len(out), 1+WeakParityShards)
			}
		} else if len(out) != 1 {
			t.Fatalf("source %d records=%d want=1", i, len(out))
		}
	}
	stats := enc.Stats()
	if stats.SourceShards != DataShards || stats.ParityShards != WeakParityShards ||
		stats.FullBlocks != 1 || stats.PartialBlocks != 0 || stats.PendingSources != 0 {
		t.Fatalf("full stats=%+v", stats)
	}

	for i := 0; i < 3; i++ {
		if out, err := enc.Add([]byte{byte(100 + i)}, t0.Add(time.Second+time.Duration(i)*time.Millisecond)); err != nil || len(out) != 1 {
			t.Fatalf("partial source %d records=%d err=%v", i, len(out), err)
		}
	}
	if out, err := enc.FlushDue(t0.Add(time.Second + 7*time.Millisecond)); err != nil || len(out) != 0 {
		t.Fatalf("early partial flush records=%d err=%v", len(out), err)
	}
	out, err := enc.FlushDue(t0.Add(time.Second + 8*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("partial parity records=%d want=3", len(out))
	}
	stats = enc.Stats()
	if stats.SourceShards != DataShards+3 || stats.ParityShards != WeakParityShards+3 ||
		stats.FullBlocks != 1 || stats.PartialBlocks != 1 || stats.PendingSources != 0 {
		t.Fatalf("final stats=%+v", stats)
	}
	wantSourceBytes := uint64(DataShards+3) * uint64(HeaderSize+1)
	wantParityBytes := uint64(WeakParityShards+3) * uint64(HeaderSize+1)
	if stats.SourceBytes != wantSourceBytes || stats.ParityBytes != wantParityBytes {
		t.Fatalf("wire byte inventory source=%d/%d parity=%d/%d stats=%+v",
			stats.SourceBytes, wantSourceBytes, stats.ParityBytes, wantParityBytes, stats)
	}
}

func TestDecoderRecoveryStatsCountOnlyReconstructedSources(t *testing.T) {
	want, sources, repairs := profileWire(t, WeakParityShards, 900)
	_ = want
	dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 8, WeakParityShards)
	if err != nil {
		t.Fatal(err)
	}
	for i := WeakParityShards; i < DataShards; i++ {
		if _, _, err := dec.Add(sources[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, repair := range repairs {
		if _, _, err := dec.Add(repair); err != nil {
			t.Fatal(err)
		}
	}
	stats := dec.PressureCounts()
	if stats.ReconstructionEvents != 1 || stats.RecoveredSources != WeakParityShards {
		t.Fatalf("recovery stats=%+v want events=1 recovered=%d", stats, WeakParityShards)
	}

	full, fullSources, _ := profileWire(t, WeakParityShards, 901)
	_ = full
	dec2, _ := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 8, WeakParityShards)
	for _, source := range fullSources {
		if _, _, err := dec2.Add(source); err != nil {
			t.Fatal(err)
		}
	}
	if got := dec2.PressureCounts(); got.ReconstructionEvents != 0 || got.RecoveredSources != 0 {
		t.Fatalf("systematic-only recovery stats=%+v", got)
	}
}
