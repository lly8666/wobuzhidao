package main

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/session"
)

func TestServiceLoopFragmentsOversizeBackendDatagramAndKeepsAssociationAlive(t *testing.T) {
	serverConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer serverConn.Close()

	peerConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peerConn.Close()

	backendConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer backendConn.Close()

	service, err := net.DialUDP("udp4", nil, backendConn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	plane, err := session.NewDataPlane(1, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}
	var id session.LiveID
	id[0] = 0x42
	peer := peerConn.LocalAddr().(*net.UDPAddr)
	peerKey := peer.String()
	if err := plane.Reserve("solo", id, peerKey, time.Now()); err != nil {
		t.Fatal(err)
	}
	cfg := linkConfigForInner(t, 576, false)
	if err := plane.Activate(id, cfg); err != nil {
		t.Fatal(err)
	}
	decodePath, err := linkdata.New(cfg, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}

	ps := &peerSession{
		peer:    cloneUDPAddr(peer),
		key:     peerKey,
		account: "solo",
		id:      id,
		sid:     "oversize-regression",
		active:  true,
		backend: backendPlatform,
		service: service,
	}
	s := &server{conn: serverConn, plane: plane}
	go s.serviceLoop(ps, service)

	serviceAddr := service.LocalAddr().(*net.UDPAddr)
	oversize := bytes.Repeat([]byte{0x7f}, int(cfg.MTU)+300)
	if _, err := backendConn.WriteToUDP(oversize, serviceAddr); err != nil {
		t.Fatal(err)
	}
	legal := []byte("legal-after-oversize")
	if _, err := backendConn.WriteToUDP(legal, serviceAddr); err != nil {
		t.Fatal(err)
	}

	if err := peerConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65535)
	var got [][]byte
	for len(got) < 2 {
		n, _, err := peerConn.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("backend datagrams were not forwarded after LINK fragmentation: %v", err)
		}
		packets, err := decodePath.Decode(append([]byte(nil), buf[:n]...))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, packets...)
	}
	if !bytes.Equal(got[0], oversize) {
		t.Fatalf("first reconstructed bytes=%d want oversize=%d", len(got[0]), len(oversize))
	}
	if !bytes.Equal(got[1], legal) {
		t.Fatalf("second reconstructed payload=%q want=%q", got[1], legal)
	}
	if gotDrop := ps.drop.Load(); gotDrop != 0 {
		t.Fatalf("drop count=%d want=0", gotDrop)
	}
	if gotTx := ps.linkTx.Load(); gotTx != 2 {
		t.Fatalf("backend tx attempts=%d want=2", gotTx)
	}
}
