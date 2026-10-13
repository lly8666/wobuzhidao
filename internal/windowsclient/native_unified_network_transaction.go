package windowsclient

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sync"
)

// N4NativeUnifiedTxn is an INACTIVE orchestration boundary, not a product
// network backend. It deliberately requires a separately implemented,
// cross-process exclusive gate and fresh created-Wintun generation proof.
// All stage implementations must use native APIs and exact-owned CAS checks;
// stages backed by a shell, foreign NIC, or fake guard cannot activate N4.
type N4UnifiedIdentity struct {
 Installation [16]byte `json:"installation"`
 Generation [16]byte `json:"generation"`
 WintunLUID uint64 `json:"wintun_luid"`
 WintunIndex uint32 `json:"wintun_index"`
}
func (v N4UnifiedIdentity) valid() bool {
 return v.Installation!=([16]byte{}) && v.Generation!=([16]byte{}) &&
  v.WintunLUID!=0 && v.WintunIndex!=0
}
type N4UnifiedExclusion interface {
 // Must hold a process-external lock for the ENTIRE Apply/Recover call.
 Acquire(context.Context) (release func(), err error)
}
type N4UnifiedOwner interface {
 // Verify fresh live process-created adapter, LUID, installation, generation.
 Verify(context.Context,N4UnifiedIdentity) error
}
// Implementations with a live Wintun handle SHOULD also implement Hold:
// ownership must not lapse after preflight and before stage writes complete.
// Hold reserves the current adapter/session until the returned release.
type N4UnifiedOwnerReservation interface {
 N4UnifiedOwner
 Hold(context.Context,N4UnifiedIdentity)(release func(),err error)
}
type N4UnifiedState string
const (
 N4UnifiedAbsent N4UnifiedState = "absent"
 N4UnifiedOwned N4UnifiedState = "owned"
 N4UnifiedPartial N4UnifiedState = "partial"
 N4UnifiedForeign N4UnifiedState = "foreign"
)
type N4UnifiedStage interface {
 Name() string
 // Snapshot is READ-ONLY; include a bounded complete pre-mutation CAS
 // receipt (never payload, secret, ticket, or packet). No OS mutations.
 Snapshot(context.Context,N4UnifiedIdentity) (json.RawMessage,error)
 // Apply requires a durably recorded intent; returns only on verified write.
 Apply(context.Context,N4UnifiedIdentity,json.RawMessage) error
 // Inspect must independently verify exact owned or original absent state.
 Inspect(context.Context,N4UnifiedIdentity,json.RawMessage)(N4UnifiedState,error)
 // Undo must enforce owner + generation and exact CAS; guard's Undo may
 // finish a partial owned WFP deletion only AFTER other stages retired.
 Undo(context.Context,N4UnifiedIdentity,json.RawMessage) error
}
type n4UnifiedRecord struct {
 Name string `json:"name"`
 State string `json:"state"`
 Before json.RawMessage `json:"before"`
}
type n4UnifiedJournal struct {
 Schema string `json:"schema"`
 Identity N4UnifiedIdentity `json:"identity"`
 Phase string `json:"phase"`
 Records []n4UnifiedRecord `json:"records"`
}
type n4UnifiedEnvelope struct {
 Data n4UnifiedJournal `json:"data"`
 SHA256 string `json:"sha256"`
}
const n4UnifiedSchema = "wbd-n4-native-network-txn/v1"
const n4UnifiedReceiptMax = 4096
var n4UnifiedOrder = [...]string{"guard","lease","mtu","dns","routes"}

type N4NativeUnifiedTxn struct {
 mu sync.Mutex
 Path string
 Identity N4UnifiedIdentity
 Gate N4UnifiedExclusion
 Owner N4UnifiedOwner
 Stages [5]N4UnifiedStage
}

func (n *N4NativeUnifiedTxn) preflight()error{
 if n==nil || n.Path=="" || !n.Identity.valid() || n.Gate==nil || n.Owner==nil {
  return ErrN4NativeGuard
 }
 for i,s:=range n.Stages{
  if s==nil || s.Name()!=n4UnifiedOrder[i]{return ErrNetworkPlan}
 }
 return nil
}
func n4UnifiedValidJournal(j n4UnifiedJournal)error{
 if j.Schema!=n4UnifiedSchema || !j.Identity.valid() ||
  (j.Phase!="applying" && j.Phase!="active" && j.Phase!="cleaning") ||
  len(j.Records)>len(n4UnifiedOrder){return ErrN4JournalConflict}
 for i,e:=range j.Records{
  if e.Name!=n4UnifiedOrder[i] ||
   (e.State!="intent" && e.State!="owned") ||
   len(e.Before)==0 || len(e.Before)>n4UnifiedReceiptMax ||
   !json.Valid(e.Before){return ErrN4JournalConflict}
  if e.State=="intent" && i!=len(j.Records)-1{return ErrN4JournalConflict}
 }
 if j.Phase=="active"{
  if len(j.Records)!=len(n4UnifiedOrder){return ErrN4JournalConflict}
  for _,e:=range j.Records{if e.State!="owned"{return ErrN4JournalConflict}}
 }
 return nil
}
func n4UnifiedEncode(j n4UnifiedJournal)([]byte,error){
 j.Schema=n4UnifiedSchema
 if err:=n4UnifiedValidJournal(j);err!=nil{return nil,err}
 payload,err:=json.Marshal(j);if err!=nil{return nil,err}
 sha:=sha256.Sum256(payload)
 return json.Marshal(n4UnifiedEnvelope{Data:j,SHA256:hex.EncodeToString(sha[:])})
}
func n4UnifiedDecode(b []byte)(n4UnifiedJournal,error){
 var e n4UnifiedEnvelope
 if err:=json.Unmarshal(b,&e);err!=nil{return n4UnifiedJournal{},errors.Join(ErrN4JournalConflict,err)}
 if err:=n4UnifiedValidJournal(e.Data);err!=nil{return n4UnifiedJournal{},err}
 payload,err:=json.Marshal(e.Data);if err!=nil{return n4UnifiedJournal{},err}
 sha:=sha256.Sum256(payload)
 if e.SHA256!=hex.EncodeToString(sha[:]){return n4UnifiedJournal{},ErrN4JournalConflict}
 return e.Data,nil
}
func n4UnifiedRead(path string)(n4UnifiedJournal,error){
 f,err:=os.Open(path);if err!=nil{return n4UnifiedJournal{},err}
 defer f.Close()
 b,err:=io.ReadAll(io.LimitReader(f,n4MaxJournalBytes+1))
 if err!=nil{return n4UnifiedJournal{},err}
 if len(b)>n4MaxJournalBytes{return n4UnifiedJournal{},ErrN4JournalConflict}
 return n4UnifiedDecode(b)
}
func n4UnifiedWrite(path string,j n4UnifiedJournal,newFile bool)error{
 b,err:=n4UnifiedEncode(j);if err!=nil{return err}
 if len(b)>n4MaxJournalBytes{return ErrN4JournalConflict}
 dir:=filepath.Dir(path)
 if err:=os.MkdirAll(dir,0700);err!=nil{return err}
 if newFile{
  f,err:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
  if errors.Is(err,os.ErrExist){return ErrN4JournalConflict}
  if err!=nil{return err}
  _,err=f.Write(b)
  if err==nil{err=f.Sync()}
  return errors.Join(err,f.Close())
 }
 f,err:=os.CreateTemp(dir,".wbd-n4-unified-*");if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(b);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}
func(n *N4NativeUnifiedTxn) withGate(ctx context.Context)(func(),error){
 if err:=n.preflight();err!=nil{return nil,err}
 if err:=ctx.Err();err!=nil{return nil,err}
 gateRelease,err:=n.Gate.Acquire(ctx)
 if err!=nil{return nil,err}
 if gateRelease==nil{return nil,ErrN4NativeGuard}
 if holder,ok:=n.Owner.(N4UnifiedOwnerReservation);ok{
  ownerRelease,err:=holder.Hold(ctx,n.Identity)
  if err!=nil || ownerRelease==nil{
   gateRelease()
   if err!=nil{return nil,err}
   return nil,ErrN4NativeGuard
  }
  return func(){ownerRelease();gateRelease()},nil
 }
 if err:=n.Owner.Verify(ctx,n.Identity);err!=nil{
  gateRelease()
  return nil,err
 }
 return gateRelease,nil
}
func(n *N4NativeUnifiedTxn) Apply(ctx context.Context)error{
 n.mu.Lock();defer n.mu.Unlock()
 release,err:=n.withGate(ctx);if err!=nil{return err}
 defer release()
 // A second Apply must reject the existing durable transaction BEFORE
 // querying OS snapshots: existing owned state is not a new foreign baseline.
 // This is an early, readable error only; O_EXCL at journal creation remains
 // the authoritative cross-process create-conflict check.
 if _,err:=os.Stat(n.Path);err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 // Obtain ALL immutable baselines before ANY mutation; no stale Wintun
 // handle or unexpected Windows setting may be silently overwritten.
 var before [5]json.RawMessage
 for i,s:=range n.Stages{
  if err:=ctx.Err();err!=nil{return err}
  v,err:=s.Snapshot(ctx,n.Identity);if err!=nil{return err}
  if len(v)==0 || len(v)>n4UnifiedReceiptMax || !json.Valid(v){return ErrNetworkPlan}
  before[i]=append(json.RawMessage(nil),v...)
 }
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:n.Identity,Phase:"applying"}
 if err:=n4UnifiedWrite(n.Path,j,true);err!=nil{return err}
 for i,s:=range n.Stages{
  if err:=ctx.Err();err!=nil{return err}
  j.Records=append(j.Records,n4UnifiedRecord{Name:s.Name(),State:"intent",Before:before[i]})
  if err:=n4UnifiedWrite(n.Path,j,false);err!=nil{return err}
  if err:=s.Apply(ctx,n.Identity,before[i]);err!=nil{return fmt.Errorf("native %s apply: %w",s.Name(),err)}
  observed,err:=s.Inspect(ctx,n.Identity,before[i])
  if err!=nil{return err}
  if observed!=N4UnifiedOwned{return ErrN4RouteAmbiguous}
  j.Records[i].State="owned"
  if err:=n4UnifiedWrite(n.Path,j,false);err!=nil{return err}
 }
 j.Phase="active"
 return n4UnifiedWrite(n.Path,j,false)
}
func(n *N4NativeUnifiedTxn) Recover(ctx context.Context)error{
 n.mu.Lock();defer n.mu.Unlock()
 release,err:=n.withGate(ctx);if err!=nil{return err}
 defer release()
 j,err:=n4UnifiedRead(n.Path)
 if errors.Is(err,os.ErrNotExist){return nil}
 if err!=nil{return err}
 if j.Identity!=n.Identity{return ErrN4JournalConflict}
 // If ANY non-guard network state might remain, the persistent full WFP
 // guard is mandatory before removing even one owned resource.
 if len(j.Records)>1{
  if j.Records[0].State!="owned"{return ErrN4NativeGuard}
  observed,err:=n.Stages[0].Inspect(ctx,n.Identity,j.Records[0].Before)
  if err!=nil{return err}
  if observed!=N4UnifiedOwned{return ErrN4NativeGuard}
 }
 j.Phase="cleaning"
 if err:=n4UnifiedWrite(n.Path,j,false);err!=nil{return err}
 for len(j.Records)>0{
  if err:=ctx.Err();err!=nil{return err}
  i:=len(j.Records)-1
  rec:=j.Records[i]
  observed,err:=n.Stages[i].Inspect(ctx,n.Identity,rec.Before)
  if err!=nil{return err}
  if observed==N4UnifiedForeign{return ErrN4ForeignRoute}
  if rec.State=="intent" && observed!=N4UnifiedAbsent{
   // Only WFP guard itself can be retired after an interrupted Arm: it
   // has deterministic installation-owned object IDs and no other stages
   // could have been called prior to persisting its "owned" marker.
   if i!=0 || observed!=N4UnifiedOwned{return ErrN4RouteAmbiguous}
  }
  if observed==N4UnifiedPartial && i!=0{return ErrN4NativeGuard}
  if observed!=N4UnifiedAbsent {
   if observed!=N4UnifiedOwned && !(i==0 && observed==N4UnifiedPartial){return ErrN4RouteAmbiguous}
   if err:=n.Stages[i].Undo(ctx,n.Identity,rec.Before);err!=nil{return err}
  }
  j.Records=j.Records[:i]
  if err:=n4UnifiedWrite(n.Path,j,false);err!=nil{return err}
 }
 return os.Remove(n.Path)
}
