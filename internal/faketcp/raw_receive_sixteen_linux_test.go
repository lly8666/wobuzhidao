//go:build linux

package faketcp

import (
	"bytes"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// E4 guarded receive-only change: keep the 8-record synchronous sender
// lock window, and never wait for a full 16-record receive batch.
func TestRawReceiveSixteenPrequeuedPacketsOwnedAndOrdered(t *testing.T) {
	if rawBatchSize != 8 || rawReceiveBatchSize != 16 {
		t.Fatalf("unexpected raw batch budgets: send=%d receive=%d", rawBatchSize,rawReceiveBatchSize)
	}
	e, sender := rawBatchPair(t, 65536+64)
	const count=16
	for i:=0; i<count; i++ {
		if err:=unix.Sendto(sender,rawBatchPacket([]byte{byte(i+1)},uint16(i+1)),0,nil);err!=nil {
			t.Fatalf("send %d: %v",i,err)
		}
	}
	var held [][]byte
	for i:=0; i<count; i++ {
		seg,wire,err:=e.ReadSegment()
		if err!=nil || !bytes.Equal(seg.Payload, []byte{byte(i+1)}) {
			t.Fatalf("read %d payload=%x err=%v",i,seg.Payload,err)
		}
		held=append(held,wire)
	}
	s:=e.IODiagnostic()
	if s.ReceiveCalls!=1 || s.ReceiveMessages!=count || s.ReceiveMulti!=1 {
		t.Fatalf("expected one no-wait recvmmsg for 16 already queued frames: %+v",s)
	}
	if err:=unix.Sendto(sender,rawBatchPacket([]byte{0xfd},90),0,nil);err!=nil { t.Fatal(err) }
	if _,_,err:=e.ReadSegment();err!=nil {t.Fatal(err)}
	for i,wire:=range held {
		seg,err:=ParseIPv4TCP(wire)
		if err!=nil || !bytes.Equal(seg.Payload,[]byte{byte(i+1)}) {
			t.Fatalf("receive ring reuse changed owned packet %d: err=%v",i,err)
		}
	}
}

func TestRawReceiveSixteenSparseOneDoesNotWaitToFill(t *testing.T) {
	e,sender:=rawBatchPair(t,65536+64)
	if err:=unix.Sendto(sender,rawBatchPacket([]byte("first"),1),0,nil);err!=nil {t.Fatal(err)}
	done:=make(chan error,1)
	go func(){
		seg,_,err:=e.ReadSegment()
		if err==nil && !bytes.Equal(seg.Payload,[]byte("first")) {err=ErrBadTCPHeader}
		done<-err
	}()
	select {
	case err:=<-done:
		if err!=nil {t.Fatal(err)}
	case <-time.After(time.Second):
		t.Fatal("recvmmsg waited for a 16-packet batch and delayed first arrival")
	}
}
