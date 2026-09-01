package windowsruntime

import (
	"errors"
	"fmt"
)

func dynamicLaneProcessNames(laneID int) (fake, dtls, link string, err error) {
	if laneID < 1 || laneID > 4 { return "", "", "", fmt.Errorf("dynamic lane id=%d out of range", laneID) }
	return fmt.Sprintf("faketcp-%d",laneID),fmt.Sprintf("dtls-%d",laneID),fmt.Sprintf("link-%d",laneID),nil
}

// StartDynamicLane takes ownership of a same-flow FakeTCP process whose bounded
// Reality-like bootstrap has already authenticated. It then brings up only this
// lane's DTLS and LINK. Shared Game/TUN/routes are intentionally untouched.
func (e *Executor) StartDynamicLane(lane LanePlan, prestartedFake Process) error {
	if prestartedFake == nil { return errors.New("dynamic lane requires prestarted FakeTCP") }
	fakeName,dtlsName,linkName,err:=dynamicLaneProcessNames(lane.ID);if err!=nil{return err}
	if lane.FakeTCP.Name!=fakeName||lane.DTLS.Name!=dtlsName||lane.Link.Name!=linkName{return errors.New("dynamic lane command names do not match lane id")}

	e.mu.Lock();defer e.mu.Unlock()
	if !e.running{return errors.New("shared Windows runtime is not running")}
	if e.cleanupPending{return errors.New("Windows runtime has pending network cleanup")}
	for _,p:=range e.processes{if p.name==fakeName||p.name==dtlsName||p.name==linkName{return fmt.Errorf("dynamic lane %d already has live process %s",lane.ID,p.name)}}
	base:=len(e.processes)
	rollback:=func(){for i:=len(e.processes)-1;i>=base;i--{_ = e.processes[i].proc.Stop()};e.processes=e.processes[:base]}

	e.processes=append(e.processes,namedProcess{name:fakeName,proc:prestartedFake})
	if err:=waitProcessReady(fakeName,prestartedFake);err!=nil{rollback();return err}
	dtls,err:=e.runner.Start(lane.DTLS);if err!=nil{rollback();return fmt.Errorf("start %s: %w",dtlsName,err)}
	e.processes=append(e.processes,namedProcess{name:dtlsName,proc:dtls})
	if err:=waitProcessReady(dtlsName,dtls);err!=nil{rollback();return err}
	link,err:=e.runner.Start(lane.Link);if err!=nil{rollback();return fmt.Errorf("start %s: %w",linkName,err)}
	e.processes=append(e.processes,namedProcess{name:linkName,proc:link})
	if err:=waitProcessReady(linkName,link);err!=nil{rollback();return err}
	return nil
}

// StopDynamicLane removes only one transport lane. Game, TUN, IPv6 state and
// routes remain alive. Callers must remove the lane from Game membership first
// so no new logical packets target a transport that is being retired.
func (e *Executor) StopDynamicLane(laneID int) error {
	fakeName,dtlsName,linkName,err:=dynamicLaneProcessNames(laneID);if err!=nil{return err}
	e.mu.Lock();defer e.mu.Unlock()
	if !e.running{return errors.New("shared Windows runtime is not running")}
	wanted:=map[string]bool{fakeName:true,dtlsName:true,linkName:true}
	found:=0
	var errs []error
	for i:=len(e.processes)-1;i>=0;i--{
		p:=e.processes[i]
		if !wanted[p.name]{continue}
		found++
		if stopErr:=p.proc.Stop();stopErr!=nil{errs=append(errs,fmt.Errorf("stop %s: %w",p.name,stopErr))}
		e.processes=append(e.processes[:i],e.processes[i+1:]...)
	}
	if found==0{return fmt.Errorf("dynamic lane %d is not running",laneID)}
	if found!=3{errs=append(errs,fmt.Errorf("dynamic lane %d process group incomplete: found=%d want=3",laneID,found))}
	return errors.Join(errs...)
}

func (e *Executor) DynamicLaneIDs() []int {
	e.mu.Lock();defer e.mu.Unlock()
	seen:=map[int]bool{}
	for _,p:=range e.processes{
		for id:=1;id<=4;id++{if p.name==fmt.Sprintf("link-%d",id){seen[id]=true}}
	}
	out:=make([]int,0,len(seen));for id:=1;id<=4;id++{if seen[id]{out=append(out,id)}};return out
}
