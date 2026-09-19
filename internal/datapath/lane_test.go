package datapath

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func testKeys() tlsrecord.KeyPair {
	var out tlsrecord.KeyPair
	for i := range out.C2S.AEADKey {
		out.C2S.AEADKey[i] = byte(i + 1)
		out.C2S.HPKey[i] = byte(0x80 + i)
		out.S2C.AEADKey[i] = byte(0x40 + i)
		out.S2C.HPKey[i] = byte(0xc0 + i)
	}
	for i := range out.C2S.IV {
		out.C2S.IV[i] = byte(0x10 + i)
		out.S2C.IV[i] = byte(0x30 + i)
	}
	return out
}

func mtuConfig(limit, parity int) pathmtu.Config {
	return pathmtu.Config{
		ConnectionMTU:  1500,
		IPv4HeaderLen:  20,
		TCPHeaderLen:   20,
		PeerMSS:        faketcp.DefaultMSS,
		PeerMSSSet:     true,
		RecordWireLimit: limit,
		ParityShards:    parity,
	}
}

func pairConfig(role Role, parity int) LaneConfig {
	cfg := LaneConfig{
		Role:              role,
		ClientRecordLimit: 1300,
		ServerRecordLimit: 1250,
		Keys:              testKeys(),
		ParityShards:      parity,
		TunnelID:          []byte("0123456789abcdef"),
	}
	cfg.IncarnationNonce[0] = 7
	if parity != 0 {
		cfg.FlushAfter = 8 * time.Millisecond
		cfg.MaxBlocks = 8
	}
	if role == RoleClient {
		cfg.TxMTU = mtuConfig(cfg.ServerRecordLimit, parity)
		cfg.RxMTU = mtuConfig(cfg.ClientRecordLimit, parity)
	} else {
		cfg.TxMTU = mtuConfig(cfg.ClientRecordLimit, parity)
		cfg.RxMTU = mtuConfig(cfg.ServerRecordLimit, parity)
	}
	return cfg
}

func lanePair(t *testing.T, parity int) (*Lane, *Lane) {
	t.Helper()
	client, err := NewLane(pairConfig(RoleClient, parity))
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLane(pairConfig(RoleServer, parity))
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}

func deliverRecords(t *testing.T, dst *Lane, records []WireRecord, now time.Time) [][]byte {
	t.Helper()
	var got [][]byte
	for _, record := range records {
		result, err := dst.InboundPayload(record.Wire, now)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.RecordErrors) != 0 || len(result.PathErrors) != 0 {
			t.Fatalf("record errors=%v path errors=%v", result.RecordErrors, result.PathErrors)
		}
		got = append(got, result.Datagrams...)
	}
	return got
}

func TestAllFixedProfilesLaneRoundTripAndImmutableConfig(t *testing.T) {
	for _, parity := range []int{0, 4, 8, 10, 12, 16, 20} {
		t.Run(profileName(parity), func(t *testing.T) {
			client, server := lanePair(t, parity)
			defer client.Close()
			defer server.Close()

			c2s := bytes.Repeat([]byte("client-datagram-"), 180)
			records, err := client.Outbound(c2s, time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			if len(records) < 2 {
				t.Fatalf("profile=%d records=%d want fragmented", parity, len(records))
			}
			// Record PN ordering is not a receive dependency.
			var got [][]byte
			for i := len(records) - 1; i >= 0; i-- {
				result, err := server.InboundPayload(records[i].Wire, time.Unix(1, 0))
				if err != nil {
					t.Fatal(err)
				}
				if len(result.RecordErrors) != 0 || len(result.PathErrors) != 0 {
					t.Fatalf("profile=%d record errors=%v path errors=%v", parity, result.RecordErrors, result.PathErrors)
				}
				got = append(got, result.Datagrams...)
			}
			if len(got) != 1 || !bytes.Equal(got[0], c2s) {
				t.Fatalf("profile=%d c2s deliveries=%d bytes=%d", parity, len(got), firstLen(got))
			}

			s2c := bytes.Repeat([]byte("server-datagram-"), 140)
			back, err := server.Outbound(s2c, time.Unix(2, 0))
			if err != nil {
				t.Fatal(err)
			}
			delivered := deliverRecords(t, client, back, time.Unix(2, 0))
			if len(delivered) != 1 || !bytes.Equal(delivered[0], s2c) {
				t.Fatalf("profile=%d s2c deliveries=%d", parity, len(delivered))
			}

			if client.Config().ParityShards != parity || server.Config().ParityShards != parity {
				t.Fatalf("profile mutated client=%d server=%d", client.Config().ParityShards, server.Config().ParityShards)
			}
			if client.TxBudget().ParityShards != parity || server.RxBudget().ParityShards != parity {
				t.Fatalf("budget/profile mismatch parity=%d", parity)
			}
		})
	}
}

func TestLaneConfigMismatchFailsBeforeActivation(t *testing.T) {
	cfg := pairConfig(RoleClient, 8)
	cfg.TxMTU.ParityShards = 4
	if _, err := NewLane(cfg); !errors.Is(err, ErrConfigMismatch) {
		t.Fatalf("parity mismatch err=%v", err)
	}

	cfg = pairConfig(RoleServer, 10)
	cfg.TxMTU.RecordWireLimit--
	if _, err := NewLane(cfg); !errors.Is(err, ErrConfigMismatch) {
		t.Fatalf("record limit mismatch err=%v", err)
	}

	cfg = pairConfig(RoleClient, 0)
	cfg.Role = 99
	if _, err := NewLane(cfg); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("role mismatch err=%v", err)
	}
}

func TestServerAdmissionHandoffBindsDirectionLimitsPeerMSSAndProfile(t *testing.T) {
	syn := faketcp.Segment{
		SrcIP: [4]byte{10, 0, 0, 1}, DstIP: [4]byte{10, 0, 0, 2},
		SrcPort: 40000, DstPort: 443,
		Seq: 100, Flags: faketcp.FlagSYN, Window: 65535,
		MSS: 1200, MSSSet: true,
	}
	assoc, err := faketcp.NewServerAssociation(syn, 9000, 100*time.Millisecond, func(faketcp.Segment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer assoc.Close()

	session := &realityfront.ServerAdmissionSession{
		Negotiated: realityfront.AdmissionResult{
			RecordVersion: realityfront.RecordVersionV1,
			TunnelID:      []byte("0123456789abcdef"),
			ClientLimit:   1100,
			ServerLimit:   1000,
			Keys:          testKeys(),
		},
	}
	cfg, err := ServerLaneConfigFromAdmission(session, assoc, ServerLaneParams{
		ConnectionMTU:   1500,
		TxIPv4HeaderLen: 20,
		TxTCPHeaderLen:  20,
		RxIPv4HeaderLen: 20,
		RxTCPHeaderLen:  20,
		ParityShards:    10,
		FlushAfter:      8 * time.Millisecond,
		MaxBlocks:       8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Role != RoleServer || cfg.ParityShards != 10 {
		t.Fatalf("server cfg role/profile=%d/%d", cfg.Role, cfg.ParityShards)
	}
	if cfg.TxMTU.RecordWireLimit != 1100 || cfg.RxMTU.RecordWireLimit != 1000 {
		t.Fatalf("direction limits tx/rx=%d/%d", cfg.TxMTU.RecordWireLimit, cfg.RxMTU.RecordWireLimit)
	}
	if !cfg.TxMTU.PeerMSSSet || cfg.TxMTU.PeerMSS != 1200 {
		t.Fatalf("server tx peer MSS=%d set=%v", cfg.TxMTU.PeerMSS, cfg.TxMTU.PeerMSSSet)
	}
	if !cfg.RxMTU.PeerMSSSet || cfg.RxMTU.PeerMSS != faketcp.DefaultMSS {
		t.Fatalf("server rx advertised MSS mirror=%d set=%v", cfg.RxMTU.PeerMSS, cfg.RxMTU.PeerMSSSet)
	}
	lane, err := NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer lane.Close()
	if lane.TxBudget().RecordWireMTU != 1100 || lane.RxBudget().RecordWireMTU != 1000 {
		t.Fatalf("derived direction budgets tx/rx=%d/%d", lane.TxBudget().RecordWireMTU, lane.RxBudget().RecordWireMTU)
	}
}

func TestMissingRecordDoesNotBlockLaterRecordForEveryProfile(t *testing.T) {
	for _, parity := range []int{0, 4, 8, 10, 12, 16, 20} {
		t.Run(profileName(parity), func(t *testing.T) {
			client, server := lanePair(t, parity)
			defer client.Close()
			defer server.Close()
			t0 := time.Unix(10, 0)

			a, err := client.Outbound([]byte("A-lost"), t0)
			if err != nil || len(a) != 1 {
				t.Fatalf("A records=%d err=%v", len(a), err)
			}
			b, err := client.Outbound([]byte("B-deliver"), t0.Add(50*time.Millisecond))
			if err != nil || len(b) != 1 {
				t.Fatalf("B records=%d err=%v", len(b), err)
			}
			result, err := server.InboundPayload(b[0].Wire, t0.Add(50*time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Datagrams) != 1 || string(result.Datagrams[0]) != "B-deliver" {
				t.Fatalf("profile=%d B delivery=%q recordErr=%v pathErr=%v", parity, result.Datagrams, result.RecordErrors, result.PathErrors)
			}
		})
	}
}

func TestOwnerTimerExpiresFECWithoutTrafficAndLateSourceStillFirstDelivers(t *testing.T) {
	client, server := lanePair(t, 10)
	defer client.Close()
	defer server.Close()
	t0 := time.Unix(20, 0)

	first, err := client.Outbound([]byte("first"), t0)
	if err != nil || len(first) != 1 {
		t.Fatalf("first records=%d err=%v", len(first), err)
	}
	delivered := deliverRecords(t, server, first, t0)
	if len(delivered) != 1 || string(delivered[0]) != "first" {
		t.Fatalf("first delivery=%q", delivered)
	}
	if st := server.Stats(); st.RxPath.Decoder.InFlight != 1 || st.RxPath.Recovery.PendingDeadlines != 1 {
		t.Fatalf("before expiry stats=%+v", st)
	}

	if err := server.Expire(t0.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	st := server.Stats()
	if st.RxPath.Decoder.InFlight != 0 || st.RxPath.Decoder.Retired != 1 ||
		st.RxPath.Recovery.PendingDeadlines != 0 || st.RxPath.Recovery.ExpireEvents != 1 {
		t.Fatalf("after expiry stats=%+v", st)
	}

	late, err := client.Outbound([]byte("late-source"), t0.Add(3*time.Second+time.Millisecond))
	if err != nil || len(late) != 1 {
		t.Fatalf("late records=%d err=%v", len(late), err)
	}
	delivered = deliverRecords(t, server, late, t0.Add(3*time.Second+time.Millisecond))
	if len(delivered) != 1 || string(delivered[0]) != "late-source" {
		t.Fatalf("late delivery=%q", delivered)
	}
	dup, err := server.InboundPayload(late[0].Wire, t0.Add(3*time.Second+2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(dup.Datagrams) != 0 || len(dup.RecordErrors) != 1 || !errors.Is(dup.RecordErrors[0], tlsrecord.ErrDuplicate) {
		t.Fatalf("late duplicate result=%+v", dup)
	}
}

func TestTransitionPayloadsEnterDetachedOwnerWithoutTCPSeqOrdering(t *testing.T) {
	client, server := lanePair(t, 0)
	defer client.Close()
	defer server.Close()
	t0 := time.Unix(30, 0)

	one, _ := client.Outbound([]byte("one"), t0)
	two, _ := client.Outbound([]byte("two"), t0)
	packets := []faketcp.TransitionPacket{
		{Seq: 9000, Payload: two[0].Wire},
		{Seq: 8000, Payload: one[0].Wire},
	}
	result, err := server.InboundTransition(packets, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Datagrams) != 2 || string(result.Datagrams[0]) != "two" || string(result.Datagrams[1]) != "one" {
		t.Fatalf("transition deliveries=%q errors=%v/%v", result.Datagrams, result.RecordErrors, result.PathErrors)
	}
}

func TestReturnedRecordWireRemainsOwnedAcrossLaterEncoderReuse(t *testing.T) {
	client, _ := lanePair(t, 4)
	defer client.Close()
	t0 := time.Unix(40, 0)

	held, err := client.Outbound(bytes.Repeat([]byte{0xa5}, 200), t0)
	if err != nil || len(held) != 1 {
		t.Fatalf("held records=%d err=%v", len(held), err)
	}
	snapshot := append([]byte(nil), held[0].Wire...)
	for i := 0; i < 40; i++ {
		if _, err := client.Outbound(bytes.Repeat([]byte{byte(i)}, 300+i), t0.Add(time.Duration(i+1)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.Flush(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(held[0].Wire, snapshot) {
		t.Fatal("record wire mutated after later LINK/FEC/seal work")
	}
	// Retransmission ownership is the exact already-sealed ciphertext.
	if !bytes.Equal(held[0].Wire, snapshot) {
		t.Fatal("retransmit wire changed")
	}
}

func TestCloseReleasesOnlyOneLaneAndConcurrentOwnerSerializesPN(t *testing.T) {
	a, b := lanePair(t, 0)
	a.Close()
	if _, err := a.Outbound([]byte("closed"), time.Unix(50, 0)); !errors.Is(err, ErrLaneClosed) {
		t.Fatalf("closed outbound err=%v", err)
	}
	if _, err := a.InboundPayload([]byte("x"), time.Unix(50, 0)); !errors.Is(err, ErrLaneClosed) {
		t.Fatalf("closed inbound err=%v", err)
	}

	const n = 64
	var wg sync.WaitGroup
	pnCh := make(chan uint64, n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			records, err := b.Outbound([]byte{byte(i)}, time.Unix(50, int64(i)))
			if err != nil {
				errCh <- err
				return
			}
			if len(records) != 1 {
				errCh <- errors.New("unexpected record count")
				return
			}
			pnCh <- records[0].PN
			errCh <- nil
		}()
	}
	wg.Wait()
	close(pnCh)
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[uint64]bool, n)
	for pn := range pnCh {
		if seen[pn] {
			t.Fatalf("duplicate PN=%d", pn)
		}
		seen[pn] = true
	}
	for pn := uint64(0); pn < n; pn++ {
		if !seen[pn] {
			t.Fatalf("missing PN=%d", pn)
		}
	}
	if st := b.Stats(); st.OutboundRecords != n || st.Closed {
		t.Fatalf("sibling lane stats=%+v", st)
	}
	b.Close()
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

func firstLen(items [][]byte) int {
	if len(items) == 0 {
		return 0
	}
	return len(items[0])
}
