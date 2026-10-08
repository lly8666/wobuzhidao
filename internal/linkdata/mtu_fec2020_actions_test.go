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

 "github.com/lly8666/wobuzhidao/internal/fec"
 "github.com/lly8666/wobuzhidao/internal/pathmtu"
)

// One Actions job = one outer-MTU/loss/traffic scenario. This exercises the
// actual WBD LINK + fixed FEC 20:20 systematic/parity encoder and decoder;
// no ciphertext, Windows driver, or full WBD client/server socket is implied.
func TestActionsMTUFEC2020Wire(t *testing.T) {
 if os.Getenv("GITHUB_ACTIONS")!="true" || os.Getenv("WBD_FEC2020_MATRIX")!="1" {
  t.Skip("separate GitHub Actions FEC20:20 qualification")
 }
 kind:=os.Getenv("WBD_MTU_KIND")
 if kind!="tcp"&&kind!="udp"&&kind!="udp-jumbo" {t.Fatalf("invalid traffic kind %q",kind)}
 outer,e:=strconv.Atoi(os.Getenv("WBD_MTU_OUTER"))
 if e!=nil || (outer!=1300 && outer!=1400) {t.Fatalf("outer=%d err=%v",outer,e)}
 loss,e:=strconv.Atoi(os.Getenv("WBD_MTU_LOSS"))
 if e!=nil || (loss!=0 && loss!=5 && loss!=10) {t.Fatalf("loss=%d err=%v",loss,e)}
 cap,e:=pathmtu.ResolveConfiguredRecordWireLimit(outer,20,20,0)
 if e!=nil || cap!=outer-40 {t.Fatalf("auto record wire limit=%d err=%v",cap,e)}
 b,e:=pathmtu.Derive(pathmtu.Config{
  ConnectionMTU:outer,IPv4HeaderLen:20,TCPHeaderLen:20,
  PeerMSS:uint16(outer-40),PeerMSSSet:true,
  RecordWireLimit:cap,ParityShards:20,
 })
 if e!=nil {t.Fatal(e)}
 expectedLINK:=outer-40-31-56
 if b.LinkFrameMTU!=expectedLINK || b.FECOverhead!=56 {t.Fatalf("FEC20:20 geometry: %+v",b)}
 tun,e:=pathmtu.DeriveTunnelInterfaceMTU(pathmtu.Config{
  ConnectionMTU:outer,IPv4HeaderLen:20,TCPHeaderLen:20,
  RecordWireLimit:cap,ParityShards:20,
 })
 if e!=nil || tun!=expectedLINK {t.Fatalf("FEC20 TUN MTU=%d want=%d err=%v",tun,expectedLINK,e)}

 cfg:=FECPathConfig{SourceMTU:b.LinkFrameMTU,ParityShards:20,FlushAfter:8*time.Millisecond,MaxBlocks:64}
 tx,e:=NewFECPath(cfg);if e!=nil {t.Fatal(e)}
 rx,e:=NewFECPath(cfg);if e!=nil {t.Fatal(e)}
 if !tx.FECEnabled() || tx.FECWireLimit()!=b.RecordPayloadMTU {
  t.Fatalf("wrong FEC20:20 TX setup: %+v",tx.State())
 }
 var sizes []int
 switch kind {
 case "tcp":sizes=[]int{8,31,96,512,1024,tun-41,tun-40,tun-39,tun,1500,4096,8192}
 case "udp":sizes=[]int{8,32,96,512,1000,tun-29,tun-28,tun-27,tun+1,1300,1500,4096,8936}
 case "udp-jumbo":sizes=[]int{8936,8937,8972,8973,9000,16000,32768,65507}
 }
 rounds:=10
 if kind=="udp-jumbo" {rounds=4}
 rng:=rand.New(rand.NewSource(int64(20261008+outer*100+loss*1000)))
 expected:=make(map[uint16][]byte)
 var id uint16
 now:=time.Unix(45000,0)
 sent,received,wireSource,wireParity,lostSource,lostParity,recoveredDatagrams,overMTU:=0,0,0,0,0,0,0,0
 for round:=0;round<rounds;round++ {
  for _,n:=range sizes {
   id++
   proto,header:=byte(17),28
   if kind=="tcp" {proto,header=6,40}
   pkt:=make([]byte,header+n)
   pkt[0],pkt[8],pkt[9]=0x45,64,proto
   binary.BigEndian.PutUint16(pkt[2:4],uint16(len(pkt)))
   binary.BigEndian.PutUint16(pkt[4:6],id)
   copy(pkt[12:16],[]byte{198,18,44,1})
   copy(pkt[16:20],[]byte{198,18,44,2})
   binary.BigEndian.PutUint16(pkt[20:22],18446)
   binary.BigEndian.PutUint16(pkt[22:24],45000)
   if proto==17 {binary.BigEndian.PutUint16(pkt[24:26],uint16(n+8))} else {pkt[32],pkt[33]=0x50,0x18}
   for i:=header;i<len(pkt);i++ {pkt[i]=byte((int(id)*17+i)%256)}
   expected[id]=pkt
   sent++
   if len(pkt)>tun {overMTU++}
   systematic,err:=tx.Encode(pkt,now)
   if err!=nil {t.Fatalf("FEC20 encode inner=%d: %v",len(pkt),err)}
   // Production FEC may generate parity for complete 20-source blocks during
   // Encode. A separate 8ms time advance flushes remaining partial blocks.
   parity,err:=tx.FlushDue(now.Add(12*time.Millisecond))
   if err!=nil {t.Fatalf("FEC20 parity flush: %v",err)}
   wires:=make([][]byte,0,len(systematic)+len(parity))
   wires=append(wires,systematic...)
   wires=append(wires,parity...)
   sourceForPacket:=0
   lostAnySource:=false
   for i,w:=range wires {
    h,err:=fec.ParseBlockHeader(w)
    if err!=nil {t.Fatalf("invalid FEC20 header: %v",err)}
    if int(h.EffectiveParityCount())!=20 {t.Fatalf("FEC profile not 20:20: %+v",h)}
    if len(w)+b.TLSLikeOverhead>b.RecordWireMTU {
     t.Fatalf("FEC20 record over limit: %d > %d",len(w)+b.TLSLikeOverhead,b.RecordWireMTU)
    }
    source:=int(h.ShardIndex)<fec.DataShards
    if source {sourceForPacket++;wireSource++} else {wireParity++}
    if rng.Intn(100)<loss {
     if source {lostSource++;lostAnySource=true} else {lostParity++}
     continue
    }
    out,err:=rx.Decode(w,now.Add(time.Duration(i)*time.Microsecond))
    if err!=nil {t.Fatalf("FEC20 decode source=%t kind=%s seq=%d: %v",source,kind,id,err)}
    for _,got:=range out {
     if len(got)<20 {t.Fatalf("short delivered IPv4 length=%d",len(got))}
     gotID:=binary.BigEndian.Uint16(got[4:6])
     orig,exists:=expected[gotID]
     if !exists || orig==nil || !bytes.Equal(got,orig) {
      t.Fatalf("duplicate, corrupt or truncated FEC20 delivery id=%d bytes=%d",gotID,len(got))
     }
     expected[gotID]=nil
     received++
    }
   }
   wantSource:=1
   if len(pkt)>tun {
    fragPayload:=tun-LinkFragmentHeaderLen
    wantSource=(len(pkt)+fragPayload-1)/fragPayload
   }
   if sourceForPacket!=wantSource {t.Fatalf("wrong LINK fragments inner=%d sources=%d want=%d",len(pkt),sourceForPacket,wantSource)}
   if lostAnySource && expected[id]==nil {recoveredDatagrams++}
   now=now.Add(30*time.Millisecond)
  }
 }
 if wireParity==0 {t.Fatal("FEC20:20 generated no parity")}
 if loss==0 && received!=sent {t.Fatalf("0pct FEC20 delivery=%d/%d",received,sent)}
 if loss!=0 {
  if lostSource+lostParity==0 {t.Fatal("loss scenario erased no records")}
  if rx.State().Decoder.RecoveredSources==0 {t.Fatal("FEC20 parity recovered zero erased systematic sources")}
  if kind!="udp-jumbo" && received<sent/2 {t.Fatalf("FEC20 unexpected low delivery: %d/%d",received,sent)}
 }
 if kind=="udp-jumbo"&&overMTU!=sent {t.Fatalf("jumbo boundary missing: %d/%d",overMTU,sent)}
 st:=rx.State()
 report:=map[string]any{
  "status":"PASS","scope":"WBD_LINK_FEC20_20_ONLY_NO_TLS_OR_PRODUCT_INGRESS",
  "outer_mtu":outer,"inner_tun_mtu":tun,"record_limit":cap,"link_frame_mtu":b.LinkFrameMTU,
  "kind":kind,"loss_percent":loss,"sent":sent,"delivered":received,
  "sources":wireSource,"parity":wireParity,"lost_sources":lostSource,"lost_parity":lostParity,
  "recovered_datagrams_with_source_loss":recoveredDatagrams,
  "decoder_recovered_sources":st.Decoder.RecoveredSources,
  "decoder_pressure_retirements":st.Decoder.PressureRetirements,
  "partial_blocks":tx.State().Encoder.PartialBlocks,
  "full_blocks":tx.State().Encoder.FullBlocks,
  "oversize_inner_datagrams":overMTU,
  "large_udp_product_ingress_qualification":"NOT_RUN",
 }
 line,_:=json.Marshal(report)
 t.Logf("WBD_FEC2020_MATRIX %s",line)
}
