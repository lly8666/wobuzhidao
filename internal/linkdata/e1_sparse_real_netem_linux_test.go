//go:build linux

package linkdata

import (
    "bytes"
    "fmt"
    "net"
    "os"
    "testing"
    "time"
)

// TestE1Uniform32msSparseParityWireCost is a deterministic byte-accounting
// proxy, NOT a CPU benchmark. Same 30 sparse 96-byte payloads, same FEC20:20,
// same source MTU, only partial parity deadline 8ms vs 32ms differs.
// The production default is 32ms; explicit 8ms exists only in this test.
func TestE1Uniform32msSparseParityWireCost(t *testing.T) {
    type totals struct { sources, parity, sourceBytes, parityBytes int }
    measure:=func(window time.Duration)totals{
        t.Helper()
        cfg:=FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:window,MaxBlocks:32}
        path,err:=NewFECPath(cfg)
        if err!=nil {t.Fatal(err)}
        base:=time.Unix(20261009,0)
        var out totals
        for i:=0;i<30;i++ {
            now:=base.Add(time.Duration(i)*12*time.Millisecond)
            due,err:=path.FlushDue(now)
            if err!=nil {t.Fatal(err)}
            for _,pkt:=range due {out.parity++;out.parityBytes+=len(pkt)}
            sources,err:=path.Encode(bytes.Repeat([]byte{0x37},96),now)
            if err!=nil || len(sources)!=1 {t.Fatalf("window=%s i=%d sources=%d err=%v",window,i,len(sources),err)}
            for _,pkt:=range sources {out.sources++;out.sourceBytes+=len(pkt)}
        }
        due,err:=path.FlushDue(base.Add(30*12*time.Millisecond+64*time.Millisecond))
        if err!=nil {t.Fatal(err)}
        for _,pkt:=range due {out.parity++;out.parityBytes+=len(pkt)}
        if !path.NextFlushDeadline().IsZero(){t.Fatalf("window=%s timer did not retire",window)}
        return out
    }
    old:=measure(8*time.Millisecond)
    current:=measure(32*time.Millisecond)
    if old.sources!=30 || current.sources!=30 || old.sourceBytes!=current.sourceBytes{
        t.Fatalf("changed immediate systematic traffic 8ms=%+v 32ms=%+v",old,current)
    }
    if current.parity>=old.parity || current.parityBytes>=old.parityBytes{
        t.Fatalf("no sparse parity wire reduction 8ms=%+v 32ms=%+v",old,current)
    }
    t.Logf("E1_UNIFORM32_RESOURCE_PROXY_PASS sparse_sources=30 8ms_parity=%d 32ms_parity=%d 8ms_parity_bytes=%d 32ms_parity_bytes=%d CPU_GAIN=NOT_MEASURED",old.parity,current.parity,old.parityBytes,current.parityBytes)
}

// TestE1Uniform32msReal15msNetemUDP is an isolated Linux loopback socket
// integration probe, not the full FakeTCP/TUN product, Normal/Game 300s
// qualification or CPU benchmark. Requires external "tc qdisc ... delay 15ms"
// applied to lo inside a temporary network namespace; fails if propagation
// looks too fast. No sleeps/timers are added to the production hot path.
func TestE1Uniform32msReal15msNetemUDP(t *testing.T) {
    if os.Getenv("WBD_E1_NETEM_LOOPBACK")!="1"{t.Skip("privileged 15ms netns loopback netem CI only")}
    cfg:=FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:32*time.Millisecond,MaxBlocks:32}
    sender,err:=NewFECPath(cfg);if err!=nil {t.Fatal(err)}
    receiver,err:=NewFECPath(cfg);if err!=nil {t.Fatal(err)}
    addr:=&net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:0}
    rx,err:=net.ListenUDP("udp4",addr);if err!=nil {t.Fatal(err)}
    defer rx.Close()
    tx,err:=net.ListenUDP("udp4",addr);if err!=nil {t.Fatal(err)}
    defer tx.Close()
    type delivered struct{kind string;at time.Time}
    events:=make(chan delivered,8)
    readErrors:=make(chan error,1)
    done:=make(chan struct{})
    go func(){
        defer close(done)
        buf:=make([]byte,2048)
        for{
            if err:=rx.SetReadDeadline(time.Now().Add(250*time.Millisecond));err!=nil{
                readErrors<-err;return
            }
            n,_,err:=rx.ReadFromUDP(buf)
            if err!=nil{
                if ne,ok:=err.(net.Error);ok&&ne.Timeout(){return}
                readErrors<-err;return
            }
            got,err:=receiver.Decode(buf[:n],time.Now())
            if err!=nil{readErrors<-err;return}
            for _,pkt:=range got {
                name:="unknown"
                switch{
                case bytes.Equal(pkt,bytes.Repeat([]byte{0x61},1372)): name="recovered"
                case bytes.Equal(pkt,bytes.Repeat([]byte{0x72},96)): name="fresh"
                }
                select {
                case events<-delivered{kind:name,at:time.Now()}:
                default:readErrors<-fmt.Errorf("unbounded duplicated delivery");return
                }
            }
        }
    }()
    send:=func(wire [][]byte)time.Time{
        t.Helper()
        stamp:=time.Now()
        for _,w:=range wire{
            if _,err:=tx.WriteToUDP(w,rx.LocalAddr().(*net.UDPAddr));err!=nil{t.Fatal(err)}
        }
        return stamp
    }
    sleepUntil:=func(target time.Time){
        if left:=time.Until(target);left>0{time.Sleep(left)}
    }
    t0:=time.Now()
    first:=bytes.Repeat([]byte{0x61},1372)
    shards,err:=sender.Encode(first,t0)
    if err!=nil||len(shards)<2{t.Fatalf("fragmented first packet: fragments=%d err=%v",len(shards),err)}
    // Explicitly erase one systematic LINK/FEC shard before the real socket.
    // All remaining sources traverse Linux UDP loopback with 15ms netem.
    send(shards[1:])
    sleepUntil(t0.Add(32*time.Millisecond))
    parity,err:=sender.FlushDue(time.Now())
    if err!=nil||len(parity)==0{t.Fatalf("32ms parity missing shards=%d err=%v",len(parity),err)}
    paritySent:=send(parity)
    sleepUntil(t0.Add(45*time.Millisecond))
    later:=bytes.Repeat([]byte{0x72},96)
    fresh,err:=sender.Encode(later,time.Now())
    if err!=nil||len(fresh)!=1{t.Fatalf("independent fresh packet sources=%d err=%v",len(fresh),err)}
    freshSent:=send(fresh)
    sleepUntil(freshSent.Add(32*time.Millisecond))
    extra,err:=sender.FlushDue(time.Now())
    if err!=nil{t.Fatal(err)}
    secondParitySent:=send(extra)
    <-done
    close(events)
    select{case err:=<-readErrors:t.Fatalf("receiver: %v",err);default:}
    counts:=map[string]int{}
    moments:=map[string]time.Time{}
    for ev:=range events{
        counts[ev.kind]++
        moments[ev.kind]=ev.at
    }
    if counts["recovered"]!=1||counts["fresh"]!=1||counts["unknown"]!=0{
        t.Fatalf("lost, duplicate or corrupt real-netem UDP arrival: %+v",counts)
    }
    recoveryDelay:=moments["recovered"].Sub(paritySent)
    freshDelay:=moments["fresh"].Sub(freshSent)
    if recoveryDelay<5*time.Millisecond||freshDelay<5*time.Millisecond{
        t.Fatalf("15ms netem was not effective: parity_to_repair=%s fresh_source_to_arrival=%s",recoveryDelay,freshDelay)
    }
    if moments["fresh"].After(secondParitySent){
        t.Fatalf("fresh independent packet stalled behind its own later parity arrival=%v parity_issued=%v",moments["fresh"].Sub(t0),secondParitySent.Sub(t0))
    }
    if !sender.NextFlushDeadline().IsZero(){t.Fatal("sparse partial parity timer did not retire")}
    t.Logf("E1_REAL_15MS_LOOPBACK_FEC20_20_SCOPED_PASS first_source_erased=1 recovered_once=1 fresh_once=1 netem15ms=1 first_repair_from_parity_send_ms=%.3f fresh_from_source_send_ms=%.3f source_default32ms=1 fullstack=NOT_RUN CPU_GAIN=NOT_PROVEN",float64(recoveryDelay.Microseconds())/1000,float64(freshDelay.Microseconds())/1000)
}
