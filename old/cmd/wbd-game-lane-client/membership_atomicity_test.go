package main

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestSetLaneTargetsDoesNotCommitPartialMembershipBeforeCandidateReady(t *testing.T) {
	old1 := loopbackUDP(t)
	old2 := loopbackUDP(t)
	candidate := loopbackUDP(t)
	app := loopbackUDP(t)

	var sid gamelane.SessionID
	sid[0] = 0x6b
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }
	pacer, err := gamelane.NewInnerPacer(0)
	if err != nil { t.Fatal(err) }
	c := &client{app: app, enc: enc, pacer: pacer, lanes: make(map[uint8]*laneConn, gamelane.MaxLanes), overlap: make(map[uint8]*laneConn, 1)}
	t.Cleanup(c.closeAllLanes)

	lane1 := gamelane.LaneTarget{ID: 1, Address: old1.LocalAddr().String()}
	lane2 := gamelane.LaneTarget{ID: 2, Address: old2.LocalAddr().String()}
	candidate1 := gamelane.LaneTarget{ID: 1, Address: candidate.LocalAddr().String()}
	if _, err := c.setLaneTargets([]gamelane.LaneTarget{lane1, lane2}); err != nil { t.Fatal(err) }

	candidatePeer := make(chan *net.UDPAddr, 1)
	probeErr := make(chan error, 1)
	go func() {
		_ = candidate.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 256)
		n, peer, err := candidate.ReadFromUDP(buf)
		if err != nil { probeErr <- err; return }
		control, err := gamelane.ParseMembershipControl(buf[:n])
		if err != nil { probeErr <- err; return }
		if control.SessionID != sid || control.LaneID != 1 || control.Op != gamelane.MembershipProbe {
			probeErr <- gamelane.ErrMalformed
			return
		}
		candidatePeer <- peer
		probeErr <- nil
	}()

	setResult := make(chan struct {
		active []uint8
		err error
	}, 1)
	go func() {
		active, err := c.setLaneTargets([]gamelane.LaneTarget{lane1, candidate1})
		setResult <- struct {
			active []uint8
			err error
		}{active: active, err: err}
	}()

	var peer *net.UDPAddr
	select {
	case peer = <-candidatePeer:
	case <-time.After(time.Second):
		t.Fatal("candidate membership Probe was not sent")
	}
	if err := <-probeErr; err != nil { t.Fatal(err) }

	// Lane 2 is being removed by this command. Atomic replacement means that
	// removal cannot become visible until the candidate in the same command has
	// completed its authenticated Probe/Ready qualification.
	if got := c.activeIDs(); !reflect.DeepEqual(got, []uint8{1, 2}) {
		t.Fatalf("membership changed before candidate qualification: active=%v want=[1 2]", got)
	}

	ready, err := gamelane.MarshalLaneReady(sid, 1)
	if err != nil { t.Fatal(err) }
	if _, err := candidate.WriteToUDP(ready, peer); err != nil { t.Fatal(err) }
	select {
	case result := <-setResult:
		if result.err != nil { t.Fatal(result.err) }
		if !reflect.DeepEqual(result.active, []uint8{1}) {
			t.Fatalf("committed active=%v want=[1]", result.active)
		}
	case <-time.After(time.Second):
		t.Fatal("membership update did not finish after candidate ready")
	}
	if got := c.activeIDs(); !reflect.DeepEqual(got, []uint8{1}) {
		t.Fatalf("post-qualification active=%v want=[1]", got)
	}
}
