package linkdata

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

func fecPathTestConfig(parity, mtu int) FECPathConfig {
	cfg := FECPathConfig{SourceMTU: mtu, ParityShards: parity}
	if parity != 0 {
		cfg.FlushAfter = 8 * time.Millisecond
		cfg.MaxBlocks = 8
	}
	return cfg
}

func TestSupportedFECProfilesIncludeOffAndEveryLegacyFixedProfile(t *testing.T) {
	want := []int{0, 4, 8, 10, 12, 16, 20}
	if got := SupportedFECProfiles(); !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles=%v want=%v", got, want)
	}
	if _, err := NewFECPath(fecPathTestConfig(9, 1400)); err == nil {
		t.Fatal("20:9 unexpectedly admitted")
	}
}

func TestAllFECProfilesSystematicPathCompletesBeforeParityFlush(t *testing.T) {
	payload := bytes.Repeat([]byte("systematic-first-"), 16)
	t0 := time.Unix(100, 0)

	for _, parity := range SupportedFECProfiles() {
		t.Run(profileName(parity), func(t *testing.T) {
			cfg := fecPathTestConfig(parity, 64)
			tx, err := NewFECPath(cfg)
			if err != nil {
				t.Fatal(err)
			}
			rx, err := NewFECPath(cfg)
			if err != nil {
				t.Fatal(err)
			}

			wire, err := tx.Encode(payload, t0)
			if err != nil {
				t.Fatal(err)
			}
			if len(wire) < 2 {
				t.Fatalf("wire=%d want fragmented payload", len(wire))
			}

			var got []byte
			for i := len(wire) - 1; i >= 0; i-- {
				packets, err := rx.Decode(wire[i], t0.Add(time.Duration(len(wire)-i)*time.Microsecond))
				if err != nil {
					t.Fatalf("wire %d: %v", i, err)
				}
				if len(packets) > 0 {
					if len(packets) != 1 {
						t.Fatalf("wire %d delivered=%d", i, len(packets))
					}
					got = packets[0]
				}
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("profile=%d systematic delivery bytes=%d want=%d", parity, len(got), len(payload))
			}

			if parity == 0 {
				if tx.FECWireLimit() != cfg.SourceMTU || rx.State().FECEnabled {
					t.Fatalf("off state=%+v limit=%d", rx.State(), tx.FECWireLimit())
				}
				return
			}
			if tx.FECWireLimit() != cfg.SourceMTU+fec.HeaderSize {
				t.Fatalf("profile=%d wire limit=%d", parity, tx.FECWireLimit())
			}
			if st := rx.State(); !st.FECEnabled || st.ParityShards != parity || st.Decoder.InFlight != 1 {
				t.Fatalf("profile=%d before parity state=%+v", parity, st)
			}

			repairs, err := tx.FlushDue(t0.Add(cfg.FlushAfter))
			if err != nil {
				t.Fatal(err)
			}
			if len(repairs) == 0 {
				t.Fatalf("profile=%d emitted no partial parity", parity)
			}
			for _, repair := range repairs {
				if _, err := rx.Decode(repair, t0.Add(cfg.FlushAfter)); err != nil {
					t.Fatal(err)
				}
			}
			if st := rx.State(); st.Decoder.InFlight != 0 || st.Recovery.PendingDeadlines != 0 {
				t.Fatalf("profile=%d after parity state=%+v", parity, st)
			}
		})
	}
}

func TestAllFixedProfilesRecoverOneMissingLINKFragment(t *testing.T) {
	payload := bytes.Repeat([]byte("fragment-recovery-"), 7)
	t0 := time.Unix(200, 0)

	for _, parity := range fec.SupportedParityShards() {
		t.Run(profileName(parity), func(t *testing.T) {
			cfg := fecPathTestConfig(parity, 64)
			tx, _ := NewFECPath(cfg)
			rx, _ := NewFECPath(cfg)

			sources, err := tx.Encode(payload, t0)
			if err != nil {
				t.Fatal(err)
			}
			if len(sources) < 3 || len(sources) >= fec.DataShards {
				t.Fatalf("sources=%d want 3..19", len(sources))
			}
			repairs, err := tx.FlushDue(t0.Add(cfg.FlushAfter))
			if err != nil {
				t.Fatal(err)
			}
			if len(repairs) != len(sources) {
				t.Fatalf("profile=%d repairs=%d sources=%d", parity, len(repairs), len(sources))
			}

			missing := len(sources) / 2
			for i, source := range sources {
				if i == missing {
					continue
				}
				packets, err := rx.Decode(source, t0.Add(time.Duration(i)*time.Microsecond))
				if err != nil {
					t.Fatal(err)
				}
				if len(packets) != 0 {
					t.Fatalf("profile=%d completed before missing fragment repaired", parity)
				}
			}

			var got []byte
			for i, repair := range repairs {
				packets, err := rx.Decode(repair, t0.Add(cfg.FlushAfter+time.Duration(i)*time.Microsecond))
				if err != nil {
					t.Fatal(err)
				}
				if len(packets) > 0 {
					got = packets[len(packets)-1]
				}
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("profile=%d recovered bytes=%d want=%d", parity, len(got), len(payload))
			}
		})
	}
}

func TestFECRecoveryDeadlineIsAbsoluteAndIdleExpiryStillRuns(t *testing.T) {
	cfg := fecPathTestConfig(10, 256)
	tx, _ := NewFECPath(cfg)
	rx, _ := NewFECPath(cfg)
	t0 := time.Unix(300, 0)

	var sources [][]byte
	for _, payload := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		out, err := tx.Encode(payload, t0)
		if err != nil || len(out) != 1 {
			t.Fatalf("encode out=%d err=%v", len(out), err)
		}
		sources = append(sources, out[0])
	}
	packets, err := rx.Decode(sources[0], t0)
	if err != nil || len(packets) != 1 || string(packets[0]) != "one" {
		t.Fatalf("first packets=%q err=%v", packets, err)
	}
	packets, err = rx.Decode(sources[1], t0.Add(FECRecoveryHorizon-10*time.Millisecond))
	if err != nil || len(packets) != 1 || string(packets[0]) != "two" {
		t.Fatalf("progress packets=%q err=%v", packets, err)
	}

	// No inbound packet drives this retirement. Progress above must not refresh
	// the deadline that was created by the first heavy source.
	rx.Expire(t0.Add(FECRecoveryHorizon))
	st := rx.State()
	if st.Decoder.InFlight != 0 || st.Decoder.Retired != 1 ||
		st.Recovery.PendingDeadlines != 0 || st.Recovery.ExpireEvents != 1 ||
		st.Recovery.ExpiredIncomplete != 1 || st.Recovery.ExpiredMissingSources != fec.DataShards-2 {
		t.Fatalf("after idle expiry state=%+v", st)
	}

	late := t0.Add(FECRecoveryHorizon + time.Millisecond)
	packets, err = rx.Decode(sources[2], late)
	if err != nil || len(packets) != 1 || string(packets[0]) != "three" {
		t.Fatalf("late first arrival packets=%q err=%v", packets, err)
	}
	packets, err = rx.Decode(sources[2], late.Add(time.Millisecond))
	if err != nil || len(packets) != 0 {
		t.Fatalf("late duplicate packets=%q err=%v", packets, err)
	}
	if st := rx.State(); st.Decoder.InFlight != 0 {
		t.Fatalf("late source resurrected heavy state=%+v", st)
	}
}

func TestMalformedFECDoesNotPoisonFollowingHealthySource(t *testing.T) {
	cfg := fecPathTestConfig(10, 256)
	tx, _ := NewFECPath(cfg)
	rx, _ := NewFECPath(cfg)
	t0 := time.Unix(400, 0)

	first, err := tx.Encode([]byte("healthy-one"), t0)
	if err != nil || len(first) != 1 {
		t.Fatalf("first out=%d err=%v", len(first), err)
	}
	bad := append([]byte(nil), first[0]...)
	bad[10] = 9
	if _, err := rx.Decode(bad, t0); err == nil {
		t.Fatal("invalid parity geometry accepted")
	}
	if st := rx.State(); st.Decoder.InFlight != 0 {
		t.Fatalf("malformed input created state=%+v", st)
	}

	packets, err := rx.Decode(first[0], t0)
	if err != nil || len(packets) != 1 || string(packets[0]) != "healthy-one" {
		t.Fatalf("healthy packets=%q err=%v", packets, err)
	}
	conflict := append([]byte(nil), first[0]...)
	conflict[len(conflict)-1] ^= 0xff
	if _, err := rx.Decode(conflict, t0); !errors.Is(err, fec.ErrShardMismatch) {
		t.Fatalf("conflicting duplicate err=%v", err)
	}

	second, err := tx.Encode([]byte("healthy-two"), t0)
	if err != nil || len(second) != 1 {
		t.Fatalf("second out=%d err=%v", len(second), err)
	}
	packets, err = rx.Decode(second[0], t0)
	if err != nil || len(packets) != 1 || string(packets[0]) != "healthy-two" {
		t.Fatalf("healthy after bad packets=%q err=%v", packets, err)
	}
}

func TestLargeDatagramCrossingFECBlockKeepsOwnedWireBytes(t *testing.T) {
	cfg := fecPathTestConfig(4, 32)
	tx, err := NewFECPath(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rx, _ := NewFECPath(cfg)
	payload := bytes.Repeat([]byte("0123456789"), 35)
	t0 := time.Unix(500, 0)

	wire, err := tx.Encode(payload, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) <= fec.DataShards {
		t.Fatalf("wire=%d did not cross a 20-source block", len(wire))
	}
	firstSnapshot := append([]byte(nil), wire[0]...)

	var got []byte
	for i, datagram := range wire {
		packets, err := rx.Decode(datagram, t0.Add(time.Duration(i)*time.Microsecond))
		if err != nil {
			t.Fatalf("wire %d: %v", i, err)
		}
		if len(packets) > 0 {
			got = packets[len(packets)-1]
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("cross-block payload bytes=%d want=%d", len(got), len(payload))
	}
	if !bytes.Equal(wire[0], firstSnapshot) {
		t.Fatal("returned wire mutated after encoder backing-slot reuse")
	}

	repairs, err := tx.FlushDue(t0.Add(cfg.FlushAfter))
	if err != nil {
		t.Fatal(err)
	}
	for _, repair := range repairs {
		if _, err := rx.Decode(repair, t0.Add(cfg.FlushAfter)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFECPathsKeepLaneLocalStateSeparate(t *testing.T) {
	cfg := fecPathTestConfig(8, 256)
	tx, _ := NewFECPath(cfg)
	laneA, _ := NewFECPath(cfg)
	laneB, _ := NewFECPath(cfg)
	t0 := time.Unix(600, 0)

	a, _ := tx.Encode([]byte("lane-a-source"), t0)
	b, _ := tx.Encode([]byte("lane-b-source"), t0)
	if packets, err := laneA.Decode(a[0], t0); err != nil || len(packets) != 1 || string(packets[0]) != "lane-a-source" {
		t.Fatalf("lane A packets=%q err=%v", packets, err)
	}
	if packets, err := laneB.Decode(b[0], t0); err != nil || len(packets) != 1 || string(packets[0]) != "lane-b-source" {
		t.Fatalf("lane B packets=%q err=%v", packets, err)
	}
	if laneA.State().Decoder.InFlight != 1 || laneB.State().Decoder.InFlight != 1 {
		t.Fatalf("lane states A=%+v B=%+v", laneA.State(), laneB.State())
	}
}

func profileName(parity int) string {
	if parity == 0 {
		return "off"
	}
	if parity < 10 {
		return "20x" + string(rune('0'+parity))
	}
	return "20x" + string([]byte{byte('0' + parity/10), byte('0' + parity%10)})
}
