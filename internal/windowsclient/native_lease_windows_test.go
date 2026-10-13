//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "net/netip"
 "testing"
 "unsafe"

 "golang.org/x/sys/windows"
)
func n4LeaseTestIntent() N4NativeLeaseIntent{
 return N4NativeLeaseIntent{InterfaceIndex:72,Prefix:netip.MustParsePrefix("10.66.0.7/32")}
}
func TestN4NativeLeaseWin32RowABIAndBadInputs(t *testing.T){
 l:=n4LeaseTestIntent()
 row,err:=n4NativeLeaseRow(l);if err!=nil{t.Fatal(err)}
 if unsafe.Sizeof(row.Address)!=28 {t.Fatalf("SOCKADDR_INET ABI width=%d",unsafe.Sizeof(row.Address))}
 if row.Address.Family!=windows.AF_INET || row.InterfaceIndex!=72 || row.InterfaceLuid!=0 ||
  row.OnLinkPrefixLength!=32 || row.SkipAsSource!=0 || 
  n4ReadSockaddrIPv4((*n4SockaddrInet)(unsafe.Pointer(&row.Address)))!=l.Prefix.Addr(){
  t.Fatalf("native static lease ABI mismatch: %+v",row)
 }
 for _,bad:=range []N4NativeLeaseIntent{
  {InterfaceIndex:0,Prefix:l.Prefix},
  {InterfaceIndex:72,Prefix:netip.MustParsePrefix("10.66.0.7/24")},
  {InterfaceIndex:72,Prefix:netip.MustParsePrefix("0.0.0.0/32")},
  {InterfaceIndex:72,Prefix:netip.MustParsePrefix("127.0.0.1/32")},
  {InterfaceIndex:72,Prefix:netip.MustParsePrefix("169.254.8.1/32")},
  {InterfaceIndex:72,Prefix:netip.MustParsePrefix("::1/128")},
 }{
  if _,err:=n4NativeLeaseRow(bad);!errors.Is(err,ErrNetworkPlan){t.Fatalf("invalid lease %+v accepted %v",bad,err)}
 }
}
func TestN4NativeLeaseWin32DADAndForeignProtection(t *testing.T){
 l:=n4LeaseTestIntent()
 row,err:=n4NativeLeaseRow(l);if err!=nil{t.Fatal(err)}
 for _,dad:=range []uint32{0,1,2,3,5}{
  row.DadState=dad
  if err:=N4NativeLeaseDADGate(row,l);!errors.Is(err,ErrN4LeaseNotPreferred){t.Fatalf("DAD %d accepted: %v",dad,err)}
 }
 row.DadState=n4DADPreferred
 if err:=N4NativeLeaseDADGate(row,l);err!=nil{t.Fatal(err)}
 row.OnLinkPrefixLength=24
 if err:=N4NativeLeaseDADGate(row,l);!errors.Is(err,ErrN4ForeignRoute){t.Fatal("foreign subnet accepted")}
 row.OnLinkPrefixLength=32
 row.SkipAsSource=1
 if err:=N4NativeLeaseDADGate(row,l);!errors.Is(err,ErrN4ForeignRoute){t.Fatal("foreign skip-as-source accepted")}
}
func TestN4NativeLeaseWriterFailsClosedBeforeAnyMutation(t *testing.T){
 l:=n4LeaseTestIntent()
 w:=N4WindowsLeaseWriter{}
 if _,err:=w.Create(context.Background(),l);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("missing guard accepted: %v",err)}
 if err:=w.DeleteConfirmed(context.Background(),n4NativeLeaseOwnerToken{});!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("unowned token accepted: %v",err)
 }
 volatile:=&n4FakeGuard{durable:false,active:true}
 w.Guard=volatile
 if _,err:=w.Create(context.Background(),l);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("volatile guard accepted: %v",err)}
 durable:=&n4FakeGuard{durable:true,active:false}
 w.Guard=durable
 if _,err:=w.Create(context.Background(),l);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("unarmed guard accepted: %v",err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=w.Create(ctx,l);!errors.Is(err,context.Canceled){t.Fatalf("cancel not respected: %v",err)}
 if err:=w.DeleteConfirmed(ctx,n4NativeLeaseOwnerToken{intent:l,created:true});!errors.Is(err,context.Canceled){t.Fatalf("cancelled delete: %v",err)}
 // All calls fail BEFORE Win32 Create/Delete. No NIC mutated by Actions.
}
func TestN4NativeLeaseReadOnlyWin32MissingInterface(t *testing.T){
 l:=N4NativeLeaseIntent{InterfaceIndex:4294967295,Prefix:netip.MustParsePrefix("192.0.2.223/32")}
 _,found,err:=(N4WindowsLeaseReader{}).Lookup(context.Background(),l)
 if err!=nil{t.Logf("Windows rejects nonexistent interface with error (safe): %v",err);return}
 if found{t.Fatal("unexpected address exists on impossible interface")}
}

type n4LeaseFakeOwner struct{
 owned bool
 err error
 calls int
}
func(f *n4LeaseFakeOwner) IsCurrentOwnedWintun(_ context.Context,index uint32)(bool,error){
 f.calls++
 if index==0{return false,ErrNetworkPlan}
 if f.err!=nil{return false,f.err}
 return f.owned,nil
}
func TestN4NativeLeaseWriterRequiresLiveProcessCreatedWintun(t *testing.T){
 l:=n4LeaseTestIntent()
 active:=&n4FakeGuard{durable:true,active:true}
 w:=N4WindowsLeaseWriter{Guard:active}
 if _,err:=w.Create(context.Background(),l);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("DNS/WFP guard alone must not allow lease create: %v",err)
 }
 owner:=&n4LeaseFakeOwner{}
 w.Adapter=owner
 if _,err:=w.Create(context.Background(),l);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed adapter allowed lease create: %v",err)
 }
 token:=n4NativeLeaseOwnerToken{intent:l,created:true}
 if err:=w.DeleteConfirmed(context.Background(),token);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("in-memory token allowed foreign adapter delete: %v",err)
 }
 if owner.calls!=2{t.Fatalf("expected owner proof for create/delete, got %d",owner.calls)}
 stale:=errors.New("stale generation")
 owner.err=stale
 if _,err:=w.Create(context.Background(),l);!errors.Is(err,stale){t.Fatalf("owner error lost: %v",err)}
 if err:=w.DeleteConfirmed(context.Background(),token);!errors.Is(err,stale){t.Fatalf("owner error lost on delete: %v",err)}
 owner.err=nil
 owner.owned=true
 if err:=w.preflight(context.Background(),l);err!=nil{t.Fatalf("valid current owner + persistent guard rejected: %v",err)}
 active.active=false
 if err:=w.preflight(context.Background(),l);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("inactive WFP guard accepted: %v",err)}
 active.active=true
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 calls:=owner.calls
 if _,err:=w.Create(ctx,l);!errors.Is(err,context.Canceled){t.Fatalf("cancelled owner lease create: %v",err)}
 if err:=w.DeleteConfirmed(ctx,token);!errors.Is(err,context.Canceled){t.Fatalf("cancelled owner lease delete: %v",err)}
 if owner.calls!=calls{t.Fatal("canceled operation queried owner or touched Windows state")}
 // Positive checks stop at preflight; no real unicast Create/Delete executed.
}
