package main

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func loopbackUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1), Port:0})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = c.Close() })
	return c
}

func TestDynamicLaneMembershipDormantWake(t *testing.T) {
	a := loopbackUDP(t)
	b := loopbackUDP(t)
	c := &client{lanes: make(map[uint8]*laneConn, gamelane.MaxLanes)}
	t.Cleanup(c.closeAllLanes)

	lane1 := gamelane.LaneTarget{ID:1, Address:a.LocalAddr().String()}
	lane2 := gamelane.LaneTarget{ID:2, Address:b.LocalAddr().String()}
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{lane1}); err != nil || !reflect.DeepEqual(got, []uint8{1}) {
		t.Fatalf("lane1 got=%v err=%v", got, err)
	}
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{lane1,lane2}); err != nil || !reflect.DeepEqual(got, []uint8{1,2}) {
		t.Fatalf("race got=%v err=%v", got, err)
	}
	if got, err := c.setLaneTargets(nil); err != nil || len(got) != 0 {
		t.Fatalf("dormant got=%v err=%v", got, err)
	}
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{lane2}); err != nil || !reflect.DeepEqual(got, []uint8{2}) {
		t.Fatalf("wake got=%v err=%v", got, err)
	}
}

func TestLaneFailureDoesNotKillOtherMembership(t *testing.T) {
	a := loopbackUDP(t)
	b := loopbackUDP(t)
	c := &client{lanes: make(map[uint8]*laneConn, gamelane.MaxLanes)}
	t.Cleanup(c.closeAllLanes)
	_, err := c.setLaneTargets([]gamelane.LaneTarget{{ID:1,Address:a.LocalAddr().String()},{ID:2,Address:b.LocalAddr().String()}})
	if err != nil { t.Fatal(err) }
	lanes := c.activeLanes()
	if len(lanes) != 2 { t.Fatalf("lanes=%d", len(lanes)) }
	c.failLane(lanes[0], errors.New("synthetic lane failure"))
	if got := c.activeIDs(); !reflect.DeepEqual(got, []uint8{2}) {
		t.Fatalf("surviving lanes=%v", got)
	}
}
