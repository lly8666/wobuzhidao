package runtimeowner

import (
    "net/netip"
    "fmt"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/faketcp"
)

// Real protected V3 owner/Runtime -> FakeTCP ciphertext -> opposite Runtime.
// Continue sending throughout all maturation periods: no drain or idle pause.
// An in-process path isolates measurement state from host netns capacity;
// native sustained 10Mbps qualification is a separate, still-required gate.
func TestN1QualityV3ContinuousBusinessEchoMaturity(t *testing.T) {
    for _,load:=range []int{180,640} {
    for _,tc:=range []struct{name string;up,down bool}{
        {"upload",true,false},
        {"download",false,true},
        {"bidirectional",true,true},
    }{
        t.Run(fmt.Sprintf("%s-%dPN-per-2s",tc.name,load),func(t *testing.T){
            lease:=runtimeLease(t)
            leaseAddr,_:=lease.Config.LeaseIPv4()
            cOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
            if err!=nil {t.Fatal(err)}
            sOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
            if err!=nil {t.Fatal(err)}
            client,err:=New(cOwner,nil)
            if err!=nil {t.Fatal(err)}
            defer client.Close()
            server,err:=New(sOwner,nil)
            if err!=nil {t.Fatal(err)}
            defer server.Close()
            var c2s,s2c []faketcp.Segment
            cc,sc:=transportPair(
                func(seg faketcp.Segment)error{c2s=append(c2s,seg);return nil},
                func(seg faketcp.Segment)error{s2c=append(s2c,seg);return nil},
                1,55126)
            cl:=runtimeLane(t,datapath.RoleClient,lease,0,126)
            sl:=runtimeLane(t,datapath.RoleServer,lease,0,126)
            cr,err:=client.AttachInitial(1,cl,cc)
            if err!=nil {t.Fatal(err)}
            sr,err:=server.AttachInitial(1,sl,sc)
            if err!=nil {t.Fatal(err)}
            nonce:=cl.Config().IncarnationNonce
            for _,side:=range []struct{rt *Runtime;refIsClient bool}{{client,true},{server,false}}{
                ref:=sr.Ref
                if side.refIsClient {ref=cr.Ref}
                if err:=side.rt.ConfigureHealth(ref,15*time.Second,nil);err!=nil {t.Fatal(err)}
                if err:=side.rt.ConfigureQualityV3(ref,nonce);err!=nil {t.Fatal(err)}
            }
            ci,si:=0,0
            pump:=func(now time.Time){
                t.Helper()
                for tries:=0;tries<48;tries++ {
                    beforeC,beforeS:=len(c2s),len(s2c)
                    for ci<len(c2s) {
                        seg:=c2s[ci];ci++
                        if err:=server.HandleSegment(sr.Ref,seg,now);err!=nil {t.Fatalf("c2s HandleSegment: %v",err)}
                    }
                    for si<len(s2c) {
                        seg:=s2c[si];si++
                        if err:=client.HandleSegment(cr.Ref,seg,now);err!=nil {t.Fatalf("s2c HandleSegment: %v",err)}
                    }
                    if ci==len(c2s)&&si==len(s2c) {return}
                    if beforeC==len(c2s)&&beforeS==len(s2c) {return}
                }
                t.Fatal("bounded encrypted segment and ACK delivery did not settle")
            }
            send:=func(up bool,n int,now time.Time){
                t.Helper()
                src,dst:=netip.MustParseAddr("1.1.1.1"),leaseAddr
                owner,rt:=sOwner,server
                if up {src,dst=leaseAddr,netip.MustParseAddr("1.1.1.1");owner,rt=cOwner,client}
                for i:=0;i<n;i++{
                    packet:=runtimeIPv4(src,dst,[]byte{byte(i),byte(i>>8),18,19})
                    records,err:=owner.NormalOutbound(packet,now)
                    if err!=nil||len(records)!=1 {t.Fatalf("normal outbound up=%v i=%d got=%d err=%v",up,i,len(records),err)}
                    if err:=rt.SendNormal(records,now);err!=nil {t.Fatal(err)}
                    if load>256 && (i+1)%64==0 {pump(now.Add(20*time.Millisecond))}
                }
            }
            start:=time.Unix(15000,0)
            if err:=client.Tick(start);err!=nil {t.Fatal(err)}
            if err:=server.Tick(start);err!=nil {t.Fatal(err)}
            pump(start)
            seenUp:=map[uint64]bool{}
            seenDown:=map[uint64]bool{}
            for cycle:=1;cycle<=9;cycle++ {
                now:=start.Add(time.Duration(cycle)*2*time.Second)
                // >256 fresh DATA PNs in any 3s maturation interval;
                // each 2s source report window changes while business stays active.
                if tc.up {send(true,load,now.Add(-100*time.Millisecond))}
                if tc.down {send(false,load,now.Add(-100*time.Millisecond))}
                pump(now.Add(-50*time.Millisecond))
                if err:=client.Tick(now);err!=nil {t.Fatal(err)}
                if err:=server.Tick(now);err!=nil {t.Fatal(err)}
                pump(now)
                if tc.up {
                    e,ok:=client.QualityEchoFeedback(cr.Ref,now)
                    if ok&&e.Valid&&e.State==datapath.QualityFeedbackEstimated {
                        if e.Fraction!=0 {t.Fatalf("no impairment must be zero estimated deficit: %+v",e)}
                        seenUp[e.SourceReportSeq]=true
                    }
                }
                if tc.down {
                    e,ok:=server.QualityEchoFeedback(sr.Ref,now)
                    if ok&&e.Valid&&e.State==datapath.QualityFeedbackEstimated {
                        if e.Fraction!=0 {t.Fatalf("no impairment must be zero estimated deficit: %+v",e)}
                        seenDown[e.SourceReportSeq]=true
                    }
                }
            }
            if tc.up&&len(seenUp)<2 {t.Fatalf("continuous upload: only %d independent matured correlated echoes, want >=2 without stopping traffic",len(seenUp))}
            if tc.down&&len(seenDown)<2 {t.Fatalf("continuous download: only %d independent matured correlated echoes, want >=2 without stopping traffic",len(seenDown))}
            if tc.up {
                st,ok:=server.ControlStats(sr.Ref)
                if !ok || st.MatureEchoEstimated<2 || st.MatureEchoLastSenderSuccess<16 {
                    t.Fatalf("continuous upload requires >=2 emitted mature ESTIMATED echoes: %+v enabled=%v",st,ok)
                }
            }
            if tc.down {
                st,ok:=client.ControlStats(cr.Ref)
                if !ok || st.MatureEchoEstimated<2 || st.MatureEchoLastSenderSuccess<16 {
                    t.Fatalf("continuous download requires >=2 emitted mature ESTIMATED echoes: %+v enabled=%v",st,ok)
                }
            }
        })
    }
    }
}
