package linkdata

import (
    "bytes"
    "fmt"
    "testing"
    "time"
)

// TestE1Uniform32msSparse15msOneWayNoHOL is a deterministic LINK/FEC
// property protector, NOT a netns RTT measurement. Network propagation is
// 15ms in each direction; a new independent packet arrives after a 45ms
// send gap. We erase exactly ONE systematic LINK fragment of the first
// datagram and deliver the scheduled parity after 32ms.
//
// This must not turn into an 8ms special case, classify packet sizes in the
// runtime hot path, or wait for FEC on an intact later independent packet.
// The 8->32ms change does inherently delay first repair by up to 24ms,
// which this test intentionally does NOT hide.
func TestE1Uniform32msSparse15msOneWayNoHOL(t *testing.T) {
    const (
        oneWay = 15 * time.Millisecond
        flushWindow = 32 * time.Millisecond
        secondOffset = 45 * time.Millisecond
        sourceMTU = 1200
    )
    t0 := time.Unix(20261009, 0)

    for _,parity := range []int{4, 20} {
        for _,size := range []int{96, 1372, 4068} {
            t.Run(fmt.Sprintf("fec20_%d_udp_%d",parity,size),func(t *testing.T) {
                cfg:=FECPathConfig{SourceMTU:sourceMTU,ParityShards:parity,
                    FlushAfter:flushWindow,MaxBlocks:32}
                sender,err:=NewFECPath(cfg)
                if err!=nil {t.Fatal(err)}
                receiver,err:=NewFECPath(cfg)
                if err!=nil {t.Fatal(err)}

                first:=bytes.Repeat([]byte{0x71},size)
                later:=bytes.Repeat([]byte{0x82},96)

                sources,err:=sender.Encode(first,t0)
                if err!=nil || len(sources)<1 || len(sources)>=20 {
                    t.Fatalf("first immediate systematic count=%d err=%v",len(sources),err)
                }
                if deadline:=sender.NextFlushDeadline();!deadline.Equal(t0.Add(flushWindow)) {
                    t.Fatalf("first parity deadline %v want %v",deadline,t0.Add(flushWindow))
                }
                early,err:=sender.FlushDue(t0.Add(flushWindow-time.Nanosecond))
                if err!=nil || len(early)!=0 {
                    t.Fatalf("parity emitted too early count=%d err=%v",len(early),err)
                }

                // The first systematic fragment is lost. All remaining
                // systematic fragments reach the decoder at 15ms, but a
                // whole first UDP datagram must not yet be delivered.
                recovered:=0
                for i,fragment:=range sources {
                    if i==0 {continue}
                    got,err:=receiver.Decode(fragment,t0.Add(oneWay))
                    if err!=nil || len(got)!=0 {
                        t.Fatalf("incomplete first packet unexpectedly delivered: fragment=%d packets=%d err=%v",i,len(got),err)
                    }
                }

                parityWire,err:=sender.FlushDue(t0.Add(flushWindow))
                if err!=nil || len(parityWire)==0 {
                    t.Fatalf("no parity at 32ms count=%d err=%v",len(parityWire),err)
                }
                if next:=sender.NextFlushDeadline();!next.IsZero() {
                    t.Fatalf("retired partial parity still scheduled: %v",next)
                }
                for _,fragment:=range parityWire {
                    got,err:=receiver.Decode(fragment,t0.Add(flushWindow+oneWay))
                    if err!=nil {t.Fatal(err)}
                    for _,packet:=range got {
                        if !bytes.Equal(packet,first) {
                            t.Fatalf("recovery produced unrelated packet: len=%d",len(packet))
                        }
                        recovered++
                    }
                }
                if recovered!=1 {
                    t.Fatalf("15ms one-way first source recovery count=%d want1 at47ms",recovered)
                }

                // New data becomes ready at 45ms and its intact source
                // reaches the receiver at 60ms. It is delivered without
                // waiting for its own 32ms parity at 77ms (arrival 92ms),
                // and never waits behind the first datagram.
                secondWire,err:=sender.Encode(later,t0.Add(secondOffset))
                if err!=nil || len(secondWire)!=1 {
                    t.Fatalf("new independent packet not immediate count=%d err=%v",len(secondWire),err)
                }
                got,err:=receiver.Decode(secondWire[0],t0.Add(secondOffset+oneWay))
                if err!=nil || len(got)!=1 || !bytes.Equal(got[0],later) {
                    t.Fatalf("new independent small UDP blocked until parity packets=%d err=%v",len(got),err)
                }
                if want:=t0.Add(secondOffset+flushWindow);!sender.NextFlushDeadline().Equal(want) {
                    t.Fatalf("second parity deadline should be based on its own first source")
                }
                extra,err:=sender.FlushDue(t0.Add(secondOffset+flushWindow))
                if err!=nil {t.Fatal(err)}
                for _,fragment:=range extra {
                    delivered,err:=receiver.Decode(fragment,t0.Add(secondOffset+flushWindow+oneWay))
                    if err!=nil || len(delivered)!=0 {
                        t.Fatalf("parity re-delivered already complete packet count=%d err=%v",len(delivered),err)
                    }
                }
                if !sender.NextFlushDeadline().IsZero() {
                    t.Fatal("sparse parity timer not retired; idle scanning would persist")
                }
                t.Logf("E1_LOW_RTT_SPARSE_LINK_ONLY fec20:%d bytes=%d one_way_ms=15 first_repair_ms=47 second_fresh_arrival_ms=60 extra_parity_wait_ms_up_to24",parity,size)
            })
        }
    }
}
