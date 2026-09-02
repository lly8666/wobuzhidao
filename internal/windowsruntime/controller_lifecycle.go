package windowsruntime

import (
	"errors"
	"fmt"
	"sort"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func cloneLanePlans(in map[int]LanePlan) map[int]LanePlan {
	out:=make(map[int]LanePlan,len(in));for id,plan:=range in{out[id]=plan};return out
}

func gameTargetsFromPlans(plans map[int]LanePlan)([]gamelane.LaneTarget,error){
	ids:=make([]int,0,len(plans));for id:=range plans{ids=append(ids,id)};sort.Ints(ids)
	out:=make([]gamelane.LaneTarget,0,len(ids))
	for _,id:=range ids{addr,err:=LaneGameTarget(plans[id]);if err!=nil{return nil,err};out=append(out,gamelane.LaneTarget{ID:uint8(id),Address:addr})}
	return out,nil
}

func lifecycleRefForID(l *logicaltunnel.LaneLifecycle,id int)(logicaltunnel.LaneRef,error){
	if l==nil{return logicaltunnel.LaneRef{},errors.New("Logical Tunnel lifecycle is unavailable")}
	for _,snap:=range l.Snapshot(){if int(snap.Ref.ID)==id{return snap.Ref,nil}}
	return logicaltunnel.LaneRef{},fmt.Errorf("logical lane %d is not active",id)
}

func(c *Controller)bootstrapRuntimeLane(profile Profile,base Underlay,expected logicaltunnel.TunnelConfig,laneID,slot int,candidate bool)(LanePlan,Process,error){
	underlay:=base;underlay.SourcePort=nextFakeTCPSourcePort()
	var bootstrap LaneBootstrap
	var err error
	if candidate{bootstrap,err=BuildCandidateLaneBootstrapSlot(profile,underlay,laneID,slot)}else{bootstrap,err=BuildLaneBootstrap(profile,underlay,laneID)}
	if err!=nil{return LanePlan{},nil,err}
	if err:=c.tickets.Clear(bootstrap.TicketPath);err!=nil{return LanePlan{},nil,fmt.Errorf("clear lane %d Reality ticket: %w",laneID,err)}
	if err:=c.tickets.Clear(bootstrap.TunnelConfigPath);err!=nil{return LanePlan{},nil,fmt.Errorf("clear lane %d tunnel config: %w",laneID,err)}
	proc,err:=c.runner.Start(bootstrap.FakeTCP);if err!=nil{return LanePlan{},nil,fmt.Errorf("start lane %d same-flow FakeTCP: %w",laneID,err)}
	owned:=true
	defer func(){if owned{_ = proc.Stop()}}()
	if err:=waitProcessMarker(fmt.Sprintf("lane %d Reality bootstrap",laneID),proc,singleFlowBootstrapReadyMarker,singleFlowBootstrapWait);err!=nil{return LanePlan{},nil,err}
	ticket,err:=c.tickets.Read(bootstrap.TicketPath);if err!=nil{return LanePlan{},nil,fmt.Errorf("read lane %d Reality ticket: %w",laneID,err)}
	raw,err:=c.tickets.Read(bootstrap.TunnelConfigPath);if err!=nil{return LanePlan{},nil,fmt.Errorf("read lane %d tunnel config: %w",laneID,err)}
	cfg,err:=decodeAuthenticatedTunnelConfig(raw);if err!=nil{return LanePlan{},nil,err}
	bootstrap.Ticket=ticket;bootstrap.TunnelConfig=cfg
	if err:=bootstrap.ValidateAuthenticated(&expected);err!=nil{return LanePlan{},nil,err}
	var plan LanePlan
	if candidate{plan,err=BuildCandidateLanePlanSlot(profile,bootstrap,slot)}else{plan,err=BuildAuthenticatedLanePlan(profile,bootstrap)}
	if err!=nil{return LanePlan{},nil,err}
	owned=false
	return plan,proc,nil
}

// Dormant removes the one shipping public transport while deliberately retaining
// authenticated Logical Tunnel state, one TUN/NAT context, IPv6 kill-switch and
// routes. New inner packets are locally dropped until Wake succeeds; there is no
// hidden ordinary-TCP fallback and no second public transport.
func(c *Controller)Dormant()error{
	c.mu.Lock()
	if c.state!=RuntimeConnected{state:=c.state;c.mu.Unlock();return fmt.Errorf("Windows runtime cannot enter dormant while %s",state)}
	c.state=RuntimeDisconnecting
	control:=c.gameControl;plans:=cloneLanePlans(c.lanePlans);lifecycle:=c.lifecycle
	c.mu.Unlock()
	if err:=setGameLaneTargets(control,nil,gameControlTimeout);err!=nil{c.mu.Lock();c.state=RuntimeConnected;c.mu.Unlock();return fmt.Errorf("enter dormant Game barrier: %w",err)}
	ids:=make([]int,0,len(plans));for id:=range plans{ids=append(ids,id)};sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	var errs []error
	for _,id:=range ids{if err:=c.executor.StopDynamicLanePlan(plans[id]);err!=nil{errs=append(errs,fmt.Errorf("stop lane %d for dormant: %w",id,err))}}
	if lifecycle!=nil{lifecycle.Dormant()}
	c.mu.Lock();c.lanePlans=map[int]LanePlan{};c.state=RuntimeDormant;c.mu.Unlock()
	return errors.Join(errs...)
}

// Wake recreates exactly one shipping public transport using a fresh public
// source port and same-flow Reality admission. Shared local TUN/routes are not
// restarted. Product profile validation rejects any lane count other than one.
func(c *Controller)Wake()error{
	c.mu.Lock()
	if c.state!=RuntimeDormant{state:=c.state;c.mu.Unlock();return fmt.Errorf("Windows runtime cannot wake while %s",state)}
	c.state=RuntimeConnecting
	profile:=c.profile;base:=c.baseUnderlay;expected:=c.tunnelConfig;control:=c.gameControl
	c.mu.Unlock()

	if err:=logicaltunnel.ValidateProductTransportLaneCount(profile.Lanes);err!=nil{c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return err}
	plans:=make(map[int]LanePlan,1)
	started:=make([]LanePlan,0,1)
	rollback:=func(){for i:=len(started)-1;i>=0;i--{_ = c.executor.StopDynamicLanePlan(started[i])}}
	plan,proc,err:=c.bootstrapRuntimeLane(profile,base,expected,1,1,false);if err!=nil{c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return fmt.Errorf("wake lane 1 bootstrap: %w",err)}
	if err:=c.executor.StartDynamicLane(plan,proc);err!=nil{c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return fmt.Errorf("wake lane 1 transport: %w",err)}
	plans[1]=plan;started=append(started,plan)
	targets,err:=gameTargetsFromPlans(plans);if err!=nil{rollback();c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return err}
	if err:=setGameLaneTargets(control,targets,gameControlTimeout);err!=nil{rollback();c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return fmt.Errorf("wake local transport barrier: %w",err)}
	lifecycle,err:=logicaltunnel.NewLaneLifecycle(1);if err!=nil{rollback();c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return err}
	if _,err:=lifecycle.AttachInitial(1);err!=nil{rollback();c.mu.Lock();c.state=RuntimeDormant;c.mu.Unlock();return err}
	c.mu.Lock();c.lanePlans=plans;c.lifecycle=lifecycle;c.state=RuntimeConnected;c.mu.Unlock();return nil
}

// ReplaceLane used to implement make-before-break by starting a candidate public
// FakeTCP association while the old one was still active. ADR-0015 forbids that
// overlap. Shipping callers must use the break-before-make Disconnect/cleanup ->
// Connect path until a state-preserving replacement implementation can prove the
// old public association is gone before emitting the replacement SYN.
func(c *Controller)ReplaceLane(laneID int)error{
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state!=RuntimeConnected{return fmt.Errorf("Windows runtime cannot replace lane while %s",c.state)}
	if laneID!=1{return fmt.Errorf("global single-flow product has no logical lane %d",laneID)}
	return errors.New("global single-flow replacement requires break-before-make Disconnect then Connect; overlapping candidate public transport is forbidden")
}
