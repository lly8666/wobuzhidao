package runtimeowner

import (
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestN2GenerationPinnedNormalTXProposalOnlyChangesNewFECBlock(t *testing.T) {
	lease:=runtimeLease(t)
	owner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
	if err!=nil {t.Fatal(err)}
	r,err:=New(owner,nil)
	if err!=nil {t.Fatal(err)}
	defer r.Close()
	cfg:=datapath.LaneConfig{
		Role:datapath.RoleClient,ClientRecordLimit:1300,ServerRecordLimit:1250,
		Keys:runtimeKeys(),ParityShards:20,TunnelID:lease.Config.TunnelID.Bytes(),
		FlushAfter:32*time.Millisecond,MaxBlocks:8,
	}
	cfg.IncarnationNonce[0]=55
	cfg.TxMTU=runtimeMTU(cfg.ServerRecordLimit,20)
	cfg.RxMTU=runtimeMTU(cfg.ClientRecordLimit,20)
	lane,err:=datapath.NewN2AdaptiveLaneForTests(cfg)
	if err!=nil {t.Fatal(err)}
	cCfg,_:=transportPair(
		func(faketcp.Segment)error{return nil},
		func(faketcp.Segment)error{return nil},1,12000)
	snap,err:=r.AttachInitial(1,lane,cCfg)
	if err!=nil {t.Fatal(err)}
	controllerCfg:=adaptivefec.Config{Mode:adaptivefec.Auto,
		InitialParity:20,MinParity:4,MaxParity:20}
	if err:=r.ConfigureN2ControllerForTests(snap.Ref,controllerCfg);!errors.Is(err,ErrTransportConfig) {
		t.Fatalf("auto advice attached before protected quality %v",err)
	}
	if err:=r.ConfigureQualityV3(snap.Ref,cfg.IncarnationNonce);err!=nil {t.Fatal(err)}
	if err:=r.ConfigureN2ControllerForTests(snap.Ref,controllerCfg);err!=nil {t.Fatal(err)}
	if err:=r.ConfigureN2ControllerForTests(snap.Ref,controllerCfg);!errors.Is(err,ErrTransportConfig) {
		t.Fatalf("repeated controller attached to same generation %v",err)
	}
	if _,ok:=r.N2ProposalForTests(snap.Ref);ok {t.Fatal("proposal exists without any local TX returned echo")}
	at:=time.Unix(150000,0)
	old,err:=lane.Outbound([]byte("old-before-advice"),at)
	if err!=nil||len(old)!=1||old[0].FEC.ConfiguredParity!=20 {
		t.Fatalf("pre-switch protected source %d %v",len(old),err)
	}
	// The authenticated returned source evidence is separately exercised
	// by TestN2ControllerConsumesOnlyAuthenticatedLocalTXEchoOnce. Inject a
	// resulting proposal here ONLY to isolate active owner switch semantics.
	r.mu.Lock()
	tport:=r.lanes[snap.Ref]
	r.mu.Unlock()
	tport.mu.Lock()
	tport.quality.n2Last=adaptivefec.Decision{Ordinary:4,Applied:4,Reason:adaptivefec.Lower}
	tport.quality.n2HasDecision=true
	tport.quality.n2DecisionAt=at.Add(time.Millisecond)
	tport.quality.n2Pending=true
	tport.mu.Unlock()
	applied,err:=r.ApplyN2ProposalForTests(snap.Ref,at.Add(2*time.Millisecond))
	if err!=nil||!applied {t.Fatalf("valid active TX advice not applied %v %v",applied,err)}
	next,err:=lane.Outbound([]byte("new-after-advice"),at.Add(2*time.Millisecond))
	if err!=nil||len(next)!=1||next[0].FEC.ConfiguredParity!=4||
		next[0].FEC.BlockID==old[0].FEC.BlockID {
		t.Fatalf("new block failed profile isolation %d %v",len(next),err)
	}
	if applied,err:=r.ApplyN2ProposalForTests(snap.Ref,at.Add(3*time.Millisecond));err!=nil||applied {
		t.Fatalf("unchanged target reapplied %v %v",applied,err)
	}
	oldParity,err:=lane.FlushDue(at.Add(32*time.Millisecond))
	if err!=nil||len(oldParity)!=1||oldParity[0].FEC.ConfiguredParity!=20 {
		t.Fatalf("old block parity deadline changed %d %v",len(oldParity),err)
	}
	newParity,err:=lane.FlushDue(at.Add(34*time.Millisecond))
	if err!=nil||len(newParity)!=1||newParity[0].FEC.ConfiguredParity!=4 {
		t.Fatalf("new block did not retain independent deadline %d %v",len(newParity),err)
	}
	tport.mu.Lock()
	tport.quality.n2Last=adaptivefec.Decision{Ordinary:8,Applied:8,Reason:adaptivefec.HoldUnqualified}
	tport.mu.Unlock()
	if applied,err:=r.ApplyN2ProposalForTests(snap.Ref,at.Add(4*time.Millisecond));err!=nil||applied {
		t.Fatalf("UNKNOWN feedback changed profile %v %v",applied,err)
	}
	if current,ok:=owner.LaneStats(snap.Ref);!ok||current.TxPath.ParityShards!=4 {
		t.Fatalf("HOLD modified active profile %+v %v",current.TxPath,ok)
	}

	// Even a still-cached last RAISE/LOWER must expire without new quality.
	tport.mu.Lock()
	tport.quality.n2Last=adaptivefec.Decision{Ordinary:8,Applied:8,Reason:adaptivefec.Raise}
	tport.quality.n2HasDecision=true
	tport.quality.n2Pending=true
	tport.quality.n2DecisionAt=at
	tport.mu.Unlock()
	if applied,err:=r.ApplyN2ProposalForTests(snap.Ref,at.Add(11*time.Second));err!=nil||applied {
		t.Fatalf("stale recommendation applied after feedback interruption %v %v",applied,err)
	}
	if current,ok:=owner.LaneStats(snap.Ref);!ok||current.TxPath.ParityShards!=4 {
		t.Fatalf("expired recommendation changed source parity %+v %v",current.TxPath,ok)
	}
	if _,err:=r.Dormant();err!=nil {t.Fatal(err)}
	if applied,err:=r.ApplyN2ProposalForTests(snap.Ref,at.Add(15*time.Second));applied||!errors.Is(err,ErrTransportMissing) {
		t.Fatalf("dormant generation accepted stale switch %v %v",applied,err)
	}
}
