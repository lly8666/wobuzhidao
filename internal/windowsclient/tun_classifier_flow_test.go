package windowsclient

import (
 "encoding/binary"
 "errors"
 "net/netip"
 "testing"
 "time"
)

func TestN4FlowScopedFiveTupleDNSAndGenerationFence(t *testing.T) {
 c,err:=NewTUNScopedFlows(n4Policy(t,true))
 if err!=nil {t.Fatal(err)}
 at:=time.Unix(1700000000,0)
 input:=n4TestIPv4("10.66.0.2","10.4.5.6",17,443,0)
 d,e:=c.Classify(input,12,87,at)
 if e!=nil || d!=TUNDirect {t.Fatalf("LAN: %v %v",d,e)}
 copy(input[16:20],netip.MustParseAddr("8.8.8.8").AsSlice())
 d,e=c.Classify(input,12,87,at)
 if e!=nil || d!=TUNProtected {t.Fatalf("same src port different dst: %v %v",d,e)}
 binary.BigEndian.PutUint16(input[22:24],53)
 d,e=c.Classify(input,12,87,at)
 if e!=nil || d!=TUNDNSProtected {t.Fatalf("same src and dst changed port to DNS: %v %v",d,e)}
 x,y:=c.Stats()
 if x!=3 || y!=0 {t.Fatalf("three bounded independent 5tuple decisions: flows=%d frag=%d",x,y)}
 _,err=c.Classify(input,0,87,at)
 if !errors.Is(err,ErrTUNFlowScope) {t.Fatalf("zero interface: %v",err)}
 _,err=c.Classify(input,12,0,at)
 if !errors.Is(err,ErrTUNFlowScope) {t.Fatalf("zero generation: %v",err)}
 d,e=c.Classify(input,12,88,at)
 if e!=nil || d!=TUNDNSProtected {t.Fatalf("new incarnation: %v %v",d,e)}
 x,_=c.Stats()
 if x!=1 {t.Fatalf("flow cache leaked old generation: %d",x)}
}

func TestN4FlowScopedFragmentAssociationNoGuessNoCrossGeneration(t *testing.T) {
 c,_:=NewTUNScopedFlows(n4Policy(t,true))
 now:=time.Unix(1700000010,0)
 p:=n4TestIPv4("10.66.0.2","10.3.4.5",17,443,0x2000)
 binary.BigEndian.PutUint16(p[4:6],0x33a3)
 later:=append([]byte(nil),p[:20]...)
 binary.BigEndian.PutUint16(later[2:4],20)
 binary.BigEndian.PutUint16(later[6:8],1)
 if _,err:=c.Classify(later,3,99,now);!errors.Is(err,ErrTUNFragmentUnmapped) {
  t.Fatalf("nonfirst fragment must not invent route: %v",err)
 }
 copyBefore:=append([]byte(nil),p...)
 d,err:=c.Classify(p,3,99,now)
 if err!=nil || d!=TUNDirect {t.Fatalf("first protected with direct route: %v %v",d,err)}
 if string(p)!=string(copyBefore) {t.Fatal("classification illegally rewrote original wire bytes")}
 d,err=c.Classify(later,3,99,now.Add(time.Second))
 if err!=nil || d!=TUNDirect {t.Fatalf("bounded later fragment: %v %v",d,err)}
 if _,err:=c.Classify(later,3,99,now.Add(4*time.Second));!errors.Is(err,ErrTUNFragmentUnmapped) {
  t.Fatalf("expired fragment accepted: %v",err)
 }
 d,err=c.Classify(p,3,99,now.Add(5*time.Second))
 if err!=nil || d!=TUNDirect {t.Fatalf("fragment association new: %v %v",d,err)}
 // Same IPv4 ID and interface but different port is ambiguous: fail closed.
 q:=append([]byte(nil),p...)
 binary.BigEndian.PutUint16(q[22:24],53)
 if _,err=c.Classify(q,3,99,now.Add(6*time.Second));!errors.Is(err,ErrTUNFragmentUnmapped) {
  t.Fatalf("same IPv4 fragment ID different flow silently rerouted: %v",err)
 }
 if _,err=c.Classify(later,3,100,now.Add(6*time.Second));!errors.Is(err,ErrTUNFragmentUnmapped) {
  t.Fatalf("old generation fragment reused: %v",err)
 }
 p[12]=10;p[13]=66;p[14]=0;p[15]=4
 if _,err=c.Classify(p,3,100,now.Add(6*time.Second));err==nil {
  t.Fatal("spoofed TUN first fragment accepted")
 }
}

func TestN4FlowScopedBoundedNoUnboundedState(t *testing.T) {
 c,_:=NewTUNScopedFlows(n4Policy(t,true))
 now:=time.Unix(1700000030,0)
 p:=n4TestIPv4("10.66.0.2","8.8.8.8",17,443,0)
 for i:=0;i<N4MaxFlowAssociations+120;i++ {
  binary.BigEndian.PutUint16(p[20:22],uint16(1024+i))
  d,e:=c.Classify(p,12,54,now)
  if e!=nil||d!=TUNProtected {t.Fatalf("loop %d: %v %v",i,d,e)}
  flows,frags:=c.Stats()
  if flows>N4MaxFlowAssociations || frags!=0 {t.Fatalf("unbounded cache: %d/%d",flows,frags)}
 }
 flows,_:=c.Stats()
 if flows!=N4MaxFlowAssociations {t.Fatalf("wrong fixed flow cap %d",flows)}
 // Reconnect without OS routing changes purges decisions at the next packet.
 _,e:=c.Classify(p,12,55,now.Add(time.Second))
 if e!=nil {t.Fatal(e)}
 flows,_=c.Stats()
 if flows!=1 {t.Fatalf("generation cache fence not applied: %d",flows)}
}

func TestN4FlowScopedDropUnsupportedFirstFragment(t *testing.T) {
 c,_:=NewTUNScopedFlows(n4Policy(t,false))
 at:=time.Unix(1700000040,0)
 p:=n4TestIPv4("10.66.0.2","192.168.1.2",17,443,0x2000)
 p=p[:23]
 binary.BigEndian.PutUint16(p[2:4],uint16(len(p)))
 if _,err:=c.Classify(p,1,1,at);!errors.Is(err,ErrInvalidIPv4) {
  t.Fatalf("short UDP first fragment: %v",err)
 }
}
