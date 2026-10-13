//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
 "time"
)

func TestN4WindowsMachineGateCancelledBeforeWindowsMutation(t *testing.T){
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 release,err:=(N4WindowsMachineGate{}).Acquire(ctx)
 if !errors.Is(err,context.Canceled) || release!=nil {
  t.Fatalf("cancelled gate acquired OS mutex: release=%v err=%v",release!=nil,err)
 }
}

func TestN4WindowsMachineGateSerializesAndReleases(t *testing.T){
 gate:=N4WindowsMachineGate{}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
 defer cancel()
 release,err:=gate.Acquire(ctx)
 if err!=nil{t.Fatalf("native machine mutex unavailable: %v",err)}
 // A concurrent goroutine is on a DIFFERENT locked OS thread and must not
 // be able to reenter the process-wide named mutex.
 blocked:=make(chan error,1)
 go func(){
  waitingCtx,cancel:=context.WithTimeout(context.Background(),220*time.Millisecond)
  defer cancel()
  next,err:=gate.Acquire(waitingCtx)
  if next!=nil{next()}
  blocked<-err
 }()
 if err:=<-blocked;!errors.Is(err,context.DeadlineExceeded){
  release()
  t.Fatalf("separate thread entered held Windows mutex: %v",err)
 }
 release()
 release() // idempotent closure; must not double-release/unlock OS thread
 ctx2,cancel2:=context.WithTimeout(context.Background(),3*time.Second)
 defer cancel2()
 next,err:=gate.Acquire(ctx2)
 if err!=nil{t.Fatalf("native gate not reusable after release: %v",err)}
 next()
}
