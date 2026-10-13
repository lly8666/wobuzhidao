//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "syscall"
 "unsafe"
)

var (
 n4WintunGetAdapterLuid=wintunDLL.NewProc("WintunGetAdapterLuid")
 n4ConvertInterfaceLuidIndex=n4Iphlpapi.NewProc("ConvertInterfaceLuidToIndex")
)

// WintunNativeIdentity pins an interface index to the actual live adapter
// *handle* and its NET_LUID. Opening another program's adapter with the same
// display name does not grant ownership of its addresses/DNS/routes.
type WintunNativeIdentity struct {
 Index uint32
 LUID uint64
 CreatedByProcess bool
}

// NativeIdentity is read-only; beginCall prevents Close from freeing the
// adapter while WintunGetAdapterLuid is in flight. It never grants ownership
// based on alias matching alone.
func(t *TUN) NativeIdentity(ctx context.Context)(WintunNativeIdentity,error){
 if t==nil{return WintunNativeIdentity{},ErrInvalidBinding}
 if err:=ctx.Err();err!=nil{return WintunNativeIdentity{},err}
 if !t.beginCall(){return WintunNativeIdentity{},ErrInvalidBinding}
 defer t.endCall()
 if t.adapter==0 || t.session==0{return WintunNativeIdentity{},ErrInvalidBinding}
 if err:=n4WintunGetAdapterLuid.Find();err!=nil{return WintunNativeIdentity{},err}
 if err:=n4ConvertInterfaceLuidIndex.Find();err!=nil{return WintunNativeIdentity{},err}
 var luid uint64
 n4WintunGetAdapterLuid.Call(t.adapter,uintptr(unsafe.Pointer(&luid)))
 if luid==0{return WintunNativeIdentity{},fmt.Errorf("%w: Wintun returned zero NET_LUID",ErrInvalidBinding)}
 var index uint32
 rc,_,_:=n4ConvertInterfaceLuidIndex.Call(uintptr(unsafe.Pointer(&luid)),uintptr(unsafe.Pointer(&index)))
 if rc!=0{return WintunNativeIdentity{},fmt.Errorf("ConvertInterfaceLuidToIndex: %w",syscall.Errno(rc))}
 if index==0{return WintunNativeIdentity{},ErrInvalidBinding}
 return WintunNativeIdentity{Index:index,LUID:luid,CreatedByProcess:t.createdByProcess},nil
}

// IsCurrentOwnedWintun satisfies n4OwnedInterfaceProof for the native MTU
// writer. It fails closed for an adapter that existed before this process,
// for a closed Wintun session, and for a replaced/stale interface index.
// The future product transaction must also persist the installation +
// generation identity before calling native Apply.
func(t *TUN) IsCurrentOwnedWintun(ctx context.Context,index uint32)(bool,error){
 if t==nil || index==0{return false,ErrInvalidBinding}
 if err:=ctx.Err();err!=nil{return false,err}
 // This check avoids loading a real wintun.dll when the handle was only
 // borrowed; no adapter is touched on refusal.
 if !t.createdByProcess{return false,nil}
 id,err:=t.NativeIdentity(ctx)
 if err!=nil{return false,err}
 return id.CreatedByProcess && id.Index==index && id.LUID!=0,nil
}
