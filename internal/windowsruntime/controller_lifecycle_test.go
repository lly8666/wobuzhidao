package windowsruntime

import (
	"encoding/json"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func gameControlResponder(t *testing.T, requests int) (string, <-chan gamelane.LaneSetCommand) {
	t.Helper()
	conn,err:=net.ListenUDP("udp4",&net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:0});if err!=nil{t.Fatal(err)}
	t.Cleanup(func(){_ = conn.Close()})
	seen:=make(chan gamelane.LaneSetCommand,requests)
	go func(){
		buf:=make([]byte,4096)
		for i:=0;i<requests;i++{
			n,peer,err:=conn.ReadFromUDP(buf);if err!=nil{return}
			cmd,err:=gamelane.ParseLaneSetCommand(buf[:n]);if err!=nil{return}
			seen<-cmd
			active:=make([]uint8,len(cmd.Lanes));for j,lane:=range cmd.Lanes{active[j]=lane.ID}
			wire,_:=json.Marshal(gamelane.LaneControlReply{OK:true,Active:active});_,_=conn.WriteToUDP(wire,peer)
		}
	}()
	return conn.LocalAddr().String(),seen
}

func setControllerGameControl(c *Controller, addr string){c.mu.Lock();c.gameControl=addr;c.mu.Unlock()}

func TestControllerDormantWakeKeepsSharedGameTunAndNetwork(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);p:=testProfile();p.TunnelIPv4="";p.Lanes=2
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	control,requests:=gameControlResponder(t,2);setControllerGameControl(c,control)
	cut:=len(r.events)
	if err:=c.Dormant();err!=nil{t.Fatal(err)}
	if c.State()!=RuntimeDormant{t.Fatalf("state=%s",c.State())}
	for _,ev:=range r.events[cut:]{if ev=="stop:game"||ev=="stop:tun"||strings.HasPrefix(ev,"run:route-")||strings.HasPrefix(ev,"run:ipv6-"){t.Fatalf("dormant disturbed shared network: %v",r.events[cut:])}}
	first:=<-requests;if len(first.Lanes)!=0{t.Fatalf("dormant Game lanes=%v",first.Lanes)}

	wakeCut:=len(r.events)
	if err:=c.Wake();err!=nil{t.Fatal(err)}
	if c.State()!=RuntimeConnected{t.Fatalf("wake state=%s",c.State())}
	second:=<-requests;if len(second.Lanes)!=2{t.Fatalf("wake Game lanes=%v",second.Lanes)}
	for _,ev:=range r.events[wakeCut:]{if ev=="start:game"||ev=="start:tun"||ev=="run:route-apply"||ev=="run:ipv6-apply"{t.Fatalf("wake restarted shared network: %v",r.events[wakeCut:])}}
	if got:=c.executor.DynamicLaneIDs();!reflect.DeepEqual(got,[]int{1,2}){t.Fatalf("wake lanes=%v",got)}
}

func TestControllerReplaceLaneUsesMBBAndAlternatesTransportSlot(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	control,requests:=gameControlResponder(t,2);setControllerGameControl(c,control)
	c.mu.Lock();before:=c.lifecycle.Snapshot()[0].Ref;c.mu.Unlock()

	cut:=len(r.events)
	if err:=c.ReplaceLane(1);err!=nil{t.Fatal(err)}
	first:=<-requests;if len(first.Lanes)!=1||first.Lanes[0].ID!=1||first.Lanes[0].Address!="127.0.0.1:47105"{t.Fatalf("first promotion=%v",first.Lanes)}
	c.mu.Lock();firstPlan:=c.lanePlans[1];after:=c.lifecycle.Snapshot()[0].Ref;c.mu.Unlock()
	if firstPlan.Slot!=5{t.Fatalf("first replacement slot=%d",firstPlan.Slot)}
	if after.Generation<=before.Generation{t.Fatalf("generation before=%+v after=%+v",before,after)}
	oldStop:=[]string{"stop:link-1","stop:dtls-1","stop:faketcp-1"}
	if !containsOrderedSubsequence(r.events[cut:],oldStop){t.Fatalf("old lane was not retired after promotion: %v",r.events[cut:])}

	if err:=c.ReplaceLane(1);err!=nil{t.Fatal(err)}
	second:=<-requests;if len(second.Lanes)!=1||second.Lanes[0].Address!="127.0.0.1:47101"{t.Fatalf("second promotion=%v",second.Lanes)}
	c.mu.Lock();secondPlan:=c.lanePlans[1];c.mu.Unlock()
	if secondPlan.Slot!=1{t.Fatalf("second replacement slot=%d",secondPlan.Slot)}
}

func TestControllerReplaceCandidateFailureLeavesOldLaneAndGameUntouched(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	control,_:=gameControlResponder(t,1);setControllerGameControl(c,control)
	r.failReady="dtls-1-candidate-s5"
	c.mu.Lock();oldPlan:=c.lanePlans[1];oldRef:=c.lifecycle.Snapshot()[0].Ref;c.mu.Unlock()
	if err:=c.ReplaceLane(1);err==nil{t.Fatal("expected candidate health failure")}
	c.mu.Lock();gotPlan:=c.lanePlans[1];gotRef:=c.lifecycle.Snapshot()[0].Ref;state:=c.state;c.mu.Unlock()
	if gotPlan.Slot!=oldPlan.Slot||gotRef!=oldRef||state!=RuntimeConnected{t.Fatalf("old authority changed plan=%+v ref=%+v state=%s",gotPlan,gotRef,state)}
}

func containsOrderedSubsequence(events,want []string)bool{j:=0;for _,ev:=range events{if j<len(want)&&ev==want[j]{j++}};return j==len(want)}

func TestGameControlResponderDoesNotHang(t *testing.T){
	addr,seen:=gameControlResponder(t,1)
	if err:=setGameLaneTargets(addr,nil,time.Second);err!=nil{t.Fatal(err)}
	select{case <-seen:case <-time.After(time.Second):t.Fatal("control request not observed")}
}
