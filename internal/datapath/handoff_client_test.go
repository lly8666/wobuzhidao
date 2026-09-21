package datapath

import (
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func TestClientLaneConfigFromAdmissionUsesNegotiatedDirections(t *testing.T) {
	session := &realityfront.ClientAdmissionSession{
		Negotiated: realityfront.AdmissionResult{
			RecordVersion: realityfront.RecordVersionV1,
			TunnelID: []byte("0123456789abcdef"),
			ClientLimit: 1300,
			ServerLimit: 1250,
			Keys: testKeys(),
		},
	}
	session.Negotiated.IncarnationNonce[0] = 9
	cfg, err := ClientLaneConfigFromAdmission(session, ClientLaneParams{
		ConnectionMTU: 1500,
		TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
		RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		PeerMSS: 1200, PeerMSSSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Role != RoleClient || cfg.ClientRecordLimit != 1300 || cfg.ServerRecordLimit != 1250 {
		t.Fatalf("client lane config=%+v", cfg)
	}
	if cfg.TxMTU.PeerMSS != 1200 || !cfg.TxMTU.PeerMSSSet ||
		cfg.TxMTU.RecordWireLimit != 1250 ||
		cfg.RxMTU.PeerMSS != faketcp.DefaultMSS || cfg.RxMTU.RecordWireLimit != 1300 {
		t.Fatalf("client direction MTU tx=%+v rx=%+v", cfg.TxMTU, cfg.RxMTU)
	}
	lane, err := NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	lane.Close()
}

func TestClientLaneConfigFromAdmissionRejectsInvalidVersion(t *testing.T) {
	_, err := ClientLaneConfigFromAdmission(&realityfront.ClientAdmissionSession{
		Negotiated: realityfront.AdmissionResult{RecordVersion: realityfront.RecordVersionV1 + 1},
	}, ClientLaneParams{})
	if !errors.Is(err, ErrAdmissionHandoff) {
		t.Fatalf("err=%v want admission handoff rejection", err)
	}
}


func TestFixedFECRuntimeDefaultsKeepOffAndAdmitOnlyLiveProfiles(t *testing.T) {
	if flush, blocks, err := FixedFECRuntimeDefaults(0); err != nil || flush != 0 || blocks != 0 {
		t.Fatalf("off defaults flush=%s blocks=%d err=%v", flush, blocks, err)
	}
	for _, parity := range []int{4, 8, 10, 12, 16, 20} {
		flush, blocks, err := FixedFECRuntimeDefaults(parity)
		if err != nil || flush != 8*time.Millisecond || blocks != 8 {
			t.Fatalf("20:%d defaults flush=%s blocks=%d err=%v", parity, flush, blocks, err)
		}
	}
	for _, parity := range []int{-1, 1, 9, 11, 21} {
		if _, _, err := FixedFECRuntimeDefaults(parity); err == nil {
			t.Fatalf("unsupported parity=%d accepted", parity)
		}
	}
}
