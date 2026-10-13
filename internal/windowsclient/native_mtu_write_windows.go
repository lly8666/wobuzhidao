//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

var n4SetIPInterface=n4Iphlpapi.NewProc("SetIpInterfaceEntry")

// n4OwnedInterfaceProof must be implemented by a future live Wintun adapter
// handle owner, NOT from the adapter alias or a mutable route-table entry.
type n4OwnedInterfaceProof interface {
 IsCurrentOwnedWintun(context.Context,uint32)(bool,error)
}

// This primitive is unconnected to the client until the original row,
// exact owner generation and intended MTU change are in a durable journal.
type n4WindowsMTUWriter struct {
 Guard CrashPersistentGuard
 Adapter n4OwnedInterfaceProof
}
type n4MTUWriteReceipt struct {
 index uint32
 before N4InterfaceIPv4Snapshot
 after uint32
 confirmed bool
}

func n4MTUSnapshotFromRow(row windows.MibIpInterfaceRow) N4InterfaceIPv4Snapshot {
 return N4InterfaceIPv4Snapshot{
  Index:row.InterfaceIndex,MTU:row.NlMtu,Metric:row.Metric,
  Connected:row.Connected!=0,WeakHostSend:row.WeakHostSend!=0,
  WeakHostReceive:row.WeakHostReceive!=0,
  AutomaticMetric:row.UseAutomaticMetric!=0,
  DisableDefaultRoutes:row.DisableDefaultRoutes!=0,
 }
}
func n4SameInterfaceBaseline(a,b N4InterfaceIPv4Snapshot)bool{
 return a==b
}
func (w n4WindowsMTUWriter) preflight(ctx context.Context,old N4InterfaceIPv4Snapshot,target uint32)error {
 if err:=ctx.Err();err!=nil{return err}
 if err:=N4VerifyOwnedTunnelMTU(old,old.Index,target);err!=nil{return err}
 if w.Guard==nil || !w.Guard.Durable() || w.Adapter==nil{return ErrN4NativeGuard}
 owned,err:=w.Adapter.IsCurrentOwnedWintun(ctx,old.Index)
 if err!=nil{return err}
 if !owned{return ErrN4ForeignRoute}
 verified,err:=w.Guard.Verify(ctx)
 if err!=nil{return fmt.Errorf("%w: guard: %v",ErrN4NativeGuard,err)}
 if !verified{return ErrN4NativeGuard}
 return nil
}

func n4GetIPv4InterfaceRow(ctx context.Context,index uint32)(windows.MibIpInterfaceRow,error){
 var row windows.MibIpInterfaceRow
 if index==0{return row,ErrNetworkPlan}
 if err:=ctx.Err();err!=nil{return row,err}
 if err:=n4InitIPInterface.Find();err!=nil{return row,err}
 if err:=n4GetIPInterface.Find();err!=nil{return row,err}
 n4InitIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 row.Family=windows.AF_INET
 row.InterfaceIndex=index
 rc,_,_:=n4GetIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return row,fmt.Errorf("GetIpInterfaceEntry: %w",syscall.Errno(rc))}
 if row.Family!=windows.AF_INET || row.InterfaceIndex!=index{return row,ErrN4ForeignRoute}
 return row,nil
}

// The IPv4 Get API may return SitePrefixLength=64 on a newly created
// Wintun despite Family=AF_INET. Per the Win32 SetIpInterfaceEntry ABI,
// IPv4 SitePrefixLength MUST be 0 on INPUT, even if the read row differs.
// Only the original CAS-confirmed NlMtu is intentionally changed.
func n4PrepareIPv4MTUSetRow(row *windows.MibIpInterfaceRow,target uint32){
 row.SitePrefixLength=0
 row.NlMtu=target
}

// Apply changes ONLY the MTU in the entire OS-read interface row. Microsoft
// requires IPv4 SitePrefixLength=0 on SetIpInterfaceEntry. Any deviation
// from the snapshotted baseline is foreign mutation, and must halt.
func (w n4WindowsMTUWriter) Apply(ctx context.Context,old N4InterfaceIPv4Snapshot,target uint32)(n4MTUWriteReceipt,error){
 var receipt n4MTUWriteReceipt
 if err:=w.preflight(ctx,old,target);err!=nil{return receipt,err}
 if target==old.MTU {return receipt,ErrNetworkPlan}
 row,err:=n4GetIPv4InterfaceRow(ctx,old.Index)
 if err!=nil{return receipt,err}
 if !n4SameInterfaceBaseline(old,n4MTUSnapshotFromRow(row)) {return receipt,ErrN4ForeignRoute}
 // Wintun GetIpInterfaceEntry may return SitePrefixLength=64 even for
 // AF_INET. Microsoft's SetIpInterfaceEntry contract requires the INPUT
 // SitePrefixLength to be zero for IPv4. The complete tracked interface
 // state was already CAS-checked above; this is a required input-only
 // normalization, not a permission to overwrite foreign interface state.
 if err:=ctx.Err();err!=nil{return receipt,err}
 if err:=n4SetIPInterface.Find();err!=nil{return receipt,err}
 n4PrepareIPv4MTUSetRow(&row,target)
 rc,_,_:=n4SetIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return receipt,fmt.Errorf("SetIpInterfaceEntry: %w",syscall.Errno(rc))}
 // If a post-write read fails, ownership is ambiguous; the caller's
 // journal (required before calling) must retain intent for recovery.
 after,err:=n4GetIPv4InterfaceRow(ctx,old.Index)
 if err!=nil{return receipt,err}
 next:=old;next.MTU=target
 if !n4SameInterfaceBaseline(next,n4MTUSnapshotFromRow(after)){return receipt,ErrN4RouteAmbiguous}
 return n4MTUWriteReceipt{index:old.Index,before:old,after:target,confirmed:true},nil
}
func (w n4WindowsMTUWriter) Restore(ctx context.Context,receipt n4MTUWriteReceipt)error {
 if !receipt.confirmed || receipt.index==0 || receipt.index!=receipt.before.Index {return ErrN4ForeignRoute}
 cur:=receipt.before
 cur.MTU=receipt.after
 // Verify the owned CURRENT post-Apply row and configured bounded MTU.
 // The ORIGINAL Windows Wintun NlMtu can legitimately be 65535; it was
 // captured before our write and is CAS-protected below. Do not reuse the
 // 576..9000 NEW-config restriction on that original restoration value.
 if err:=w.preflight(ctx,cur,receipt.after);err!=nil{return err}
 row,err:=n4GetIPv4InterfaceRow(ctx,receipt.index)
 if err!=nil{return err}
 if !n4SameInterfaceBaseline(cur,n4MTUSnapshotFromRow(row)){return ErrN4ForeignRoute}
 if err:=ctx.Err();err!=nil{return err}
 if err:=n4SetIPInterface.Find();err!=nil{return err}
 n4PrepareIPv4MTUSetRow(&row,receipt.before.MTU)
 rc,_,_:=n4SetIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return fmt.Errorf("Restore SetIpInterfaceEntry: %w",syscall.Errno(rc))}
 return nil
}
