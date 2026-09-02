package windowsruntime

import (
	"errors"
	"fmt"
	"strings"
)

func validateDynamicLanePlan(lane LanePlan) error {
	if lane.ID!=1{return ErrOverlappingPublicFlow}
	if lane.Slot==0{lane.Slot=lane.ID}
	if lane.Slot!=1{return ErrOverlappingPublicFlow}
	if !strings.HasPrefix(lane.FakeTCP.Name,"faketcp-")||!strings.HasPrefix(lane.DTLS.Name,"dtls-")||!strings.HasPrefix(lane.Link.Name,"link-"){
		return errors.New("dynamic transport commands require FakeTCP/DTLS/LINK process names")
	}
	if lane.FakeTCP.Name==lane.DTLS.Name||lane.FakeTCP.Name==lane.Link.Name||lane.DTLS.Name==lane.Link.Name{return errors.New("dynamic transport process names must be distinct")}
	return nil
}

func laneProcessNameSet(lane LanePlan) map[string]bool {
	return map[string]bool{lane.FakeTCP.Name:true,lane.DTLS.Name:true,lane.Link.Name:true}
}

func hasPublicFakeTCPLocked(processes []namedProcess) bool {
	for _,p:=range processes{if p.name=="faketcp"||strings.HasPrefix(p.name,"faketcp-"){return true}}
	return false
}

// StartDynamicLane is now only the Dormant -> Wake transport reattachment path.
// Shared Game/TUN/routes may remain alive while dormant, but there must be zero
// public FakeTCP process groups before this method is called. It may create only
// logical transport ID/slot 1. This prevents any A+B public-flow overlap even if
// a future caller bypasses Controller.ReplaceLane.
func (e *Executor) StartDynamicLane(lane LanePlan, prestartedFake Process) error {
	if prestartedFake==nil{return errors.New("dynamic transport requires prestarted FakeTCP")}
	if err:=validateDynamicLanePlan(lane);err!=nil{return err}
	wanted:=laneProcessNameSet(lane)

	e.mu.Lock();defer e.mu.Unlock()
	if !e.running{return errors.New("shared Windows runtime is not running")}
	if e.cleanupPending{return errors.New("Windows runtime has pending network cleanup")}
	if hasPublicFakeTCPLocked(e.processes){return ErrOverlappingPublicFlow}
	for _,p:=range e.processes{if wanted[p.name]{return fmt.Errorf("dynamic transport process %s already exists",p.name)}}
	base:=len(e.processes)
	rollback:=func(){for i:=len(e.processes)-1;i>=base;i--{_ = e.processes[i].proc.Stop()};e.processes=e.processes[:base]}

	e.processes=append(e.processes,namedProcess{name:lane.FakeTCP.Name,proc:prestartedFake})
	if err:=waitProcessReady(lane.FakeTCP.Name,prestartedFake);err!=nil{rollback();return err}
	dtls,err:=e.runner.Start(lane.DTLS);if err!=nil{rollback();return fmt.Errorf("start %s: %w",lane.DTLS.Name,err)}
	e.processes=append(e.processes,namedProcess{name:lane.DTLS.Name,proc:dtls})
	if err:=waitProcessReady(lane.DTLS.Name,dtls);err!=nil{rollback();return err}
	link,err:=e.runner.Start(lane.Link);if err!=nil{rollback();return fmt.Errorf("start %s: %w",lane.Link.Name,err)}
	e.processes=append(e.processes,namedProcess{name:lane.Link.Name,proc:link})
	if err:=waitProcessReady(lane.Link.Name,link);err!=nil{rollback();return err}
	return nil
}

func (e *Executor) StopDynamicLanePlan(lane LanePlan) error {
	if err:=validateDynamicLanePlan(lane);err!=nil{return err}
	wanted:=laneProcessNameSet(lane)
	e.mu.Lock();defer e.mu.Unlock()
	if !e.running{return errors.New("shared Windows runtime is not running")}
	found:=0
	var errs []error
	for i:=len(e.processes)-1;i>=0;i--{
		p:=e.processes[i]
		if !wanted[p.name]{continue}
		found++
		if stopErr:=p.proc.Stop();stopErr!=nil{errs=append(errs,fmt.Errorf("stop %s: %w",p.name,stopErr))}
		e.processes=append(e.processes[:i],e.processes[i+1:]...)
	}
	if found==0{return fmt.Errorf("dynamic transport process group is not running")}
	if found!=3{errs=append(errs,fmt.Errorf("dynamic transport process group incomplete: found=%d want=3",found))}
	return errors.Join(errs...)
}

// StopDynamicLane is the shipping ID=1 compatibility wrapper.
func (e *Executor) StopDynamicLane(laneID int) error {
	fake,dtls,link,err:=normalLaneCommandsForStop(laneID);if err!=nil{return err}
	return e.StopDynamicLanePlan(LanePlan{ID:laneID,Slot:laneID,FakeTCP:fake,DTLS:dtls,Link:link})
}

func normalLaneCommandsForStop(laneID int)(Command,Command,Command,error){
	if laneID!=1{return Command{},Command{},Command{},ErrOverlappingPublicFlow}
	return Command{Name:"faketcp-1"},Command{Name:"dtls-1"},Command{Name:"link-1"},nil
}

func (e *Executor) DynamicLaneIDs() []int {
	e.mu.Lock();defer e.mu.Unlock()
	for _,p:=range e.processes{if p.name=="link-1"{return []int{1}}}
	return nil
}
