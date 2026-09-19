package datapath

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func TestPaddingLeavesFECAndLINKPayloadByteExactForEveryProfile(t *testing.T) {
	payload := []byte{0x41, 0x00, 0x42, 0x00, 0x00}
	for _, parity := range []int{0, 4, 8, 10, 12, 16, 20} {
		t.Run(profileName(parity), func(t *testing.T) {
			plainLane, err := NewLane(pairConfig(RoleClient, parity))
			if err != nil {
				t.Fatal(err)
			}
			defer plainLane.Close()
			paddedLane, err := NewLane(pairConfig(RoleClient, parity))
			if err != nil {
				t.Fatal(err)
			}
			defer paddedLane.Close()
			server, err := NewLane(pairConfig(RoleServer, parity))
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()

			plain, err := plainLane.Outbound(payload, time.Unix(100, 0))
			if err != nil || len(plain) != 1 {
				t.Fatalf("plain records=%d err=%v", len(plain), err)
			}
			padded, err := paddedLane.OutboundWithPadding(payload, time.Unix(100, 0), PaddingRequest{Bytes: 9})
			if err != nil || len(padded) != 1 {
				t.Fatalf("padded records=%d err=%v", len(padded), err)
			}
			if padded[0].PaddingBytes != 9 || len(padded[0].Wire) != len(plain[0].Wire)+9 {
				t.Fatalf("padding metadata=%d plain/padded wire=%d/%d", padded[0].PaddingBytes, len(plain[0].Wire), len(padded[0].Wire))
			}

			opener, err := tlsrecord.NewOpener(testKeys().C2S, paddedLane.TxBudget().RecordWireMTU)
			if err != nil {
				t.Fatal(err)
			}
			plainRecord, err := opener.OpenRecord(plain[0].Wire)
			if err != nil {
				t.Fatal(err)
			}
			paddedRecord, err := opener.OpenRecord(padded[0].Wire)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plainRecord.Payload, paddedRecord.Payload) {
				t.Fatalf("profile=%d padding changed LINK/FEC payload
plain=%x
padded=%x", parity, plainRecord.Payload, paddedRecord.Payload)
			}

			result, err := server.InboundPayload(padded[0].Wire, time.Unix(100, 0))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.RecordErrors) != 0 || len(result.PathErrors) != 0 ||
				len(result.Datagrams) != 1 || !bytes.Equal(result.Datagrams[0], payload) {
				t.Fatalf("profile=%d result=%+v", parity, result)
			}
			stats := paddedLane.Stats()
			if stats.PaddingRequests != 1 || stats.PaddedRecords != 1 ||
				stats.RequestedPaddingBytes != 9 || stats.PaddingBytes != 9 ||
				stats.PaddingBudgetSkips != 0 {
				t.Fatalf("profile=%d stats=%+v", parity, stats)
			}
		})
	}
}

func TestFullSizeSourceAndParitySkipPaddingWithoutExtraRecord(t *testing.T) {
	for _, parity := range []int{0, 4, 8, 10, 12, 16, 20} {
		t.Run(profileName(parity), func(t *testing.T) {
			client, err := NewLane(pairConfig(RoleClient, parity))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			full := bytes.Repeat([]byte{0x5a}, client.TxBudget().LinkFrameMTU)

			records, err := client.OutboundWithPadding(full, time.Unix(110, 0), PaddingRequest{Bytes: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].PaddingBytes != 0 {
				t.Fatalf("profile=%d source records=%d padding=%d", parity, len(records), firstPadding(records))
			}
			if len(records[0].Wire) != client.TxBudget().RecordWireMTU {
				t.Fatalf("profile=%d source wire=%d record mtu=%d", parity, len(records[0].Wire), client.TxBudget().RecordWireMTU)
			}
			if parity != 0 {
				repairs, err := client.FlushWithPadding(PaddingRequest{Bytes: 1})
				if err != nil {
					t.Fatal(err)
				}
				if len(repairs) != 1 || repairs[0].PaddingBytes != 0 ||
					len(repairs[0].Wire) != client.TxBudget().RecordWireMTU {
					t.Fatalf("profile=%d repairs=%d padding=%d wire=%d", parity, len(repairs), firstPadding(repairs), firstWireLen(repairs))
				}
			}
			stats := client.Stats()
			wantSkips := uint64(1)
			if parity != 0 {
				wantSkips = 2
			}
			if stats.PaddingBudgetSkips != wantSkips || stats.PaddingBytes != 0 {
				t.Fatalf("profile=%d stats=%+v", parity, stats)
			}
		})
	}
}

func TestPaddingDoesNotIncreaseFragmentOrRecordCountAcrossMTUMatrix(t *testing.T) {
	for _, mtu := range []int{576, 1280, 1400, 1500, 1600, 9000} {
		for _, parity := range []int{0, 20} {
			cfg := pairConfig(RoleClient, parity)
			cfg.ClientRecordLimit = tlsrecord.MaxWireLen
			cfg.ServerRecordLimit = tlsrecord.MaxWireLen
			cfg.TxMTU = mtuConfig(tlsrecord.MaxWireLen, parity)
			cfg.RxMTU = mtuConfig(tlsrecord.MaxWireLen, parity)
			cfg.TxMTU.ConnectionMTU = mtu
			cfg.RxMTU.ConnectionMTU = mtu
			cfg.TxMTU.PeerMSS = uint16(mtu - 40)
			cfg.RxMTU.PeerMSS = uint16(mtu - 40)

			plain, err := NewLane(cfg)
			if err != nil {
				t.Fatalf("mtu=%d parity=%d plain: %v", mtu, parity, err)
			}
			padded, err := NewLane(cfg)
			if err != nil {
				plain.Close()
				t.Fatalf("mtu=%d parity=%d padded: %v", mtu, parity, err)
			}
			payload := bytes.Repeat([]byte{0x33}, plain.TxBudget().LinkFrameMTU+1)
			plainRecords, err := plain.Outbound(payload, time.Unix(120, 0))
			if err != nil {
				t.Fatalf("mtu=%d parity=%d plain outbound: %v", mtu, parity, err)
			}
			paddedRecords, err := padded.OutboundWithPadding(payload, time.Unix(120, 0), PaddingRequest{Bytes: 32})
			if err != nil {
				t.Fatalf("mtu=%d parity=%d padded outbound: %v", mtu, parity, err)
			}
			if len(paddedRecords) != len(plainRecords) {
				t.Fatalf("mtu=%d parity=%d padding changed records=%d want=%d", mtu, parity, len(paddedRecords), len(plainRecords))
			}
			for i, record := range paddedRecords {
				if len(record.Wire) > padded.TxBudget().RecordWireMTU ||
					padded.TxBudget().OuterPacketLenForRecord(len(record.Wire)) > padded.TxBudget().EffectivePacketMTU ||
					len(record.Wire) > padded.TxBudget().PeerMSS {
					t.Fatalf("mtu=%d parity=%d record=%d budget=%+v wire=%d", mtu, parity, i, padded.TxBudget(), len(record.Wire))
				}
			}
			if padded.Stats().PaddingBudgetSkips == 0 {
				t.Fatalf("mtu=%d parity=%d expected full first fragment padding skip", mtu, parity)
			}
			plain.Close()
			padded.Close()
		}
	}
}

func TestPaddingPolicyNegativeRejectsBeforeFECStateAndHeadroomSkipIsImmediate(t *testing.T) {
	client, err := NewLane(pairConfig(RoleClient, 10))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.OutboundWithPadding([]byte("bad"), time.Unix(130, 0), PaddingRequest{Bytes: -1}); !errors.Is(err, tlsrecord.ErrInvalidPadding) {
		t.Fatalf("negative padding err=%v", err)
	}
	good, err := client.Outbound([]byte("good"), time.Unix(130, 0))
	if err != nil || len(good) != 1 || good[0].PN != 0 {
		t.Fatalf("good records=%d pn=%d err=%v", len(good), firstPN(good), err)
	}
	opener, _ := tlsrecord.NewOpener(testKeys().C2S, client.TxBudget().RecordWireMTU)
	record, err := opener.OpenRecord(good[0].Wire)
	if err != nil {
		t.Fatal(err)
	}
	h, err := fec.ParseBlockHeader(record.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if h.BlockID != 1 || h.ShardIndex != 0 {
		t.Fatalf("negative request mutated FEC state header=%+v", h)
	}

	full := bytes.Repeat([]byte{0x44}, client.TxBudget().LinkFrameMTU)
	skipped, err := client.OutboundWithPadding(full, time.Unix(131, 0), PaddingRequest{Bytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || skipped[0].PaddingBytes != 0 {
		t.Fatalf("skip records=%d padding=%d", len(skipped), firstPadding(skipped))
	}
	if client.Stats().PaddingRejected != 1 || client.Stats().PaddingBudgetSkips == 0 {
		t.Fatalf("stats=%+v", client.Stats())
	}
}

func TestPaddedLostAStillDeliversBAndReturnedCiphertextStaysImmutable(t *testing.T) {
	for _, parity := range []int{0, 4, 8, 10, 12, 16, 20} {
		t.Run(profileName(parity), func(t *testing.T) {
			client, server := lanePair(t, parity)
			defer client.Close()
			defer server.Close()
			t0 := time.Unix(140, 0)
			a, err := client.OutboundWithPadding([]byte("A-lost"), t0, PaddingRequest{Bytes: 13})
			if err != nil || len(a) != 1 {
				t.Fatalf("A records=%d err=%v", len(a), err)
			}
			aSnapshot := append([]byte(nil), a[0].Wire...)
			b, err := client.OutboundWithPadding([]byte("B-deliver"), t0.Add(50*time.Millisecond), PaddingRequest{Bytes: 13})
			if err != nil || len(b) != 1 {
				t.Fatalf("B records=%d err=%v", len(b), err)
			}
			result, err := server.InboundPayload(b[0].Wire, t0.Add(50*time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Datagrams) != 1 || string(result.Datagrams[0]) != "B-deliver" {
				t.Fatalf("profile=%d B result=%+v", parity, result)
			}
			if !bytes.Equal(a[0].Wire, aSnapshot) {
				t.Fatal("padded ciphertext mutated after later seal")
			}
			// Retransmission owns the exact existing ciphertext; no re-padding.
			retransmit := append([]byte(nil), a[0].Wire...)
			if !bytes.Equal(retransmit, aSnapshot) {
				t.Fatal("retransmit ciphertext changed")
			}
		})
	}
}

func firstPadding(records []WireRecord) int {
	if len(records) == 0 {
		return -1
	}
	return records[0].PaddingBytes
}

func firstWireLen(records []WireRecord) int {
	if len(records) == 0 {
		return 0
	}
	return len(records[0].Wire)
}

func firstPN(records []WireRecord) uint64 {
	if len(records) == 0 {
		return ^uint64(0)
	}
	return records[0].PN
}
