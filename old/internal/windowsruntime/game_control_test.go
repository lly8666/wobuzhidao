package windowsruntime

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestSetGameLaneTargetsDormantAndWake(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:0})
	if err != nil { t.Fatal(err) }
	defer server.Close()
	requests := make(chan gamelane.LaneSetCommand, 2)
	go func(){
		buf:=make([]byte,4096)
		for i:=0;i<2;i++{
			n,peer,err:=server.ReadFromUDP(buf);if err!=nil{return}
			cmd,err:=gamelane.ParseLaneSetCommand(buf[:n]);if err!=nil{return}
			requests<-cmd
			active:=make([]uint8,len(cmd.Lanes));for j,lane:=range cmd.Lanes{active[j]=lane.ID}
			wire,_:=json.Marshal(gamelane.LaneControlReply{OK:true,Active:active})
			_,_=server.WriteToUDP(wire,peer)
		}
	}()

	control:=server.LocalAddr().String()
	if err:=setGameLaneTargets(control,nil,time.Second);err!=nil{t.Fatal(err)}
	wake:=[]gamelane.LaneTarget{{ID:2,Address:"127.0.0.1:47102"},{ID:1,Address:"127.0.0.1:47101"}}
	if err:=setGameLaneTargets(control,wake,time.Second);err!=nil{t.Fatal(err)}
	first:=<-requests;second:=<-requests
	if len(first.Lanes)!=0{t.Fatalf("dormant lanes=%v",first.Lanes)}
	if len(second.Lanes)!=2{t.Fatalf("wake lanes=%v",second.Lanes)}
}

func TestSetGameLaneTargetsRejectsNonLoopbackControl(t *testing.T) {
	if err:=setGameLaneTargets("10.0.0.1:48102",nil,10*time.Millisecond);err==nil{t.Fatal("non-loopback Game control accepted")}
}
