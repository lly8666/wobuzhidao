package linkdata

import (
 "bytes"
 "encoding/binary"
 "encoding/json"
 "math/rand"
 "os"
 "strconv"
 "testing"
 "time"

 "github.com/lly8666/wobuzhidao/internal/pathmtu"
)

// Actions-only real WBD LINK fragmentation/reassembly; erasures are not TCP
// retransmission, full TLS-like encryption, TUN injection or WBD end-to-end.
func TestActionsMTULossWire(t *testing.T) {
 if os.Getenv("GITHUB_ACTIONS")!="true" || os.Getenv("WBD_MTU_MATRIX")!="1" { t.Skip("Actions-only matrix") }
 kind:=os.Getenv("WBD_MTU_KIND")
 if kind!="tcp" && kind!="udp" && kind!="udp-jumbo" { t.Fatalf("kind %q",kind) }
 outer,e:=strconv.Atoi(os.Getenv("WBD_MTU_OUTER"));if e!=nil || (outer!=1300 && outer!=1400) {t.Fatalf("outer %d: %v",outer,e)}
 loss,e:=strconv.Atoi(os.Getenv("WBD_MTU_LOSS"));if e!=nil || (loss!=0 && loss!=5 && loss!=10) {t.Fatalf("loss %d: %v",loss,e)}

 // Stricter of the two default 1300/1250 record directions, FEC off.
 b,e:=pathmtu.Derive(pathmtu.Config{ConnectionMTU:outer,IPv4HeaderLen:20,TCPHeaderLen:20,PeerMSS:uint16(outer-40),PeerMSSSet:true,RecordWireLimit:1250,ParityShards:0})
 if e!=nil {t.Fatal(e)}
 if b.LinkFrameMTU!=1219 {t.Fatalf("LINK budget=%d",b.LinkFrameMTU)}
 tx,e:=NewFECPath(FECPathConfig{SourceMTU:b.LinkFrameMTU});if e!=nil {t.Fatal(e)}
 rx,e:=NewFECPath(FECPathConfig{SourceMTU:b.LinkFrameMTU});if e!=nil {t.Fatal(e)}
 var sizes []int
 switch kind {
 case "tcp":sizes=[]int{8,31,96,512,1024,1179,1180,1181,1200,1460,4096,8192}
 case "udp":sizes=[]int{8,32,96,512,1000,1191,1192,1193,1300,1500,4096,8936}
 case "udp-jumbo":sizes=[]int{8936,8937,8972,8973,9000,16000,32768,65507}
 }
 expected:=make(map[uint16][]byte)
 rng:=rand.New(rand.NewSource(int64(20261008+outer*100+loss)))
 now:=time.Unix(30000,0)
 emitted,erased,received,oversize:=0,0,0,0
 var id uint16
 rounds:=12;if kind=="udp-jumbo" {rounds=6}
 for round:=0;round<rounds;round++ {
  for _,n:=range sizes {
   id++
   proto,head:=byte(17),28;if kind=="tcp" {proto,head=6,40}
   pkt:=make([]byte,head+n)
   pkt[0],pkt[8],pkt[9]=0x45,64,proto
   binary.BigEndian.PutUint16(pkt[2:4],uint16(len(pkt)))
   binary.BigEndian.PutUint16(pkt[4:6],id)
   copy(pkt[12:16],[]byte{198,18,44,1})
   copy(pkt[16:20],[]byte{198,18,44,2})
   binary.BigEndian.PutUint16(pkt[20:22],18446)
   binary.BigEndian.PutUint16(pkt[22:24],45000)
   if proto==17 {binary.BigEndian.PutUint16(pkt[24:26],uint16(n+8))} else {pkt[32],pkt[33]=0x50,0x18}
   for i:=head;i<len(pkt);i++ {pkt[i]=byte((int(id)*17+i)%256)}
   expected[id]=pkt
   if len(pkt)>b.LinkFrameMTU {oversize++}
   wire,err:=tx.Encode(pkt,now);if err!=nil {t.Fatalf("LINK encode %d bytes: %v",len(pkt),err)}
   if len(pkt)<=b.LinkFrameMTU && len(wire)!=1 {t.Fatalf("unnecessary fragmented %d into %d",len(pkt),len(wire))}
   if len(pkt)>b.LinkFrameMTU && len(wire)<=1 {t.Fatalf("no fragmentation %d",len(pkt))}
   for _,w:=range wire {
    emitted++
    if len(w)+b.TLSLikeOverhead>b.RecordWireMTU {t.Fatal("record overflow")}
    if rng.Intn(100)<loss {erased++;continue}
    decoded,err:=rx.Decode(w,now)
    if err!=nil {t.Fatalf("LINK decode: %v",err)}
    for _,got:=range decoded {
     if len(got)<20 {t.Fatal("truncated IPv4")}
     found:=binary.BigEndian.Uint16(got[4:6])
     original,ok:=expected[found]
     if !ok || original==nil || !bytes.Equal(got,original) {t.Fatalf("corrupt/duplicate/truncated IP id=%d bytes=%d",found,len(got))}
     expected[found]=nil
     received++
    }
    now=now.Add(time.Microsecond)
   }
  }
 }
 if loss==0 && received!=len(sizes)*rounds {t.Fatalf("zero-loss delivered %d/%d",received,len(sizes)*rounds)}
 if loss!=0 && erased==0 {t.Fatal("no simulated erasures")}
 if kind!="udp-jumbo" && received==0 {t.Fatal("no delivered payloads")}
 report:=map[string]any{
  "status":"PASS","scope":"WBD_LINK_ONLY_NO_APP_TUN_OR_TLS",
  "outer":outer,"kind":kind,"loss_percent":loss,
  "record_wire_limit":b.RecordWireMTU,"link_frame_mtu":b.LinkFrameMTU,
  "emitted_wire":emitted,"erased_wire":erased,"sent_datagrams":len(sizes)*rounds,
  "delivered_datagrams":received,"oversize_LINK_datagrams":oversize,
  "large_udp_platform_ingress_qualification":"NOT_TESTED",
 }
 if kind=="udp-jumbo" && loss>0 {report["jumbo_delivery_with_erasure"]="NOT_GUARANTEED"}
 output,_:=json.Marshal(report);t.Logf("WBD_MTU_MATRIX %s",output)
}
