//go:build windows

package windowsclient

import (
    "context"
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "testing"
)

// Fake WFP is intentionally confined to unit tests. No live WFP filters
// may be installed on a shared hosted Actions runner for this recovery.
type n4OwnedOrphanFakeGuard struct {
    state N4UnifiedState
    inspectErr error
    disarmErr error
    inspectCalls int
    disarmCalls int
}
func (g *n4OwnedOrphanFakeGuard) ownedState(ctx context.Context) (N4UnifiedState,error) {
    g.inspectCalls++
    if err:=ctx.Err();err!=nil{return "",err}
    return g.state,g.inspectErr
}
func (g *n4OwnedOrphanFakeGuard) DisarmAfterRoutesRemoved(ctx context.Context)error {
    g.disarmCalls++
    if err:=ctx.Err();err!=nil{return err}
    if g.disarmErr!=nil{return g.disarmErr}
    g.state=N4UnifiedAbsent
    return nil
}
func n4GuardOwnedOrphanID() N4UnifiedIdentity {
    id:=n4OrphanTestIdentity()
    // Synthetic identity cannot collide with any interface on an Actions VM;
    // the real GetAdaptersAddresses scan is still required to complete.
    id.WintunIndex=4294967294
    id.WintunLUID=0xfffffffffffffff0
    return id
}
func n4OwnedGuardJournal(t *testing.T,id N4UnifiedIdentity)string{
    t.Helper()
    path:=filepath.Join(t.TempDir(),"guard-owned.json")
    j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying",
      Records:[]n4UnifiedRecord{{Name:"guard",State:"owned",Before:json.RawMessage(`{"absent":true}`)}}}
    if err:=n4UnifiedWrite(path,j,true);err!=nil{t.Fatal(err)}
    return path
}
func TestN4WindowsOwnedGuardOrphanRetireRejectsAmbiguousWALBeforeWFP(t *testing.T){
    id:=n4GuardOwnedOrphanID()
    path:=n4OwnedGuardJournal(t,id)
    guard:=&n4OwnedOrphanFakeGuard{state:N4UnifiedOwned}
    wrong:=id;wrong.Generation[0]^=1
    if err:=n4RetireOwnedGuardOrphanWAL(context.Background(),path,wrong,guard);!errors.Is(err,ErrN4JournalConflict){
        t.Fatalf("foreign generation accepted: %v",err)
    }
    if guard.inspectCalls!=0{t.Fatal("invalid journal inspected WFP")}
    // Sidecars are vetoed even if content is corrupt/unparseable.
    if err:=os.WriteFile(path+".routes",[]byte("ambiguous route WAL"),0600);err!=nil{t.Fatal(err)}
    if err:=n4RetireOwnedGuardOrphanWAL(context.Background(),path,id,guard);!errors.Is(err,ErrN4JournalConflict){
        t.Fatalf("route sidecar allowed: %v",err)
    }
    if err:=os.Remove(path+".routes");err!=nil{t.Fatal(err)}
    if guard.inspectCalls!=0 || guard.disarmCalls!=0{t.Fatal("ambiguous WAL invoked WFP")}
    if _,err:=os.Stat(path);err!=nil{t.Fatalf("original WAL lost: %v",err)}
    ctx,cancel:=context.WithCancel(context.Background());cancel()
    if err:=n4RetireOwnedGuardOrphanWAL(ctx,path,id,guard);!errors.Is(err,context.Canceled){
        t.Fatalf("cancelled call continued: %v",err)
    }
    if guard.disarmCalls!=0{t.Fatal("cancelled WFP mutated")}
}
func TestN4WindowsOwnedGuardOrphanRetireFakeWFPScopedStates(t *testing.T){
    for _,state:=range []N4UnifiedState{N4UnifiedOwned,N4UnifiedPartial,N4UnifiedAbsent}{
        t.Run(string(state),func(t *testing.T){
            id:=n4GuardOwnedOrphanID()
            path:=n4OwnedGuardJournal(t,id)
            guard:=&n4OwnedOrphanFakeGuard{state:state}
            if err:=n4RetireOwnedGuardOrphanWAL(context.Background(),path,id,guard);err!=nil{
                t.Fatalf("guard-only recovery with FAKE WFP: %v",err)
            }
            want:=1
            if state==N4UnifiedAbsent{want=0}
            if guard.disarmCalls!=want{t.Fatalf("unexpected WFP disarm count %d",guard.disarmCalls)}
            if guard.inspectCalls<2{t.Fatal("missing post-disarm WFP CAS verification")}
            if _,err:=os.Stat(path);!errors.Is(err,os.ErrNotExist){t.Fatalf("WAL not retired: %v",err)}
            if err:=n4RetireOwnedGuardOrphanWAL(context.Background(),path,id,guard);err!=nil{
                t.Fatalf("repeat Stop not idempotent: %v",err)
            }
        })
    }
}
func TestN4WindowsOwnedGuardOrphanRetireNeverDiscardsFailedWFP(t *testing.T){
    cases:=[]struct{
        state N4UnifiedState
        inspectErr error
        disarmErr error
    }{
        {state:N4UnifiedForeign},
        {state:N4UnifiedOwned,inspectErr:ErrN4ForeignRoute},
        {state:N4UnifiedPartial,disarmErr:ErrN4NativeGuard},
    }
    for _,tt:=range cases {
        t.Run(string(tt.state),func(t *testing.T){
            id:=n4GuardOwnedOrphanID()
            path:=n4OwnedGuardJournal(t,id)
            guard:=&n4OwnedOrphanFakeGuard{state:tt.state,inspectErr:tt.inspectErr,disarmErr:tt.disarmErr}
            if err:=n4RetireOwnedGuardOrphanWAL(context.Background(),path,id,guard);err==nil{
                t.Fatal("foreign/failed WFP treated as retired")
            }
            if _,err:=os.Stat(path);err!=nil{t.Fatalf("unfinished WAL lost: %v",err)}
        })
    }
}
