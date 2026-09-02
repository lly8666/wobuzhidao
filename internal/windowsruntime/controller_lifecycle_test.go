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

func TestControllerDormantWakeKeepsSharedTunAndRestoresExactlyOnePublicTransport(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	control,requests:=gameControlResponder(t,2);setControllerGameControl(c,control)
	cut:=len(r.events)
	if err:=c.Dormant();err!=nil{t.Fatal(err)}
	if c.State()!=RuntimeDormant{t.Fatalf("state=%s",c.State())}
	for _,ev:=range r.events[cut:]{if ev=="stop:game"||ev=="stop:tun"||strings.HasPrefix(ev,"run:route-")||strings.HasPrefix(ev,"run:ipv6-"){t.Fatalf("dormant disturbed shared network: %v",r.events[cut:])}}
	first:=<-requests;if len(first.Lanes)!=0{t.Fatalf("dormant local targets=%v",first.Lanes)}

	wakeCut:=len(r.events)
	if err:=c.Wake();err!=nil{t.Fatal(err)}
	if c.State()!=RuntimeConnected{t.Fatalf("wake state=%s",c.State())}
	second:=<-requests;if len(second.Lanes)!=1||second.Lanes[0].ID!=1{t.Fatalf("wake local targets=%v",second.Lanes)}
	for _,ev:=range r.events[wakeCut:]{if ev=="start:game"||ev=="start:tun"||ev=="run:route-apply"||ev=="run:ipv6-apply"{t.Fatalf("wake restarted shared network: %v",r.events[wakeCut:])}}
	if got:=c.executor.DynamicLaneIDs();!reflect.DeepEqual(got,[]int{1}){t.Fatalf("wake lanes=%v",got)}
}

func TestControllerReplaceLaneRejectsMakeBeforeBreakPublicOverlap(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	cut:=len(r.events)
	err:=c.ReplaceLane(1)
	if err==nil||!strings.Contains(err.Error(),"break-before-make"){t.Fatalf("ReplaceLane error=%v",err)}
	for _,ev:=range r.events[cut:]{
		if strings.Contains(ev,"candidate")||strings.HasPrefix(ev,"start:faketcp-")||strings.HasPrefix(ev,"start:dtls-")||strings.HasPrefix(ev,"start:link-"){
			t.Fatalf("replacement created overlapping public candidate: %v",r.events[cut:])
		}
	}
	c.mu.Lock();plans:=cloneLanePlans(c.lanePlans);state:=c.state;c.mu.Unlock()
	if state!=RuntimeConnected||len(plans)!=1{t.Fatalf("replacement rejection mutated state=%s plans=%v",state,plans)}
}

func TestControllerReplaceRejectsNonexistentSecondLane(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);p:=testProfile();p.TunnelIPv4="";p.Lanes=1
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	err:=c.ReplaceLane(2)
	if err==nil||!strings.Contains(err.Error(),"no logical lane 2"){t.Fatalf("ReplaceLane(2) error=%v",err)}
}

func TestGameControlResponderDoesNotHang(t *testing.T){
	addr,seen:=gameControlResponder(t,1)
	if err:=setGameLaneTargets(addr,nil,time.Second);err!=nil{t.Fatal(err)}
	select{case <-seen:case <-time.After(time.Second):t.Fatal("control request not observed")}
}
