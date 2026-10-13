//go:build windows

package windowsclient

import (
    "bytes"
    "errors"
    "io"
    "testing"
    "time"
)

type n4TCPDummyWriter struct{ writes int }
func (w *n4TCPDummyWriter) WritePacket(p []byte)(int,error){w.writes++;return len(p),nil}

func TestN4TCPBoundedQueueOwnershipAndClose(t *testing.T) {
    writer:=&n4TCPDummyWriter{}
    r:=&n4TCPPacketRW{out:writer, packets:make(chan []byte,1),done:make(chan struct{})}
    p:=[]byte{1,2,3}
    if err:=r.Inject(p);err!=nil{t.Fatal(err)}
    p[0]=9
    if err:=r.Inject(p);!errors.Is(err,ErrN4TCPQueueFull){t.Fatalf("full queue should fail closed: %v",err)}
    buf:=make([]byte,16)
    n,err:=r.Read(buf)
    if err!=nil||!bytes.Equal(buf[:n],[]byte{1,2,3}){t.Fatalf("must own packet n=%d err=%v bytes=%v",n,err,buf[:n])}
    if _,err=r.Write([]byte{7});err!=nil{t.Fatal(err)}
    if writer.writes!=1{t.Fatal("reply packet writer not used")}
    r.Close();r.Close()
    if err:=r.Inject([]byte{1});!errors.Is(err,io.EOF){t.Fatalf("closed queue accepted input: %v",err)}
    if _,err:=r.Write([]byte{7});!errors.Is(err,io.EOF){t.Fatalf("closed queue accepted reply: %v",err)}
}

func TestN4TCPRejectsGenerationAndUnrelatedProtocols(t *testing.T) {
    r:=&n4TCPPacketRW{out:&n4TCPDummyWriter{},packets:make(chan []byte,1),done:make(chan struct{})}
    d:=&UserspaceTCPDirect{iface:17,generation:4,rw:r}
    p:=make([]byte,40)
    p[0]=0x45;p[9]=6;p[2]=0;p[3]=40
    if err:=d.ForwardDirectIPv4(p,17,3);!errors.Is(err,ErrTUNFlowScope){t.Fatalf("generation: %v",err)}
    if err:=d.ForwardDirectIPv4(p,19,4);!errors.Is(err,ErrTUNFlowScope){t.Fatalf("iface: %v",err)}
    p[9]=17
    if err:=d.ForwardDirectIPv4(p,17,4);!errors.Is(err,ErrN4TCPPacket){t.Fatalf("UDP: %v",err)}
    p[9]=6
    if err:=d.ForwardDirectIPv4(p,17,4);err!=nil{t.Fatalf("valid TCP: %v",err)}
    if n:=len(r.packets);n!=1{t.Fatalf("enqueued=%d",n)}
    r.Close()
    _=time.Second
}
