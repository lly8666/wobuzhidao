package windowsclient

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/netip"
 "os"
 "path/filepath"
 "slices"
 "sync"
)

var (
 ErrN4NativeGuard = errors.New("windowsclient: crash-persistent native DNS and IPv6 guard required")
 ErrN4JournalConflict = errors.New("windowsclient: existing native route journal; recover first")
 ErrN4RouteAmbiguous = errors.New("windowsclient: unconfirmed route ownership; retain journal and fail closed")
 ErrN4ForeignRoute = errors.New("windowsclient: route is not owned; refuse to replace")
)
const n4JournalSchema = "wbd-n4-native-owned-routes/v1"
const n4MaxNativeCaptureRoutes = 8
// A bounded owned-state journal, never a route database or packet log.
const n4MaxJournalBytes = 64 << 10

type NativeRouteKey struct {
 Prefix netip.Prefix `json:"prefix"`
 Interface uint32 `json:"interface"`
 NextHop netip.Addr `json:"next_hop"`
 Metric uint32 `json:"metric"`
}
type NativeRouteBackend interface {
 Lookup(context.Context, NativeRouteKey) (exists bool, exactMatch bool, err error)
 CreateIfAbsent(context.Context, NativeRouteKey) error
 RemoveOwned(context.Context, NativeRouteKey) error
}
// Durable MUST be true only for a firewall/DNS guard persisted independently
// of this process. A dynamic-session WFP guard is never adequate: it would
// disappear on crash while Windows keeps the capture routes.
type CrashPersistentGuard interface {
 Durable() bool
 Arm(context.Context) error
 Verify(context.Context) (bool,error)
 Disarm(context.Context) error
}
// An optional, strictly narrower recovery capability: the route journal
// MUST prove that all routes were removed before this is invoked. It lets a
// native WFP backend finish an interrupted partial deletion of only its own
// exact-matching filters. Never use it while any route intent remains.
type n4PostRouteGuardFinalizer interface {
 DisarmAfterRoutesRemoved(context.Context) error
}
func n4FinishGuardAfterRoutes(ctx context.Context,g CrashPersistentGuard)error{
 if f,ok:=g.(n4PostRouteGuardFinalizer);ok{
  return f.DisarmAfterRoutesRemoved(ctx)
 }
 return g.Disarm(ctx)
}
type n4Entry struct {
 Key NativeRouteKey `json:"key"`
 State string `json:"state"` // intent, owned
}
type n4Journal struct {
 Schema string `json:"schema"`
 Phase string `json:"phase"`
 GuardArmed bool `json:"guard_armed"`
 Entries []n4Entry `json:"entries"`
}
type n4Envelope struct {
 Data n4Journal `json:"data"`
 SHA256 string `json:"sha256"`
}
type N4OwnedRoutes struct {
 mu sync.Mutex
 Path string
 Route NativeRouteBackend
 Guard CrashPersistentGuard
}

func validateN4CompactOwned(plan NetworkPlan) ([]NativeRouteKey,error) {
 if plan.StatePath=="" || plan.AdapterAlias=="" || plan.Physical.InterfaceIndex==0 ||
 !plan.Physical.NextHop4.Is4() || plan.Physical.NextHop4==netip.IPv4Unspecified() ||
 !plan.Server4.Is4() || !plan.Lease4.IsValid() || !plan.Lease4.Addr().Is4() ||
 plan.Lease4.Bits()!=32 || len(plan.DirectRoutes)!=0 ||
 len(plan.CaptureRoutes)<2 || len(plan.CaptureRoutes)>n4MaxNativeCaptureRoutes {
  return nil,ErrNetworkPlan
 }
 full:=map[string]bool{"0.0.0.0/1":false,"128.0.0.0/1":false}
 used:=map[string]bool{}
 keys:=[]NativeRouteKey{}
 if plan.UnderlayRoute.Kind!="underlay" ||
  plan.UnderlayRoute.InterfaceIndex!=plan.Physical.InterfaceIndex ||
  plan.UnderlayRoute.Prefix!=netip.PrefixFrom(plan.Server4,32) ||
  plan.UnderlayRoute.NextHop!=plan.Physical.NextHop4 {return nil,ErrNetworkPlan}
 keys=append(keys,NativeRouteKey{Prefix:plan.UnderlayRoute.Prefix,Interface:plan.Physical.InterfaceIndex,NextHop:plan.Physical.NextHop4,Metric:5})
 for _,r:=range plan.CaptureRoutes{
  if !r.Prefix.IsValid() || !r.Prefix.Addr().Is4() || r.Prefix!=r.Prefix.Masked() ||
   r.Kind!="capture" || r.InterfaceIndex!=0 || r.AdapterAlias!=plan.AdapterAlias {return nil,ErrNetworkPlan}
  s:=r.Prefix.String()
  if used[s]{return nil,ErrNetworkPlan};used[s]=true
  if _,ok:=full[s];ok{full[s]=true
  }else if r.Prefix.Bits()!=32 || !slices.Contains(plan.DNSServers4,r.Prefix.Addr()) {
   return nil,ErrNetworkPlan
  }
 }
 for _,ok:=range full{if !ok{return nil,ErrNetworkPlan}}
 return keys,nil
}

 // N4BuildCompactNativeRouteKeys creates a bounded, deterministic native route
 // batch. The physical server /32 escape MUST precede all Wintun captures.
 // The caller MUST independently establish current created-Wintun ownership,
 // crash-persistent WFP/DNS protection and a durable lease/MTU/DNS journal.
 // This helper does not grant ownership and is not connected to the CLI.
func N4BuildCompactNativeRouteKeys(plan NetworkPlan, wintunIndex uint32) ([]NativeRouteKey,error) {
 underlay,err:=validateN4CompactOwned(plan)
 if err!=nil{return nil,err}
 if wintunIndex==0 || wintunIndex==plan.Physical.InterfaceIndex{return nil,ErrNetworkPlan}
 keys:=append([]NativeRouteKey(nil),underlay...)
 for _,r:=range plan.CaptureRoutes {
  keys=append(keys,NativeRouteKey{
   Prefix:r.Prefix,Interface:wintunIndex,NextHop:netip.IPv4Unspecified(),Metric:5,
  })
 }
 if err:=n4ValidateRouteBatch(keys);err!=nil{return nil,err}
 return keys,nil
}

// n4ValidateRouteBatch is the final no-host-CIDR preflight before any OS
// write. Position zero is the server escape; all remaining entries are
// exactly the /1 pair and at most six /32 captures (DNS exceptions). Caller
// must first use the plan validator to prove those /32s are configured DNS.
func n4ValidateRouteBatch(keys []NativeRouteKey) error {
 if len(keys)<3 || len(keys)>n4MaxNativeCaptureRoutes+1{return ErrNetworkPlan}
 zero:=netip.IPv4Unspecified()
 low:=netip.MustParsePrefix("0.0.0.0/1")
 high:=netip.MustParsePrefix("128.0.0.0/1")
 seen:=make(map[netip.Prefix]bool,len(keys))
 var captureIf uint32
 lowSeen,highSeen:=false,false
 for i,k:=range keys {
  if k.Interface==0 || k.Metric!=5 || !k.Prefix.IsValid() ||
   !k.Prefix.Addr().Is4() || k.Prefix!=k.Prefix.Masked() ||
   !k.NextHop.Is4() || seen[k.Prefix] {return ErrNetworkPlan}
  seen[k.Prefix]=true
  if i==0 {
   if k.Prefix.Bits()!=32 || k.NextHop==zero {return ErrNetworkPlan}
   continue
  }
  if k.NextHop!=zero || k.Interface==keys[0].Interface{return ErrNetworkPlan}
  if captureIf==0{captureIf=k.Interface}
  if captureIf!=k.Interface{return ErrNetworkPlan}
  switch k.Prefix {
  case low:lowSeen=true
  case high:highSeen=true
  default:
   if k.Prefix.Bits()!=32{return ErrNetworkPlan}
  }
 }
 if !lowSeen || !highSeen || captureIf==0{return ErrNetworkPlan}
 return nil
}

func n4Encode(j n4Journal)([]byte,error){
 j.Schema=n4JournalSchema
 data,err:=json.Marshal(j);if err!=nil{return nil,err}
 h:=sha256.Sum256(data)
 return json.MarshalIndent(n4Envelope{Data:j,SHA256:hex.EncodeToString(h[:])},"","  ")
}
func n4Decode(raw []byte)(n4Journal,error){
 var e n4Envelope
 if err:=json.Unmarshal(raw,&e);err!=nil{return n4Journal{},err}
 payload,err:=json.Marshal(e.Data);if err!=nil{return n4Journal{},err}
 h:=sha256.Sum256(payload)
 if e.Data.Schema!=n4JournalSchema || e.SHA256!=hex.EncodeToString(h[:]) ||
  (e.Data.Phase!="applying"&&e.Data.Phase!="active"&&e.Data.Phase!="cleaning") {
  return n4Journal{},ErrN4JournalConflict
 }
 if err:=n4ValidateJournal(e.Data);err!=nil{return n4Journal{},err}
 return e.Data,nil
}
// Check the decoded state even if its hash matches: the checksum is not an
// authorization to delete unexpected routes or accept impossible phases.
func n4ValidateJournal(j n4Journal)error{
 if len(j.Entries)>n4MaxNativeCaptureRoutes+1 ||
  (!j.GuardArmed && (j.Phase!="applying" || len(j.Entries)!=0)) ||
  (j.Phase=="active" && !j.GuardArmed){return ErrN4JournalConflict}
 seen:=make(map[netip.Prefix]bool,len(j.Entries))
 keys:=make([]NativeRouteKey,0,len(j.Entries))
 var captureIf uint32
 for i,v:=range j.Entries{
  k:=v.Key
  if (v.State!="intent" && v.State!="owned") || k.Interface==0 ||
   k.Metric!=5 || !k.Prefix.IsValid() || !k.Prefix.Addr().Is4() ||
   k.Prefix!=k.Prefix.Masked() || !k.NextHop.Is4() ||
   seen[k.Prefix]{return ErrN4JournalConflict}
  seen[k.Prefix]=true
  if i==0{
   if k.Prefix.Bits()!=32 || k.NextHop==netip.IPv4Unspecified(){return ErrN4JournalConflict}
  }else{
   if k.NextHop!=netip.IPv4Unspecified() || k.Interface==j.Entries[0].Key.Interface ||
    (k.Prefix!=netip.MustParsePrefix("0.0.0.0/1") &&
     k.Prefix!=netip.MustParsePrefix("128.0.0.0/1") &&
     k.Prefix.Bits()!=32){return ErrN4JournalConflict}
   if captureIf==0{captureIf=k.Interface}
   if captureIf!=k.Interface{return ErrN4JournalConflict}
  }
  keys=append(keys,k)
 }
 if j.Phase=="active" && n4ValidateRouteBatch(keys)!=nil{return ErrN4JournalConflict}
 return nil
}

func n4WriteJournal(path string,j n4Journal)error{
 b,err:=n4Encode(j);if err!=nil{return err}
 dir:=filepath.Dir(path)
 if err:=os.MkdirAll(dir,0700);err!=nil{return err}
 f,err:=os.CreateTemp(dir,".wbd-n4-journal-*");if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(append(b,'\n'));err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}
func n4ReadJournal(path string)(n4Journal,error){
 f,err:=os.Open(path);if err!=nil{return n4Journal{},err}
 defer f.Close()
 b,err:=io.ReadAll(io.LimitReader(f,n4MaxJournalBytes+1))
 if err!=nil{return n4Journal{},err}
 if len(b)>n4MaxJournalBytes{return n4Journal{},ErrN4JournalConflict}
 return n4Decode(b)
}
func n4NewJournal(path string,j n4Journal)error{
 dir:=filepath.Dir(path)
 if err:=os.MkdirAll(dir,0700);err!=nil{return err}
 f,err:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if errors.Is(err,os.ErrExist){return ErrN4JournalConflict}
 if err!=nil{return err}
 b,err:=n4Encode(j)
 if err==nil {_,err=f.Write(append(b,'\n'))}
 if err==nil {err=f.Sync()}
 closeErr:=f.Close()
 return errors.Join(err,closeErr)
}
func(n *N4OwnedRoutes) preflight()error{
 if n==nil || n.Route==nil || n.Guard==nil || n.Path=="" || !n.Guard.Durable() {return ErrN4NativeGuard}
 return nil
}
func(n *N4OwnedRoutes) ApplyRoutes(ctx context.Context,keys []NativeRouteKey)error{
 n.mu.Lock();defer n.mu.Unlock()
 if err:=n.preflight();err!=nil{return err}
 if err:=n4ValidateRouteBatch(keys);err!=nil{return err}
 if err:=ctx.Err();err!=nil{return err}
 j:=n4Journal{Phase:"applying",GuardArmed:false}
 // Journal MUST exist before touching a persistent external guard, otherwise
 // a crash during Arm could orphan firewall state with no recovery pointer.
 if err:=n4NewJournal(n.Path,j);err!=nil{return err}
 if err:=n.Guard.Arm(ctx);err!=nil{return fmt.Errorf("%w: %v",ErrN4NativeGuard,err)}
 j.GuardArmed=true
 if err:=n4WriteJournal(n.Path,j);err!=nil{return err}
 ok,err:=n.Guard.Verify(ctx)
 if err!=nil || !ok{return errors.Join(ErrN4NativeGuard,err)}
 for _,key:=range keys {
  if err:=ctx.Err();err!=nil{return err}
  exists,_,err:=n.Route.Lookup(ctx,key)
  if err!=nil{return err}
  if exists{return ErrN4ForeignRoute}
  j.Entries=append(j.Entries,n4Entry{Key:key,State:"intent"})
  if err:=n4WriteJournal(n.Path,j);err!=nil{return err}
  if err:=n.Route.CreateIfAbsent(ctx,key);err!=nil{return err}
  j.Entries[len(j.Entries)-1].State="owned"
  if err:=n4WriteJournal(n.Path,j);err!=nil{return err}
 }
 j.Phase="active"
 return n4WriteJournal(n.Path,j)
}
func(n *N4OwnedRoutes) Recover(ctx context.Context)error{
 n.mu.Lock();defer n.mu.Unlock()
 if err:=n.preflight();err!=nil{return err}
 j,err:=n4ReadJournal(n.Path)
 if errors.Is(err,os.ErrNotExist){return nil}
 if err!=nil{return fmt.Errorf("%w: %v",ErrN4JournalConflict,err)}
 if err:=ctx.Err();err!=nil{return err}
 ok,err:=n.Guard.Verify(ctx)
 if err!=nil{return errors.Join(ErrN4NativeGuard,err)}
 if !j.GuardArmed {
  // A journal is written before Arm; routes are never added until an armed
  // marker is saved. With no route entries, only a freshly verified, FULL
  // exact-owned persistent guard can be safely disarmed after this crash.
  if !ok{return ErrN4NativeGuard}
  if err:=n.Guard.Disarm(ctx);err!=nil{return err}
  return os.Remove(n.Path)
 }
 if !ok {
  // Power loss may follow Disarm but precede journal unlink. This is safe
  // to complete ONLY when every route was already journaled as removed.
  // The guard backend's idempotent Disarm must reject partial/foreign state.
  if j.Phase!="cleaning" || len(j.Entries)!=0{return ErrN4NativeGuard}
  if err:=n4FinishGuardAfterRoutes(ctx,n.Guard);err!=nil{return err}
  return os.Remove(n.Path)
 }
 j.Phase="cleaning"
 if err:=n4WriteJournal(n.Path,j);err!=nil{return err}
 for i:=len(j.Entries)-1;i>=0;i-- {
  entry:=j.Entries[i]
  exists,match,err:=n.Route.Lookup(ctx,entry.Key);if err!=nil{return err}
  if entry.State=="intent" && exists{return ErrN4RouteAmbiguous}
  if exists && !match{return ErrN4ForeignRoute}
  if entry.State=="owned"&&exists {
   if err:=n.Route.RemoveOwned(ctx,entry.Key);err!=nil{return err}
  }
  j.Entries=append(j.Entries[:i],j.Entries[i+1:]...)
  if err:=n4WriteJournal(n.Path,j);err!=nil{return err}
 }
 if err:=n4FinishGuardAfterRoutes(ctx,n.Guard);err!=nil{return err}
 return os.Remove(n.Path)
}
