//go:build linux

package platformproxy

import (
	"net"
	"testing"
)

func TestTCPOriginalDstUsesAcceptedLocalAddress(t *testing.T) {
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan *net.TCPConn, 1)
	go func() {
		conn, _ := net.DialTCP("tcp4", nil, ln.Addr().(*net.TCPAddr))
		done <- conn
	}()
	accepted, err := ln.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	client := <-done
	if client != nil {
		defer client.Close()
	}
	got, err := TCPOriginalDst(accepted)
	if err != nil {
		t.Fatal(err)
	}
	want := ln.Addr().(*net.TCPAddr).AddrPort()
	if got != want {
		t.Fatalf("original dst=%v want=%v", got, want)
	}
}
