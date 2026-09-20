package datapath

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestTunnelPaddingPolicyDefaultsOffAndLocksAfterAttach(t *testing.T) {
	owner, err := NewTunnelOwner(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	if policy := owner.PaddingPolicy(); policy != (TunnelPaddingPolicy{}) {
		t.Fatalf("default policy=%+v", policy)
	}
	for _, policy := range []TunnelPaddingPolicy{
		{BytesPerRecord: 1},
		{Enabled: true, BytesPerRecord: 0, MaxPaddingBytes: 10, MaxPaddingRatioPPM: 1},
		{Enabled: true, BytesPerRecord: 1, MaxPaddingBytes: 0, MaxPaddingRatioPPM: 1},
		{Enabled: true, BytesPerRecord: 1, MaxPaddingBytes: 10, MaxPaddingRatioPPM: 0},
		{Enabled: true, BytesPerRecord: 1, MaxPaddingBytes: 10, MaxPaddingRatioPPM: PaddingRatioScalePPM + 1},
	} {
		if err := policy.Validate(); !errors.Is(err, ErrPaddingPolicy) {
			t.Fatalf("policy=%+v err=%v", policy, err)
		}
	}

	policy := TunnelPaddingPolicy{
		Enabled:            true,
		BytesPerRecord:     10,
		MaxPaddingBytes:    20,
		MaxPaddingRatioPPM: 250_000,
	}
	if err := owner.ConfigurePadding(policy); err != nil {
		t.Fatal(err)
	}
	lane := tunnelTestLane(t, RoleClient, 0, 80)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	if err := owner.ConfigurePadding(TunnelPaddingPolicy{}); !errors.Is(err, ErrPaddingPolicyLocked) {
		t.Fatalf("reconfigure after attach err=%v", err)
	}

	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	records, err := flow.Outbound(bytes.Repeat([]byte{0x41}, 20), time.Unix(600, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PaddingBytes != 0 {
		t.Fatalf("first ratio-limited records=%d padding=%d", len(records), firstPadding(records))
	}
	stats := owner.Stats().Padding
	if !stats.Enabled || stats.UsefulPayloadBytes != 20 || stats.PaddingBytes != 0 || stats.BudgetSkips != 1 {
		t.Fatalf("padding stats=%+v", stats)
	}
}

func TestNormalFlowsShareTunnelPaddingRatioAndTotalBudget(t *testing.T) {
	owner, err := NewTunnelOwner(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := owner.ConfigurePadding(TunnelPaddingPolicy{
		Enabled:            true,
		BytesPerRecord:     10,
		MaxPaddingBytes:    20,
		MaxPaddingRatioPPM: 250_000,
	}); err != nil {
		t.Fatal(err)
	}
	lane := tunnelTestLane(t, RoleClient, 0, 81)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	a, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	b, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte{0x42}, 20)
	var got []int
	for i, flow := range []*BusinessFlow{a, b, a, b, a} {
		records, err := flow.Outbound(payload, time.Unix(601, int64(i)*int64(time.Millisecond)))
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("send=%d records=%d", i, len(records))
		}
		got = append(got, records[0].PaddingBytes)
	}
	want := []int{0, 10, 0, 10, 0}
	if len(got) != len(want) {
		t.Fatalf("padding sequence=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("padding sequence=%v want=%v", got, want)
		}
	}

	stats := owner.Stats().Padding
	if stats.LogicalPayloads != 5 || stats.UsefulPayloadBytes != 100 ||
		stats.RecordRequests != 5 || stats.PaddedRecords != 2 ||
		stats.PaddingBytes != 20 || stats.BudgetSkips != 3 ||
		stats.HeadroomSkips != 0 {
		t.Fatalf("owner padding stats=%+v", stats)
	}
	laneStats := lane.Stats()
	if laneStats.PaddingRequests != 5 || laneStats.PaddingBytes != 20 ||
		laneStats.PaddingBudgetSkips != 3 {
		t.Fatalf("lane padding stats=%+v", laneStats)
	}
}

func TestFECParityConsumesButDoesNotEarnTunnelPaddingBudget(t *testing.T) {
	owner, err := NewTunnelOwner(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := owner.ConfigurePadding(TunnelPaddingPolicy{
		Enabled:            true,
		BytesPerRecord:     10,
		MaxPaddingBytes:    100,
		MaxPaddingRatioPPM: 50_000,
	}); err != nil {
		t.Fatal(err)
	}
	lane := tunnelTestLane(t, RoleClient, 4, 82)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte{0x43}, 20)
	var records int
	var padding int
	for i := 0; i < 20; i++ {
		out, err := flow.Outbound(payload, time.Unix(602, int64(i)*int64(time.Millisecond)))
		if err != nil {
			t.Fatal(err)
		}
		records += len(out)
		for _, record := range out {
			padding += record.PaddingBytes
		}
	}
	if records != 24 {
		t.Fatalf("20:4 records=%d want=24", records)
	}
	if padding != 20 {
		t.Fatalf("padding=%d want=20", padding)
	}
	stats := owner.Stats().Padding
	if stats.LogicalPayloads != 20 || stats.UsefulPayloadBytes != 400 ||
		stats.RecordRequests != 24 || stats.PaddedRecords != 2 ||
		stats.PaddingBytes != 20 || stats.BudgetSkips != 22 {
		t.Fatalf("padding stats=%+v", stats)
	}
}

func TestGameLaneCopiesDoNotMultiplyUsefulPaddingCredit(t *testing.T) {
	lease := gameLease(t)
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(lease, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := owner.ConfigurePadding(TunnelPaddingPolicy{
		Enabled:            true,
		BytesPerRecord:     10,
		MaxPaddingBytes:    100,
		MaxPaddingRatioPPM: 250_000,
	}); err != nil {
		t.Fatal(err)
	}
	for id := uint8(1); id <= 3; id++ {
		lane := leasedTestLane(t, RoleClient, 0, 90+id, lease)
		if _, err := owner.AttachInitial(id, lane); err != nil {
			t.Fatal(err)
		}
	}

	packet := businessIPv4Packet(
		leaseAddr,
		netip.MustParseAddr("1.1.1.1"),
		bytes.Repeat([]byte{0x44}, 20),
	)
	for i := 0; i < 2; i++ {
		out, err := owner.GameOutbound(packet, time.Unix(603, int64(i)*int64(time.Millisecond)))
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Lanes) != 3 || len(out.Failures) != 0 {
			t.Fatalf("send=%d lanes=%d failures=%v", i, len(out.Lanes), out.Failures)
		}
		var padded int
		for _, lane := range out.Lanes {
			for _, record := range lane.Records {
				if record.PaddingBytes > 0 {
					padded++
				}
			}
		}
		if padded != 1 {
			t.Fatalf("send=%d padded lane records=%d want=1", i, padded)
		}
	}

	stats := owner.Stats().Padding
	if stats.LogicalPayloads != 2 || stats.UsefulPayloadBytes != uint64(2*len(packet)) ||
		stats.RecordRequests != 6 || stats.PaddedRecords != 2 ||
		stats.PaddingBytes != 20 || stats.BudgetSkips != 4 {
		t.Fatalf("game padding stats=%+v packet_len=%d", stats, len(packet))
	}
}

func TestPaddingBudgetSurvivesReplacementDormantAndWake(t *testing.T) {
	owner, err := NewTunnelOwner(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := owner.ConfigurePadding(TunnelPaddingPolicy{
		Enabled:            true,
		BytesPerRecord:     10,
		MaxPaddingBytes:    20,
		MaxPaddingRatioPPM: PaddingRatioScalePPM,
	}); err != nil {
		t.Fatal(err)
	}
	oldLane := tunnelTestLane(t, RoleClient, 0, 100)
	oldSnap, err := owner.AttachInitial(1, oldLane)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0x45}, 20)

	first, err := flow.Outbound(payload, time.Unix(604, 0))
	if err != nil || firstPadding(first) != 10 {
		t.Fatalf("first padding=%d err=%v", firstPadding(first), err)
	}

	candidate := tunnelTestLane(t, RoleClient, 0, 101)
	if err := owner.BeginSameIDReplacement(oldSnap.Ref, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.PromoteSameIDReplacement(oldSnap.Ref); err != nil {
		t.Fatal(err)
	}
	second, err := flow.Outbound(payload, time.Unix(605, 0))
	if err != nil || firstPadding(second) != 10 {
		t.Fatalf("replacement padding=%d err=%v", firstPadding(second), err)
	}

	if _, err := owner.Dormant(); err != nil {
		t.Fatal(err)
	}
	wake := tunnelTestLane(t, RoleClient, 0, 102)
	if _, err := owner.AttachInitial(1, wake); err != nil {
		t.Fatal(err)
	}
	third, err := flow.Outbound(payload, time.Unix(606, 0))
	if err != nil || firstPadding(third) != 0 {
		t.Fatalf("wake padding=%d err=%v", firstPadding(third), err)
	}
	stats := owner.Stats().Padding
	if stats.LogicalPayloads != 3 || stats.UsefulPayloadBytes != 60 ||
		stats.PaddingBytes != 20 || stats.PaddedRecords != 2 ||
		stats.BudgetSkips != 1 {
		t.Fatalf("persistent padding stats=%+v", stats)
	}
}

func TestDefaultOffOwnerPathStillUsesZeroPadding(t *testing.T) {
	owner, err := NewTunnelOwner(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	lane := tunnelTestLane(t, RoleClient, 0, 110)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	records, err := flow.Outbound([]byte("default-off"), time.Unix(607, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PaddingBytes != 0 {
		t.Fatalf("records=%d padding=%d", len(records), firstPadding(records))
	}
	if stats := owner.Stats().Padding; stats != (TunnelPaddingStats{}) {
		t.Fatalf("default-off stats=%+v", stats)
	}
	if lane.Stats().PaddingRequests != 0 {
		t.Fatalf("default-off lane stats=%+v", lane.Stats())
	}
}
