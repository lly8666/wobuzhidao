package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func TestProtectedAdmissionNegotiatesExporterContextBeforeFinalReply(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer peer.Close()
	defer assoc.Close()

	nonce := [16]byte{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
	var sawPreparedReply bool
	peer.onServerPayload = func(faketcp.Segment) {
		if state, ok := assoc.TransitionState(); ok && state == faketcp.TransitionPrepared {
			sawPreparedReply = true
		}
	}

	type serverResult struct {
		session *ServerAdmissionSession
		err     error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		session, err := EstablishServer(context.Background(), assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    3 * time.Second,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit:      1450,
			Random:           bytes.NewReader(nonce[:]),
		})
		serverDone <- serverResult{session: session, err: err}
	}()

	clientSession, err := EstablishClient(context.Background(), peer, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test",
			RouteKey:   routeKey,
			Timeout:    3 * time.Second,
		},
		Username:    "solo",
		Password:    "correct-password",
		TunnelID:    []byte("0123456789abcdef"),
		ClientLimit: 1500,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := <-serverDone
	if server.err != nil {
		t.Fatal(server.err)
	}
	if !sawPreparedReply {
		t.Fatal("final protected admission reply arrived before transition prepare")
	}
	if clientSession.Negotiated.RecordVersion != RecordVersionV1 ||
		server.session.Negotiated.RecordVersion != RecordVersionV1 {
		t.Fatal("record version was not fixed to v1")
	}
	if clientSession.Negotiated.IncarnationNonce != nonce ||
		server.session.Negotiated.IncarnationNonce != nonce {
		t.Fatalf("nonce mismatch client=%x server=%x want=%x",
			clientSession.Negotiated.IncarnationNonce,
			server.session.Negotiated.IncarnationNonce,
			nonce)
	}
	if clientSession.Negotiated.ClientLimit != 1500 ||
		clientSession.Negotiated.ServerLimit != 1450 ||
		!bytes.Equal(clientSession.Negotiated.TunnelID, []byte("0123456789abcdef")) {
		t.Fatalf("client negotiated=%#v", clientSession.Negotiated)
	}
	if clientSession.Negotiated.Keys != server.session.Negotiated.Keys {
		t.Fatal("client/server exporter keys differ after protected negotiation")
	}
	if clientSession.TLS.Keys != clientSession.Negotiated.Keys ||
		server.session.TLS.Keys != server.session.Negotiated.Keys {
		t.Fatal("session keys are not the negotiated exporter keys")
	}
	if clientSession.TLS.Conn != nil || server.session.TLS.Conn != nil {
		t.Fatal("successful admission retained an old TLS writer after detach")
	}
	if len(server.session.EarlyRecords) != 0 {
		t.Fatalf("unexpected early records: %#v", server.session.EarlyRecords)
	}
	if state, ok := assoc.TransitionState(); !ok || state != faketcp.TransitionDetached {
		t.Fatalf("transition state=%v ok=%v want detached", state, ok)
	}
	if want := peer.NextSendSeq(); server.session.Boundary != want {
		t.Fatalf("boundary=%d client next=%d", server.session.Boundary, want)
	}
}

func TestProtectedAdmissionAuthenticationFailureDoesNotPrepareTransition(t *testing.T) {
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
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    3 * time.Second,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit:      1450,
		})
		serverDone <- err
	}()

	_, clientErr := EstablishClient(context.Background(), peer, ClientAdmissionConfig{
		TLS: ClientConfig{ServerName: "target.test", RouteKey: routeKey, Timeout: 3 * time.Second},
		Username: "solo", Password: "wrong-password",
		TunnelID: []byte("0123456789abcdef"), ClientLimit: 1500,
	})
	if !errors.Is(clientErr, ErrAdmissionAuth) {
		t.Fatalf("client err=%v want auth failure", clientErr)
	}
	if serverErr := <-serverDone; !errors.Is(serverErr, ErrAdmissionAuth) {
		t.Fatalf("server err=%v want auth failure", serverErr)
	}
	if _, ok := assoc.TransitionState(); ok {
		t.Fatal("authentication failure prepared transition")
	}
}

func TestAdmissionUnknownRecordVersionIsExplicitlyRejected(t *testing.T) {
	wire := make([]byte, admissionRequestLen)
	copy(wire[:4], admissionMagic)
	binary.BigEndian.PutUint16(wire[4:6], RecordVersionV1+1)
	binary.BigEndian.PutUint16(wire[6:8], 1500)
	binary.BigEndian.PutUint16(wire[8:10], 1)
	binary.BigEndian.PutUint16(wire[10:12], 1)
	binary.BigEndian.PutUint16(wire[12:14], 1)

	if _, err := readAdmissionRequest(bytes.NewReader(wire)); !errors.Is(err, ErrAdmissionVersion) {
		t.Fatalf("err=%v want unsupported version", err)
	}
	if got := admissionStatus(ErrAdmissionVersion); got != admissionVersionFail {
		t.Fatalf("status=%d want version failure", got)
	}
}

func TestAdmissionReplyRejectsContextEchoMismatch(t *testing.T) {
	req := AdmissionRequest{
		RecordVersion: RecordVersionV1,
		TunnelID:      []byte("AAAAAAAAAAAAAAAA"),
		ClientLimit:   1500,
		Username:      "u",
		Password:      "p",
	}
	result := AdmissionResult{
		RecordVersion: RecordVersionV1,
		TunnelID:      []byte("BBBBBBBBBBBBBBBB"),
		ClientLimit:   1500,
		ServerLimit:   1450,
	}
	wire, err := marshalAdmissionReply(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readAdmissionReply(bytes.NewReader(wire), req); !errors.Is(err, ErrAdmissionParams) {
		t.Fatalf("err=%v want context mismatch", err)
	}
}

func TestAdmissionRecordLimitsMatchRecordConstructorBounds(t *testing.T) {
	if validRecordLimit(uint16(tlsrecord.FixedWireOverhead - 1)) {
		t.Fatal("limit below fixed wire overhead accepted")
	}
	if !validRecordLimit(uint16(tlsrecord.FixedWireOverhead)) {
		t.Fatal("minimum legal wire limit rejected")
	}
	if !validRecordLimit(uint16(tlsrecord.MaxWireLen)) {
		t.Fatal("maximum legal wire limit rejected")
	}
	if validRecordLimit(uint16(tlsrecord.MaxWireLen + 1)) {
		t.Fatal("limit above record implementation maximum accepted")
	}
}
