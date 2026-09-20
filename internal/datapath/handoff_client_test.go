package datapath

import (
	"errors"
	"testing"

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
