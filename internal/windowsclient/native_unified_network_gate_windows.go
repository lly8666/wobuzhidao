//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "fmt"
 "runtime"
 "sync"

 "golang.org/x/sys/windows"
)

// N4WindowsMachineGate is a real Win32, cross-process exclusion primitive.
// The namespace is machine-wide rather than installation- or session-local:
// a second WBD instance must not race network WFP/route/DNS mutations.
// It DOES NOT prove that no foreign Wintun is present, grant privilege, or
// replace the installed-Wintun owner and generation check.
// The native unified transaction remains disconnected from the product.
type N4WindowsMachineGate struct{}

// This fixed name must not include untrusted adapter names or user input.
// A colliding, inaccessible, wrong-type Windows object fails closed.
const n4MachineMutexName = `Global\WBD-N4-NativeNetworkTxn-Exclusive-v1`
const n4MachineMutexWaitMillis uint32 = 50
const n4WaitTimeout uint32 = 0x00000102

// Acquire's returned release closure MUST run on the acquiring goroutine:
// a Windows mutex is owned by an OS thread, not by the Go process.
// Pin the Go goroutine to that thread through the entire transaction.
func (N4WindowsMachineGate) Acquire(ctx context.Context)(func(),error){
 if err:=ctx.Err();err!=nil{return nil,err}
 runtime.LockOSThread()
 fail:=func(h windows.Handle,e error)(func(),error){
  if h!=0{_ = windows.CloseHandle(h)}
  runtime.UnlockOSThread()
  return nil,e
 }
 name,err:=windows.UTF16PtrFromString(n4MachineMutexName)
 if err!=nil{return fail(0,err)}
 h,err:=windows.CreateMutex(nil,false,name)
 if h==0{return fail(h,fmt.Errorf("%w: CreateMutexW: %v",ErrN4NativeGuard,err))}
 if err!=nil && !errors.Is(err,windows.ERROR_ALREADY_EXISTS){
  return fail(h,fmt.Errorf("%w: CreateMutexW: %v",ErrN4NativeGuard,err))
 }
 for {
  if err:=ctx.Err();err!=nil{return fail(h,err)}
  result,waitErr:=windows.WaitForSingleObject(h,n4MachineMutexWaitMillis)
  switch result{
  case windows.WAIT_OBJECT_0,windows.WAIT_ABANDONED:
   // An abandoned mutex is now owned by this thread. The caller MUST
   // reconcile/validate the write-ahead journal before any OS write:
   // Apply checks durable O_EXCL state, Recover checks checksum+generation.
   var once sync.Once
   return func(){
    once.Do(func(){
     err:=windows.ReleaseMutex(h)
     closeErr:=windows.CloseHandle(h)
     runtime.UnlockOSThread()
     if err!=nil || closeErr!=nil{
      panic(fmt.Sprintf("N4 machine mutex release unsafe: %v; close: %v",err,closeErr))
     }
    })
   },nil
  case n4WaitTimeout:
   continue
  default:
   return fail(h,fmt.Errorf("%w: WaitForSingleObject status=%d: %v",ErrN4NativeGuard,result,waitErr))
  }
 }
}
