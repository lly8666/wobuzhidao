package main

import (
	"encoding/hex"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

func TestGameServerRebindDormantAndWake(t *testing.T) {
	echo, err := net.ListenUDP("udp4", &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:0})
	if err != nil { t.Fatal(err) }
	defer echo.Close()
	metaPeer := make(chan *net.UDPAddr, 1)
	go func(){
		buf:=make([]byte,65535)
		for{
			n,peer,err:=echo.ReadFromUDP(buf); if err!=nil{return}
			if _,ok:=rawipbackend.UnmarshalTunnelMeta(buf[:n]);ok{
				select{case metaPeer<-peer:default:}
			}
		}
	}()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:0})
	if err != nil { t.Fatal(err) }
	s:=&server{conn:conn,serviceAddr:echo.LocalAddr().(*net.UDPAddr),replayWindow:4096,maxSessions:4,maxLanes:4,idle:time.Minute,sessions:map[gamelane.SessionID]*gameSession{},peerSession:map[string]gamelane.SessionID{},peerMeta:map[string]rawipbackend.TunnelMeta{}}
	defer s.Close()

	var sid gamelane.SessionID
	for i:=range sid{sid[i]=byte(i+1)}
	meta:=rawipbackend.TunnelMeta{TunnelID:logicaltunnel.TunnelID(hex.EncodeToString(sid[:])),Address4:netip.MustParseAddr("10.66.0.1")}
	oldPeer:=&net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:31001}
	newPeer:=&net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:31002}
	wakePeer:=&net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:31003}

	if err:=s.registerPeerMeta(oldPeer,meta,time.Now());err!=nil{t.Fatal(err)}
	gs,err:=s.bindLane(sid,1,oldPeer,meta,time.Now());if err!=nil{t.Fatal(err)}
	select{case <-metaPeer:case <-time.After(time.Second):t.Fatal("downstream metadata not registered")}

	if err:=s.registerPeerMeta(newPeer,meta,time.Now());err!=nil{t.Fatal(err)}
	if _,err:=s.bindLane(sid,1,newPeer,meta,time.Now());err!=nil{t.Fatal(err)}
	gs.mu.Lock(); rebound:=gs.lanes[1]; bound:=len(gs.lanes); gs.mu.Unlock()
	if bound!=1 || rebound==nil || rebound.String()!=newPeer.String(){t.Fatalf("rebind lanes=%d peer=%v",bound,rebound)}
	s.mu.Lock(); _,oldMeta:=s.peerMeta[oldPeer.String()]; _,oldSession:=s.peerSession[oldPeer.String()]; s.mu.Unlock()
	if oldMeta||oldSession{t.Fatal("old lane peer remained authoritative after authenticated rebind")}

	leaveWire,err:=gamelane.MarshalLaneLeave(sid,1);if err!=nil{t.Fatal(err)}
	leave,err:=gamelane.ParseMembershipControl(leaveWire);if err!=nil{t.Fatal(err)}
	if err:=s.handleMembership(newPeer,leave,time.Now());err!=nil{t.Fatal(err)}
	gs.mu.Lock(); dormantLanes:=len(gs.lanes); gs.mu.Unlock()
	if dormantLanes!=0{t.Fatalf("dormant lanes=%d",dormantLanes)}
	s.mu.Lock(); if s.sessions[sid]!=gs{s.mu.Unlock();t.Fatal("Logical Tunnel session removed on lane leave")}; s.mu.Unlock()

	// Deliver a downstream packet while no lane is active. The service goroutine
	// must stay alive and account a dormant drop instead of returning forever.
	select{
	case peer:=<-metaPeer:
		if _,err:=echo.WriteToUDP([]byte("dormant-downlink"),peer);err!=nil{t.Fatal(err)}
	case <-time.After(20*time.Millisecond):
		// The first metadata peer was already consumed. service.LocalAddr is the
		// connected UDP source seen by echo, so obtain it directly.
		if _,err:=echo.WriteToUDP([]byte("dormant-downlink"),gs.service.LocalAddr().(*net.UDPAddr));err!=nil{t.Fatal(err)}
	}
	deadline:=time.Now().Add(time.Second)
	for{
		gs.mu.Lock(); drops:=gs.dormantDrop; gs.mu.Unlock()
		if drops>0{break}
		if time.Now().After(deadline){t.Fatal("dormant downstream did not stay in live service loop")}
		time.Sleep(time.Millisecond)
	}

	if err:=s.registerPeerMeta(wakePeer,meta,time.Now());err!=nil{t.Fatal(err)}
	if _,err:=s.bindLane(sid,1,wakePeer,meta,time.Now());err!=nil{t.Fatal(err)}
	gs.mu.Lock(); wake:=gs.lanes[1]; lanes:=len(gs.lanes); gs.mu.Unlock()
	if lanes!=1 || wake==nil || wake.String()!=wakePeer.String(){t.Fatalf("wake lanes=%d peer=%v",lanes,wake)}
}
