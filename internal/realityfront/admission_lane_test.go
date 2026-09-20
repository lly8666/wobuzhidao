package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"
)

func TestProtectedAdmissionCarriesAndEchoesLaneID(t *testing.T) {
	req := AdmissionRequest{
		RecordVersion: RecordVersionV1,
		LaneID:        3,
		TunnelID:      []byte("0123456789abcdef"),
		ClientLimit:   1300,
		Username:      "solo",
		Password:      "secret",
	}
	wire, err := marshalAdmissionRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readAdmissionRequest(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	if got.LaneID != 3 {
		t.Fatalf("lane id=%d want=3", got.LaneID)
	}

	result := AdmissionResult{
		RecordVersion: RecordVersionV1,
		LaneID:        3,
		TunnelID:      append([]byte(nil), req.TunnelID...),
		ClientLimit:   req.ClientLimit,
		ServerLimit:   1250,
	}
	reply, err := marshalAdmissionReply(result)
	if err != nil {
		t.Fatal(err)
	}
	negotiated, err := readAdmissionReply(bytes.NewReader(reply), req)
	if err != nil {
		t.Fatal(err)
	}
	if negotiated.LaneID != 3 {
		t.Fatalf("negotiated lane id=%d want=3", negotiated.LaneID)
	}

	tampered := append([]byte(nil), reply...)
	tampered[23] = 2
	if _, err := readAdmissionReply(bytes.NewReader(tampered), req); !errors.Is(err, ErrAdmissionParams) {
		t.Fatalf("tampered lane err=%v want admission params", err)
	}
}

func TestProtectedAdmissionRejectsOutOfRangeLaneID(t *testing.T) {
	req := AdmissionRequest{
		RecordVersion: RecordVersionV1,
		LaneID:        5,
		TunnelID:      []byte("0123456789abcdef"),
		ClientLimit:   1300,
		Username:      "solo",
		Password:      "secret",
	}
	if _, err := marshalAdmissionRequest(req); !errors.Is(err, ErrAdmissionParams) {
		t.Fatalf("request lane err=%v want admission params", err)
	}
	result := AdmissionResult{
		RecordVersion: RecordVersionV1,
		LaneID:        5,
		TunnelID:      append([]byte(nil), req.TunnelID...),
		ClientLimit:   1300,
		ServerLimit:   1250,
	}
	if _, err := marshalAdmissionReply(result); !errors.Is(err, ErrAdmissionParams) {
		t.Fatalf("reply lane err=%v want admission params", err)
	}
}

func TestProtectedAdmissionValidatorRejectsBeforeSuccessReply(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer peer.Close()
	defer assoc.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := EstablishServer(context.Background(), assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey: routeKey,
				TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout: 3 * time.Second,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit: 1250,
			ValidateRequest: func(req AdmissionRequest) error {
				if req.LaneID != 2 {
					t.Fatalf("validator lane=%d want=2", req.LaneID)
				}
				return errors.New("lane slot busy")
			},
		})
		serverDone <- err
	}()

	_, clientErr := EstablishClient(context.Background(), peer, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second,
		},
		Username: "solo", Password: "correct-password",
		TunnelID: []byte("0123456789abcdef"), ClientLimit: 1300, LaneID: 2,
	})
	if !errors.Is(clientErr, ErrAdmissionParams) {
		t.Fatalf("client err=%v want admission params", clientErr)
	}
	if serverErr := <-serverDone; !errors.Is(serverErr, ErrAdmissionParams) {
		t.Fatalf("server err=%v want admission params", serverErr)
	}
	if _, ok := assoc.TransitionState(); ok {
		t.Fatal("rejected lifecycle request prepared transition")
	}
}
