package main

import (
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestCandidateQualificationWaitsForReadyOnExactLaneSocket(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1)})
	if err != nil { t.Fatal(err) }
	defer server.Close()

	var sid gamelane.SessionID
	for i := range sid { sid[i] = byte(i + 1) }
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }
	remote := server.LocalAddr().(*net.UDPAddr)
	conn, err := net.DialUDP("udp4", nil, remote)
	if err != nil { t.Fatal(err) }
	defer conn.Close()
	lane := &laneConn{id: 2, addr: remote.String(), conn: conn, ready: make(chan struct{})}
	c := &client{enc: enc, lanes: map[uint8]*laneConn{2: lane}, overlap: make(map[uint8]*laneConn)}
	go c.laneLoop(lane)

	serverErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 256)
		n, peer, err := server.ReadFromUDP(buf)
		if err != nil { serverErr <- err; return }
		control, err := gamelane.ParseMembershipControl(buf[:n])
		if err != nil { serverErr <- err; return }
		if control.SessionID != sid || control.LaneID != 2 || control.Op != gamelane.MembershipProbe {
			serverErr <- gamelane.ErrMalformed; return
		}
		ready, err := gamelane.MarshalLaneReady(sid, 2)
		if err != nil { serverErr <- err; return }
		_, err = server.WriteToUDP(ready, peer)
		serverErr <- err
	}()

	if err := c.qualifyLane(lane, time.Second); err != nil { t.Fatal(err) }
	if err := <-serverErr; err != nil { t.Fatal(err) }
}
