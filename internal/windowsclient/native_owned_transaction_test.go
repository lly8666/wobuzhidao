package windowsclient

import (
 "context"
 "errors"
 "net/netip"
 "os"
 "path/filepath"
 "sync"
 "testing"
)

type n4FakeGuard struct {
 durable, active bool
 armErr, verifyErr, disarmErr error
 armCount, disarmCount int
}
func(g *n4FakeGuard) Durable()bool{return g.durable}
func(g *n4FakeGuard) Arm(context.Context)error{
 g.armCount++
 if g.armErr!=nil{return g.armErr}
 g.active=true;return nil
}
func(g *n4FakeGuard) Verify(context.Context)(bool,error){return g.active,g.verifyErr}
func(g *n4FakeGuard) Disarm(context.Context)error{
 g.disarmCount++
 if g.disarmErr!=nil{return g.disarmErr}
 g.active=false;return nil
}
type n4FakeRoute struct {
 mu sync.Mutex
 existing map[NativeRouteKey]bool
 created, removed int
 createErrAt, removeErrAt int
 cancelAt int
 cancel context.CancelFunc
}
func (f *n4FakeRoute) Lookup(_ context.Context,key NativeRouteKey)(bool,bool,error) {
 f.mu.Lock();defer f.mu.Unlock()
 match,found:=f.existing[key];return found,match,nil
}
func (f *n4FakeRoute) CreateIfAbsent(_ context.Context,key NativeRouteKey)error {
 f.mu.Lock();defer f.mu.Unlock()
 f.created++
 if f.createErrAt==f.created{return errors.New("injected CreateIfAbsent failure")}
 if _,exists:=f.existing[key];exists{return ErrN4ForeignRoute}
 f.existing[key]=true
 if f.cancelAt==f.created && f.cancel!=nil{f.cancel()}
 return nil
}
func (f *n4FakeRoute) RemoveOwned(_ context.Context,key NativeRouteKey)error{
 f.mu.Lock();defer f.mu.Unlock()
 f.removed++
 if f.removeErrAt==f.removed{return errors.New("injected RemoveOwned failure")}
 if match:=f.existing[key];!match{return ErrN4ForeignRoute}
 delete(f.existing,key);return nil
}

func n4TestKeys()[]NativeRouteKey {
 return []NativeRouteKey{
  {Prefix:netip.MustParsePrefix("198.51.100.10/32"),Interface:15,NextHop:netip.MustParseAddr("192.0.2.1"),Metric:5},
  {Prefix:netip.MustParsePrefix("0.0.0.0/1"),Interface:21,NextHop:netip.MustParseAddr("0.0.0.0"),Metric:5},
  {Prefix:netip.MustParsePrefix("128.0.0.0/1"),Interface:21,NextHop:netip.MustParseAddr("0.0.0.0"),Metric:5},
 }
}
func n4TestManager(t *testing.T)(*N4OwnedRoutes,*n4FakeRoute,*n4FakeGuard){
 t.Helper()
 r:=&n4FakeRoute{existing:make(map[NativeRouteKey]bool)}
 g:=&n4FakeGuard{durable:true}
 return &N4OwnedRoutes{Path:filepath.Join(t.TempDir(),"n4-owned-state.json"),Route:r,Guard:g},r,g
}
func TestN4NativeTxnSuccessfulApplyCleanupTwice(t *testing.T){
 n,r,g:=n4TestManager(t);ctx:=context.Background()
 if err:=n.ApplyRoutes(ctx,n4TestKeys());err!=nil{t.Fatal(err)}
 state,err:=n4ReadJournal(n.Path)
 if err!=nil || state.Phase!="active" || len(state.Entries)!=3{t.Fatalf("state=%+v error=%v",state,err)}
 if !g.active || len(r.existing)!=3 {t.Fatal("guard not kept active across capture lifetime")}
 if err=n.Recover(ctx);err!=nil{t.Fatal(err)}
 if err=n.Recover(ctx);err!=nil{t.Fatalf("idempotent cleanup failed: %v",err)}
 if len(r.existing)!=0 || g.active || g.disarmCount!=1 {t.Fatalf("not fully cleaned: routes=%v guard=%+v",r.existing,g)}
 if _,err=os.Stat(n.Path);!errors.Is(err,os.ErrNotExist){t.Fatalf("journal retained after clean: %v",err)}
}
func TestN4NativeTxnCrashRecoveryFreshManager(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n.ApplyRoutes(context.Background(),n4TestKeys());err!=nil{t.Fatal(err)}
 fresh:=&N4OwnedRoutes{Path:n.Path,Route:r,Guard:g}
 if err:=fresh.ApplyRoutes(context.Background(),n4TestKeys());!errors.Is(err,ErrN4JournalConflict) {
  t.Fatalf("refused to detect previous instance journal: %v",err)
 }
 if err:=fresh.Recover(context.Background());err!=nil{t.Fatal(err)}
 if len(r.existing)!=0 {t.Fatal("crash recovery leaked owned routes")}
}
func TestN4NativeTxnForeignPreexistingNeverDeleted(t *testing.T){
 n,r,g:=n4TestManager(t);keys:=n4TestKeys()
 r.existing[keys[0]]=false
 if err:=n.ApplyRoutes(context.Background(),keys);!errors.Is(err,ErrN4ForeignRoute){t.Fatalf("unexpected error: %v",err)}
 if !g.active {t.Fatal("crash-persistent guard disengaged before recovery")}
 if err:=n.Recover(context.Background());err!=nil{t.Fatal(err)}
 if match,found:=r.existing[keys[0]];!found || match {t.Fatal("foreign route was overwritten/deleted")}
 if r.removed!=0 {t.Fatal("foreign route deletion attempted")}
}
func TestN4NativeTxnCrashWindowAmbiguousIntentNeverDeletes(t *testing.T){
 n,r,_:=n4TestManager(t);keys:=n4TestKeys()
 r.createErrAt=1
 if err:=n.ApplyRoutes(context.Background(),keys);err==nil{t.Fatal("injected failure not observed")}
 // Simulate outside actor created a route identical to the interrupted intent.
 r.existing[keys[0]]=true
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4RouteAmbiguous) {
  t.Fatalf("ambiguous must halt: %v",err)
 }
 if r.removed!=0 {t.Fatal("ambiguous route deleted")}
 if _,err:=os.Stat(n.Path);err!=nil{t.Fatalf("journal lost: %v",err)}
}
func TestN4NativeTxnPartialFailureRollsBackConfirmedOnly(t *testing.T){
 n,r,_:=n4TestManager(t);keys:=n4TestKeys()
 r.createErrAt=2
 if err:=n.ApplyRoutes(context.Background(),keys);err==nil{t.Fatal("missing injected failure")}
 if len(r.existing)!=1{t.Fatalf("expected first confirmed route: %v",r.existing)}
 if err:=n.Recover(context.Background());err!=nil{t.Fatal(err)}
 if len(r.existing)!=0{t.Fatalf("confirmed first route remained: %v",r.existing)}
 if r.removed!=1{t.Fatalf("removed %d routes want 1",r.removed)}
}
func TestN4NativeTxnDeleteFailureKeepsJournalThenRetries(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n.ApplyRoutes(context.Background(),n4TestKeys());err!=nil{t.Fatal(err)}
 r.removeErrAt=1
 if err:=n.Recover(context.Background());err==nil{t.Fatal("expected RemoveOwned failure")}
 if !g.active{t.Fatal("guard disabled after route deletion failed")}
 if _,err:=os.Stat(n.Path);err!=nil{t.Fatal("journal erased during partial deletion")}
 r.removeErrAt=0
 if err:=n.Recover(context.Background());err!=nil{t.Fatal(err)}
 if len(r.existing)!=0{t.Fatal("missing delete on retry")}
}
func TestN4NativeTxnCorruptedOrMissingGuardFailsClosed(t *testing.T){
 n,r,g:=n4TestManager(t);keys:=n4TestKeys()
 g.durable=false
 if err:=n.ApplyRoutes(context.Background(),keys);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("volatile WFP accepted: %v",err)}
 if len(r.existing)!=0 || g.armCount!=0 {t.Fatal("guard bypassed")}
 g.durable=true
 if err:=n.ApplyRoutes(context.Background(),keys);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(n.Path,[]byte("tampered"),0600);err!=nil{t.Fatal(err)}
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4JournalConflict){t.Fatalf("corruption permitted: %v",err)}
 if !g.active || len(r.existing)!=3 {t.Fatal("corrupt journal caused unsafe delete")}
}
func TestN4NativeTxnInterruptedGuardArmNeverEnablesRoute(t *testing.T){
 n,r,g:=n4TestManager(t);g.armErr=errors.New("injected guard failure")
 if err:=n.ApplyRoutes(context.Background(),n4TestKeys());!errors.Is(err,ErrN4NativeGuard){t.Fatalf("error %v",err)}
 if r.created!=0 {t.Fatal("route created without protection")}
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatalf("partial guard journal erased: %v",err)}
}
func TestN4NativeTxnCancelledMidApplyRecordsOwnedState(t *testing.T){
 n,r,g:=n4TestManager(t);ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 r.cancel=cancel;r.cancelAt=1
 if err:=n.ApplyRoutes(ctx,n4TestKeys());!errors.Is(err,context.Canceled){t.Fatalf("cancel ignored: %v",err)}
 if !g.active {t.Fatal("capture guard removed on cancellation")}
 if err:=n.Recover(context.Background());err!=nil{t.Fatal(err)}
 if len(r.existing)!=0{t.Fatal("cancelled route remains")}
}
func TestN4NativeTxnRejectsInvalidRouteAndMissingBackend(t *testing.T){
 n,r,g:=n4TestManager(t);keys:=n4TestKeys()
 tests:=[]struct{name string;mutate func([]NativeRouteKey)}{
  {"zero-iface",func(x []NativeRouteKey){x[0].Interface=0}},
  {"non-canonical",func(x []NativeRouteKey){x[0].Prefix=netip.MustParsePrefix("192.0.2.17/24")}},
  {"ipv6",func(x []NativeRouteKey){x[0].NextHop=netip.MustParseAddr("::1")}},
  {"bad-metric",func(x []NativeRouteKey){x[0].Metric=0}},
 }
 for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){
  clone:=append([]NativeRouteKey(nil),keys...);tc.mutate(clone)
  if err:=n.ApplyRoutes(context.Background(),clone);!errors.Is(err,ErrNetworkPlan){t.Fatalf("invalid route accepted: %v",err)}
 })}
 if g.armCount!=0 || r.created!=0 {t.Fatal("invalid plan touched OS")}
 n.Guard=nil
 if err:=n.ApplyRoutes(context.Background(),keys);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("missing guard: %v",err)}
}
func TestN4NativeTxnGoRaceConcurrentApplyFenced(t *testing.T){
 n,r,_:=n4TestManager(t)
 var wg sync.WaitGroup
 results:=make(chan error,16)
 for i:=0;i<16;i++ {wg.Add(1);go func(){defer wg.Done();results<-n.ApplyRoutes(context.Background(),n4TestKeys())}()}
 wg.Wait();close(results)
 ok,conflicts:=0,0
 for err:=range results {switch{
 case err==nil: ok++
 case errors.Is(err,ErrN4JournalConflict): conflicts++
 default: t.Fatalf("unexpected concurrent result %v",err)
 }}
 if ok!=1 || conflicts!=15 || len(r.existing)!=3{t.Fatalf("ok=%d conflicts=%d routes=%d",ok,conflicts,len(r.existing))}
 if err:=n.Recover(context.Background());err!=nil{t.Fatal(err)}
}
func TestN4CompactOwnedValidatorToleratesCNInMemoryOnly(t *testing.T){
 cfg:=Config{
  AdapterAlias:"WBD",TunnelMTU:1249,Lease4:netip.MustParsePrefix("10.66.0.7/32"),
  Server4:netip.MustParseAddr("198.51.100.10"),
  Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
  StatePath:"test-native-state.json",
  DNSServers4:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
 }
 plan,err:=BuildCompactNetworkPlan(cfg);if err!=nil{t.Fatal(err)}
 underlay,err:=validateN4CompactOwned(plan)
 if err!=nil || len(underlay)!=1 || len(plan.CaptureRoutes)!=4 {t.Fatalf("validation err=%v keys=%v plan=%+v",err,underlay,plan)}
 plan.CaptureRoutes=append(plan.CaptureRoutes,OwnedRoute{
   Kind:"capture",Prefix:netip.MustParsePrefix("203.0.113.0/24"),AdapterAlias:"WBD",
 })
 if _,err=validateN4CompactOwned(plan);!errors.Is(err,ErrNetworkPlan){t.Fatal("CN route leaked into host route policy")}
}


func TestN4CompactNativeRouteKeysPhysicalEscapeBeforeCapture(t *testing.T){
 cfg:=Config{
  AdapterAlias:"WBD",TunnelMTU:1249,Lease4:netip.MustParsePrefix("10.66.0.7/32"),
  Server4:netip.MustParseAddr("198.51.100.10"),
  Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
  StatePath:"test-native-state.json",
  DNSServers4:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
 }
 plan,err:=BuildCompactNetworkPlan(cfg);if err!=nil{t.Fatal(err)}
 keys,err:=N4BuildCompactNativeRouteKeys(plan,21);if err!=nil{t.Fatal(err)}
 if len(keys)!=5 || keys[0].Prefix!=netip.MustParsePrefix("198.51.100.10/32") ||
  keys[0].Interface!=12 || keys[0].NextHop!=netip.MustParseAddr("192.0.2.1"){
  t.Fatalf("physical underlay escape missing/late: %+v",keys)
 }
 expected:=map[netip.Prefix]bool{
  netip.MustParsePrefix("0.0.0.0/1"):false,
  netip.MustParsePrefix("128.0.0.0/1"):false,
  netip.MustParsePrefix("1.1.1.1/32"):false,
  netip.MustParsePrefix("8.8.8.8/32"):false,
 }
 for _,k:=range keys[1:] {
  if _,ok:=expected[k.Prefix];!ok || k.Interface!=21 ||
   k.NextHop!=netip.IPv4Unspecified() || k.Metric!=5 {
   t.Fatalf("noncompact or unbound capture: %+v",k)
  }
  expected[k.Prefix]=true
 }
 for p,ok:=range expected{if !ok{t.Fatalf("capture missing: %v",p)}}
 if err:=n4ValidateRouteBatch(keys);err!=nil{t.Fatal(err)}
 if _,err:=N4BuildCompactNativeRouteKeys(plan,12);!errors.Is(err,ErrNetworkPlan) {
  t.Fatalf("physical/Wintun loopback accepted: %v",err)
 }
 if _,err:=N4BuildCompactNativeRouteKeys(plan,0);!errors.Is(err,ErrNetworkPlan) {
  t.Fatalf("missing Wintun interface accepted: %v",err)
 }
 plan.CaptureRoutes=append(plan.CaptureRoutes,OwnedRoute{
  Kind:"capture",Prefix:netip.MustParsePrefix("203.0.113.0/24"),AdapterAlias:"WBD",
 })
 if _,err:=N4BuildCompactNativeRouteKeys(plan,21);!errors.Is(err,ErrNetworkPlan) {
  t.Fatalf("CIDR expansion escaped plan preflight: %v",err)
 }
}
func TestN4NativeTxnRejectsNoncompactRouteBatchBeforeGuard(t *testing.T){
 good:=n4TestKeys()
 tests:=[]struct{name string; mutate func([]NativeRouteKey)[]NativeRouteKey}{
  {"missing-high-half",func(x []NativeRouteKey)[]NativeRouteKey{return x[:2]}},
  {"wrong-escape-order",func(x []NativeRouteKey)[]NativeRouteKey{x[0],x[1]=x[1],x[0];return x}},
  {"duplicate-half",func(x []NativeRouteKey)[]NativeRouteKey{x[2]=x[1];return x}},
  {"default-route",func(x []NativeRouteKey)[]NativeRouteKey{x[1].Prefix=netip.MustParsePrefix("0.0.0.0/0");return x}},
  {"CN-route",func(x []NativeRouteKey)[]NativeRouteKey{return append(x,NativeRouteKey{Prefix:netip.MustParsePrefix("203.0.113.0/24"),Interface:21,NextHop:netip.IPv4Unspecified(),Metric:5})}},
  {"capture-gateway",func(x []NativeRouteKey)[]NativeRouteKey{x[1].NextHop=netip.MustParseAddr("192.0.2.1");return x}},
  {"capture-on-physical",func(x []NativeRouteKey)[]NativeRouteKey{x[1].Interface=15;return x}},
  {"split-capture-interface",func(x []NativeRouteKey)[]NativeRouteKey{x[2].Interface=22;return x}},
  {"bad-metric",func(x []NativeRouteKey)[]NativeRouteKey{x[2].Metric=6;return x}},
  {"second-escape",func(x []NativeRouteKey)[]NativeRouteKey{return append(x,NativeRouteKey{Prefix:netip.MustParsePrefix("203.0.113.4/32"),Interface:15,NextHop:netip.MustParseAddr("192.0.2.1"),Metric:5})}},
 }
 for _,tt:=range tests {t.Run(tt.name,func(t *testing.T){
  n,r,g:=n4TestManager(t)
  keys:=tt.mutate(append([]NativeRouteKey(nil),good...))
  if err:=n.ApplyRoutes(context.Background(),keys);!errors.Is(err,ErrNetworkPlan){
   t.Fatalf("invalid route batch accepted: %v",err)
  }
  if g.armCount!=0 || r.created!=0 {t.Fatal("OS state touched before strict route preflight")}
  if _,err:=os.Stat(n.Path);!errors.Is(err,os.ErrNotExist) {t.Fatalf("journal unexpectedly created: %v",err)}
 })}
}


func TestN4NativeTxnCrashAfterGuardArmBeforeJournalMarker(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n4NewJournal(n.Path,n4Journal{Phase:"applying"});err!=nil{t.Fatal(err)}
 if err:=g.Arm(context.Background());err!=nil{t.Fatal(err)}
 fresh:=&N4OwnedRoutes{Path:n.Path,Route:r,Guard:g}
 if err:=fresh.Recover(context.Background());err!=nil{t.Fatal(err)}
 if g.active || g.disarmCount!=1 || r.removed!=0{t.Fatal("pre-marker recovery touched routes or leaked guard")}
 if _,err:=os.Stat(n.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
 if err:=fresh.Recover(context.Background());err!=nil{t.Fatal(err)}
}
func TestN4NativeTxnUnverifiedPreArmGuardStillBlocksCleanup(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n4NewJournal(n.Path,n4Journal{Phase:"applying"});err!=nil{t.Fatal(err)}
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatal(err)}
 if g.disarmCount!=0 || r.removed!=0{t.Fatal("unverified WFP state discarded")}
 if _,err:=os.Stat(n.Path);err!=nil{t.Fatal(err)}
}
func TestN4NativeTxnCrashAfterDisarmBeforeUnlink(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n.ApplyRoutes(context.Background(),n4TestKeys());err!=nil{t.Fatal(err)}
 for _,key:=range n4TestKeys(){delete(r.existing,key)}
 if err:=n4WriteJournal(n.Path,n4Journal{Phase:"cleaning",GuardArmed:true});err!=nil{t.Fatal(err)}
 if err:=g.Disarm(context.Background());err!=nil{t.Fatal(err)}
 if err:=(&N4OwnedRoutes{Path:n.Path,Route:r,Guard:g}).Recover(context.Background());err!=nil{t.Fatal(err)}
 if g.active || r.removed!=0{t.Fatal("incorrect recovery after disarm")}
 if _,err:=os.Stat(n.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
}
func TestN4NativeTxnGuardLossWithRoutesFailsClosed(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n.ApplyRoutes(context.Background(),n4TestKeys());err!=nil{t.Fatal(err)}
 g.active=false
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatal(err)}
 if len(r.existing)!=3 || g.disarmCount!=0{t.Fatal("routes deleted without guard")}
 if _,err:=os.Stat(n.Path);err!=nil{t.Fatal(err)}
}
func TestN4NativeTxnEmptyCleanupRequiresSafeIdempotentDisarm(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=n4NewJournal(n.Path,n4Journal{Phase:"applying"});err!=nil{t.Fatal(err)}
 if err:=n4WriteJournal(n.Path,n4Journal{Phase:"cleaning",GuardArmed:true});err!=nil{t.Fatal(err)}
 g.disarmErr=ErrN4ForeignRoute
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4ForeignRoute){t.Fatal(err)}
 if _,err:=os.Stat(n.Path);err!=nil{t.Fatal(err)}
 if r.removed!=0{t.Fatal("removed unowned route")}
}
func TestN4NativeTxnJournalSizeAndImpossibleStateRejected(t *testing.T){
 n,r,g:=n4TestManager(t)
 if err:=os.WriteFile(n.Path,make([]byte,n4MaxJournalBytes+1),0600);err!=nil{t.Fatal(err)}
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4JournalConflict){t.Fatal(err)}
 for _,j:=range []n4Journal{
  {Phase:"applying",GuardArmed:false,Entries:[]n4Entry{{Key:n4TestKeys()[0],State:"owned"}}},
  {Phase:"active",GuardArmed:false},
  {Phase:"active",GuardArmed:true,Entries:[]n4Entry{{Key:n4TestKeys()[0],State:"owned"}}},
  {Phase:"cleaning",GuardArmed:true,Entries:[]n4Entry{{Key:NativeRouteKey{Prefix:netip.MustParsePrefix("203.0.113.0/24"),Interface:4,NextHop:netip.MustParseAddr("192.0.2.1"),Metric:5},State:"owned"}}},
 }{
  if err:=n4WriteJournal(n.Path,j);err!=nil{t.Fatal(err)}
  if err:=n.Recover(context.Background());!errors.Is(err,ErrN4JournalConflict){t.Fatalf("accepted invalid journal %+v: %v",j,err)}
 }
 if r.removed!=0 || g.disarmCount!=0{t.Fatal("invalid journal caused side effects")}
}

type n4RecoveredPartialGuard struct{
 n4FakeGuard
 finalizations int
}
func(g *n4RecoveredPartialGuard) DisarmAfterRoutesRemoved(context.Context)error{
 g.finalizations++
 g.active=false
 return nil
}
func TestN4NativeTxnPartialGuardFinalizeOnlyAfterEmptyJournal(t *testing.T){
 n,r,_:=n4TestManager(t)
 guard:=&n4RecoveredPartialGuard{n4FakeGuard:n4FakeGuard{durable:true}}
 n.Guard=guard
 if err:=n.ApplyRoutes(context.Background(),n4TestKeys());err!=nil{t.Fatal(err)}
 // A guard lost some filters during a crash while routes still exist.
 guard.active=false
 if err:=n.Recover(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatalf("guard loss while routes live must halt: %v",err)}
 if guard.finalizations!=0 || len(r.existing)!=3{t.Fatal("partial cleanup ran before routes cleared")}
 for _,key:=range n4TestKeys(){delete(r.existing,key)}
 if err:=n4WriteJournal(n.Path,n4Journal{Phase:"cleaning",GuardArmed:true});err!=nil{t.Fatal(err)}
 if err:=n.Recover(context.Background());err!=nil{t.Fatal(err)}
 if guard.finalizations!=1 || r.removed!=0{t.Fatal("final cleanup omitted or altered OS routes")}
 if _,err:=os.Stat(n.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
}
