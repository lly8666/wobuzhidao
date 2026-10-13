//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

var (
 n4InitIPInterface=n4Iphlpapi.NewProc("InitializeIpInterfaceEntry")
 n4GetIPInterface=n4Iphlpapi.NewProc("GetIpInterfaceEntry")
)

// N4InterfaceIPv4Snapshot contains only copied, read-only native IP
// interface state for a specific verified Windows interface index.
// GetIPInterfaceEntry is supported without admin rights. The product must
// journal the FULL original native row before eventual SetIpInterfaceEntry,
// and must refuse to write a non-owned/changed WinTun interface.
type N4InterfaceIPv4Snapshot struct {
 Index uint32
 MTU uint32
 Metric uint32
 Connected bool
 WeakHostSend bool
 WeakHostReceive bool
 AutomaticMetric bool
 DisableDefaultRoutes bool
}
type N4WindowsInterfaceReader struct{}
func (N4WindowsInterfaceReader) ReadIPv4(ctx context.Context,index uint32)(N4InterfaceIPv4Snapshot,error){
 if index==0{return N4InterfaceIPv4Snapshot{},ErrNetworkPlan}
 if err:=ctx.Err();err!=nil{return N4InterfaceIPv4Snapshot{},err}
 if err:=n4InitIPInterface.Find();err!=nil{return N4InterfaceIPv4Snapshot{},err}
 if err:=n4GetIPInterface.Find();err!=nil{return N4InterfaceIPv4Snapshot{},err}
 var row windows.MibIpInterfaceRow
 n4InitIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 row.Family=windows.AF_INET
 row.InterfaceIndex=index
 rc,_,_:=n4GetIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return N4InterfaceIPv4Snapshot{},fmt.Errorf("GetIpInterfaceEntry: %w",syscall.Errno(rc))}
 if row.InterfaceIndex!=index || row.Family!=windows.AF_INET {
  return N4InterfaceIPv4Snapshot{},ErrNetworkPlan
 }
 return N4InterfaceIPv4Snapshot{
  Index:row.InterfaceIndex,MTU:row.NlMtu,Metric:row.Metric,
  Connected:row.Connected!=0,WeakHostSend:row.WeakHostSend!=0,
  WeakHostReceive:row.WeakHostReceive!=0,
  AutomaticMetric:row.UseAutomaticMetric!=0,
  DisableDefaultRoutes:row.DisableDefaultRoutes!=0,
 },nil
}
func N4VerifyOwnedTunnelMTU(s N4InterfaceIPv4Snapshot,index uint32,mtu uint32)error{
 if index==0 || s.Index!=index || s.MTU<576 || s.MTU>65535 ||
  mtu<576 || mtu>9000 || s.MTU==0{return ErrNetworkPlan}
 // Only a LIVE WBD-owned Wintun index is eligible for later MTU updates;
 // ownership is not inferred from this read-only snapshot.
 return nil
}
