package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sync"
 "testing"
)

type n4UnifiedTestGate struct{ mu sync.Mutex }
func(g *n4UnifiedTestGate) Acquire(ctx context.Context)(func(),error){
 if err:=ctx.Err();err!=nil{return nil,err}
 g.mu.Lock()
 return func(){g.mu.Unlock()},nil
}
type n4UnifiedTestOwner struct{ err error; count int }
func(o *n4UnifiedTestOwner) Verify(_ context.Context, id N4UnifiedIdentity)error{
 o.count++
 if !id.valid(){return ErrN4ForeignRoute}
 return o.err
}
type n4UnifiedTestStage struct{
 name string
 state N4UnifiedState
 trace *[]string
 applyErr error
 undoErr error
 applyThenCancel context.CancelFunc
 undoCount int
}
func(s *n4UnifiedTestStage) Name()string{return s.name}
func(s *n4UnifiedTestStage) Snapshot(_ context.Context,id N4UnifiedIdentity)(json.RawMessage,error){
 *s.trace=append(*s.trace,"snapshot:"+s.name)
 if s.state!=N4UnifiedAbsent{return nil,ErrN4ForeignRoute}
 return json.RawMessage(fmt.Sprintf(`{"interface":%d,"stage":"%s"}`,id.WintunIndex,s.name)),nil
}
func(s *n4UnifiedTestStage) Apply(_ context.Context,_ N4UnifiedIdentity,_ json.RawMessage)error{
 *s.trace=append(*s.trace,"apply:"+s.name)
 if s.applyErr!=nil{return s.applyErr}
 s.state=N4UnifiedOwned
 if s.applyThenCancel!=nil{s.applyThenCancel()}
 return nil
}
func(s *n4UnifiedTestStage) Inspect(_ context.Context,_ N4UnifiedIdentity,_ json.RawMessage)(N4UnifiedState,error){
 *s.trace=append(*s.trace,"inspect:"+s.name)
 return s.state,nil
}
func(s *n4UnifiedTestStage) Undo(_ context.Context,_ N4UnifiedIdentity,_ json.RawMessage)error{
 *s.trace=append(*s.trace,"undo:"+s.name)
 s.undoCount++
 if s.undoErr!=nil{return s.undoErr}
 s.state=N4UnifiedAbsent
 return nil
}
func n4UnifiedTestTxn(t *testing.T)(*N4NativeUnifiedTxn,*[5]*n4UnifiedTestStage,*[]string,*n4UnifiedTestOwner){
 t.Helper()
 var id N4UnifiedIdentity
 id.Installation[0]=11
 id.Generation[0]=23
 id.WintunIndex=21
 id.WintunLUID=0x11ce
 trace:=&[]string{}
 var st [5]*n4UnifiedTestStage
 tx:=&N4NativeUnifiedTxn{
  Path:filepath.Join(t.TempDir(),"native-unified-owned.json"),
  Identity:id,Gate:&n4UnifiedTestGate{},
 }
 owner:=&n4UnifiedTestOwner{}
 tx.Owner=owner
 for i,name:=range n4UnifiedOrder{
  st[i]=&n4UnifiedTestStage{name:name,state:N4UnifiedAbsent,trace:trace}
  tx.Stages[i]=st[i]
 }
 return tx,&st,trace,owner
}
func TestN4NativeTxnUnifiedHappyAndCrashRecovery(t *testing.T){
 tx,st,trace,_:=n4UnifiedTestTxn(t)
 if err:=tx.Apply(context.Background());err!=nil{t.Fatal(err)}
 journal,err:=n4UnifiedRead(tx.Path)
 if err!=nil || journal.Phase!="active" || len(journal.Records)!=5{t.Fatalf("active journal=%+v %v",journal,err)}
 want:=[]string{"snapshot:guard","snapshot:lease","snapshot:mtu","snapshot:dns","snapshot:routes"}
 for i,s:=range want{if (*trace)[i]!=s{t.Fatalf("all baselines must precede mutation: %v",*trace)}}
 for i,s:=range st{if s.state!=N4UnifiedOwned{t.Fatalf("stage %d not owned",i)}}
 fresh:=&N4NativeUnifiedTxn{Path:tx.Path,Identity:tx.Identity,Gate:tx.Gate,Owner:tx.Owner,Stages:tx.Stages}
 if err:=fresh.Recover(context.Background());err!=nil{t.Fatal(err)}
 if err:=fresh.Recover(context.Background());err!=nil{t.Fatalf("Stop twice: %v",err)}
 for i,s:=range st{if s.state!=N4UnifiedAbsent || s.undoCount!=1{t.Fatalf("leaked stage %d: %+v",i,s)}}
 after:=*trace
 var undone []string
 for _,line:=range after{if len(line)>=5 && line[:5]=="undo:"{undone=append(undone,line)}}
 for i,w:=range []string{"undo:routes","undo:dns","undo:mtu","undo:lease","undo:guard"}{
  if undone[i]!=w{t.Fatalf("unsafe recovery order: %v",undone)}
 }
 if _,err:=os.Stat(tx.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
}
func TestN4NativeTxnUnifiedCancelAfterOwnedStageThenRecover(t *testing.T){
 tx,st,_,_:=n4UnifiedTestTxn(t)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 st[1].applyThenCancel=cancel
 if err:=tx.Apply(ctx);!errors.Is(err,context.Canceled){t.Fatalf("expected cancel: %v",err)}
 j,err:=n4UnifiedRead(tx.Path)
 if err!=nil || len(j.Records)!=2 || j.Records[1].State!="owned"{t.Fatalf("cancel journal %+v: %v",j,err)}
 if err:=tx.Recover(context.Background());err!=nil{t.Fatal(err)}
 for i,s:=range st{if s.state!=N4UnifiedAbsent{t.Fatalf("stage %d remained owned",i)}}
}
func TestN4NativeTxnUnifiedIntentAmbiguousNeverDeletes(t *testing.T){
 tx,st,_,_:=n4UnifiedTestTxn(t)
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:tx.Identity,Phase:"applying",
  Records:[]n4UnifiedRecord{
   {Name:"guard",State:"owned",Before:json.RawMessage(`{}`)},
   {Name:"lease",State:"intent",Before:json.RawMessage(`{}`)},
  }}
 st[0].state=N4UnifiedOwned
 st[1].state=N4UnifiedOwned // post-syscall crash, before owned marker
 if err:=n4UnifiedWrite(tx.Path,j,true);err!=nil{t.Fatal(err)}
 if err:=tx.Recover(context.Background());!errors.Is(err,ErrN4RouteAmbiguous){t.Fatalf("intent illegally reclaimed: %v",err)}
 if st[1].undoCount!=0 || st[0].undoCount!=0{t.Fatal("ambiguous object deleted")}
 if _,err:=os.Stat(tx.Path);err!=nil{t.Fatal(err)}
}
func TestN4NativeTxnUnifiedGuardCrashIntentExactOwnedRecovery(t *testing.T){
 tx,st,_,_:=n4UnifiedTestTxn(t)
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:tx.Identity,Phase:"applying",
  Records:[]n4UnifiedRecord{{Name:"guard",State:"intent",Before:json.RawMessage(`{}`)}}}
 if err:=n4UnifiedWrite(tx.Path,j,true);err!=nil{t.Fatal(err)}
 st[0].state=N4UnifiedOwned
 if err:=tx.Recover(context.Background());err!=nil{t.Fatal(err)}
 if st[0].undoCount!=1{t.Fatal("fully exact-owned guard not reclaimed")}
}
func TestN4NativeTxnUnifiedGuardPartialOnlyAfterOtherStagesRetire(t *testing.T){
 tx,st,_,_:=n4UnifiedTestTxn(t)
 if err:=tx.Apply(context.Background());err!=nil{t.Fatal(err)}
 st[0].state=N4UnifiedPartial
 if err:=tx.Recover(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatalf("guard loss while routes active: %v",err)}
 for _,s:=range st{if s.undoCount!=0{t.Fatal("mutation with partial WFP guard")}}
 st[0].state=N4UnifiedOwned
 st[0].undoErr=errors.New("interrupted owned-WFP filter deletion")
 if err:=tx.Recover(context.Background());err==nil{t.Fatal("missing injected guard cleanup failure")}
 j,err:=n4UnifiedRead(tx.Path);if err!=nil || len(j.Records)!=1 || j.Phase!="cleaning"{t.Fatalf("route teardown not durably recorded: %+v %v",j,err)}
 st[0].state=N4UnifiedPartial
 st[0].undoErr=nil
 if err:=tx.Recover(context.Background());err!=nil{t.Fatal(err)}
 if st[0].state!=N4UnifiedAbsent{t.Fatal("partial WFP filter not finalized")}
}
func TestN4NativeTxnUnifiedForeignAndGenerationFence(t *testing.T){
 tx,st,_,owner:=n4UnifiedTestTxn(t)
 if err:=tx.Apply(context.Background());err!=nil{t.Fatal(err)}
 changed:=&N4NativeUnifiedTxn{Path:tx.Path,Identity:tx.Identity,Gate:tx.Gate,Owner:tx.Owner,Stages:tx.Stages}
 changed.Identity.Generation[0]++
 if err:=changed.Recover(context.Background());!errors.Is(err,ErrN4JournalConflict){t.Fatalf("wrong generation recovered: %v",err)}
 st[3].state=N4UnifiedForeign
 if err:=tx.Recover(context.Background());!errors.Is(err,ErrN4ForeignRoute){t.Fatalf("foreign DNS cleared: %v",err)}
 if st[3].undoCount!=0{t.Fatal("foreign resolver overwritten")}
 if _,err:=os.Stat(tx.Path);err!=nil{t.Fatal("journal lost after foreign state")}
 owner.err=ErrN4ForeignRoute
 if err:=tx.Recover(context.Background());!errors.Is(err,ErrN4ForeignRoute){t.Fatalf("stale owner bypassed: %v",err)}
}
func TestN4NativeTxnUnifiedJournalBoundAndTamper(t *testing.T){
 tx,st,_,_:=n4UnifiedTestTxn(t)
 if err:=os.WriteFile(tx.Path,make([]byte,n4MaxJournalBytes+1),0600);err!=nil{t.Fatal(err)}
 if err:=tx.Recover(context.Background());!errors.Is(err,ErrN4JournalConflict){t.Fatalf("oversized journal: %v",err)}
 for _,j:=range []n4UnifiedJournal{
  {Schema:n4UnifiedSchema,Identity:tx.Identity,Phase:"active"},
  {Schema:n4UnifiedSchema,Identity:tx.Identity,Phase:"applying",Records:[]n4UnifiedRecord{{Name:"routes",State:"owned",Before:json.RawMessage(`{}`)}}},
  {Schema:n4UnifiedSchema,Identity:tx.Identity,Phase:"applying",Records:[]n4UnifiedRecord{{Name:"guard",State:"invalid",Before:json.RawMessage(`{}`)}}},
 }{
  if err:=n4UnifiedWrite(tx.Path,j,false);!errors.Is(err,ErrN4JournalConflict){t.Fatalf("encoded invalid state: %+v %v",j,err)}
 }
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:tx.Identity,Phase:"applying"}
 if err:=n4UnifiedWrite(tx.Path,j,false);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(tx.Path,[]byte("tampered"),0600);err!=nil{t.Fatal(err)}
 if err:=tx.Recover(context.Background());!errors.Is(err,ErrN4JournalConflict){t.Fatalf("tampered journal accepted: %v",err)}
 for _,s:=range st{if s.undoCount!=0{t.Fatal("tampered journal mutated OS")}}
}
func TestN4NativeTxnUnifiedExclusiveCreateCollision(t *testing.T){
 tx,_,_,_:=n4UnifiedTestTxn(t)
 var wg sync.WaitGroup
 ch:=make(chan error,12)
 for i:=0;i<12;i++ {
  wg.Add(1)
  go func(){defer wg.Done();ch<-tx.Apply(context.Background())}()
 }
 wg.Wait();close(ch)
 success,conflicts:=0,0
 for e:=range ch{
  if e==nil{success++}else if errors.Is(e,ErrN4JournalConflict){conflicts++}else{t.Fatalf("unexpected concurrent result: %v",e)}
 }
 if success!=1 || conflicts!=11{t.Fatalf("success=%d conflicts=%d",success,conflicts)}
 if err:=tx.Recover(context.Background());err!=nil{t.Fatal(err)}
}
func TestN4NativeTxnUnifiedRequiresGenerationGateAndAllStages(t *testing.T){
 tx,st,_,_:=n4UnifiedTestTxn(t)
 tx.Gate=nil
 if err:=tx.Apply(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatal(err)}
 tx.Gate=&n4UnifiedTestGate{}
 tx.Identity.Generation=[16]byte{}
 if err:=tx.Apply(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatal(err)}
 tx.Identity.Generation[0]=23
 tx.Stages[4]=nil
 if err:=tx.Apply(context.Background());!errors.Is(err,ErrNetworkPlan){t.Fatal(err)}
 if _,err:=os.Stat(tx.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
 for _,s:=range st{if s.applyErr!=nil || s.undoCount!=0{t.Fatal("preflight mutated stage")}}
}
