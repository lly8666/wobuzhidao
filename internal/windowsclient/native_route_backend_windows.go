//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "syscall"
 "unsafe"
)

// N4WindowsRouteBackend bridges the fail-closed owned journal to Win32
// IP Helper. No syscall executes until a caller explicitly invokes a method.
// An absent native crash-persistent WFP/DNS guard prevents product activation.
type N4WindowsRouteBackend struct{}

func n4NativeRouteMatches(row n4ForwardRow,key NativeRouteKey)bool{
 return row.Index==key.Interface &&
  int(row.Destination.Bits)==key.Prefix.Bits() &&
  n4ReadSockaddrIPv4(&row.Destination.Address)==key.Prefix.Addr() &&
  n4ReadSockaddrIPv4(&row.NextHop)==key.NextHop &&
  row.Metric==key.Metric &&
  row.Loopback==0 && row.Publish==0
}

func (N4WindowsRouteBackend) Lookup(ctx context.Context,key NativeRouteKey)(bool,bool,error){
 if err:=ctx.Err();err!=nil{return false,false,err}
 row,err:=N4NativeRouteEntry(key.Prefix,key.Interface,key.NextHop)
 if err!=nil{return false,false,err}
 if err:=n4RouteGet.Find();err!=nil{return false,false,err}
 rc,_,_:=n4RouteGet.Call(uintptr(unsafe.Pointer(&row)))
 switch syscall.Errno(rc){
 case 0:return true,n4NativeRouteMatches(row,key),nil
 case n4ErrorNotFound:return false,false,nil
 default:return false,false,fmt.Errorf("GetIpForwardEntry2: %w",syscall.Errno(rc))
 }
}
func (b N4WindowsRouteBackend) CreateIfAbsent(ctx context.Context,key NativeRouteKey)error{
 if err:=ctx.Err();err!=nil{return err}
 present,_,err:=b.Lookup(ctx,key)
 if err!=nil{return err}
 if present{return ErrN4ForeignRoute}
 row,err:=N4NativeRouteEntry(key.Prefix,key.Interface,key.NextHop)
 if err!=nil{return err}
 row.Metric=key.Metric
 if err:=n4RouteCreate.Find();err!=nil{return err}
 rc,_,_:=n4RouteCreate.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return fmt.Errorf("CreateIpForwardEntry2: %w",syscall.Errno(rc))}
 return nil
}
func(b N4WindowsRouteBackend) RemoveOwned(ctx context.Context,key NativeRouteKey)error{
 if err:=ctx.Err();err!=nil{return err}
 present,match,err:=b.Lookup(ctx,key)
 if err!=nil{return err}
 if !present{return nil}
 if !match{return ErrN4ForeignRoute}
 row,err:=N4NativeRouteEntry(key.Prefix,key.Interface,key.NextHop)
 if err!=nil{return err}
 if err:=n4RouteDelete.Find();err!=nil{return err}
 rc,_,_:=n4RouteDelete.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return fmt.Errorf("DeleteIpForwardEntry2: %w",syscall.Errno(rc))}
 return nil
}
