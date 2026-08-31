package windowsruntime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Process interface { Stop() error }

type readyProcess interface { WaitReady(marker string, timeout time.Duration) error }

type Runner interface {
	Run(Command) error
	Start(Command) (Process, error)
}

type readinessSpec struct { marker string; timeout time.Duration }

func commandReadiness(name string) (readinessSpec, bool) {
	switch {
	case name == "faketcp" || strings.HasPrefix(name, "faketcp-"):
		return readinessSpec{marker:"READY role=client",timeout:25*time.Second},true
	case name == "dtls" || strings.HasPrefix(name, "dtls-"):
		return readinessSpec{marker:"READY role=client",timeout:25*time.Second},true
	case name == "link" || strings.HasPrefix(name, "link-"):
		return readinessSpec{marker:"WBD_LINK_READY role=client",timeout:12*time.Second},true
	case name == "game":
		return readinessSpec{marker:"WBD_GAME_LANE_CLIENT_READY",timeout:10*time.Second},true
	case name == "tun":
		return readinessSpec{marker:"WBD_TUN_READY mode=client",timeout:10*time.Second},true
	default:
		return readinessSpec{},false
	}
}

type Executor struct {
	mu sync.Mutex
	runner Runner
	plan Plan
	processes []namedProcess
	running bool
	cleanupPending bool
}

type namedProcess struct { name string; proc Process }

func NewExecutor(runner Runner) *Executor { if runner==nil{runner=OSRunner{}};return &Executor{runner:runner} }

func (e *Executor) Start(plan Plan) error { e.mu.Lock();defer e.mu.Unlock();return e.startLocked(plan,nil) }

func (e *Executor) StartAfterFakeTCP(plan Plan, fake Process) error {
	if fake==nil{return errors.New("prestarted FakeTCP process is required")}
	e.mu.Lock();defer e.mu.Unlock();return e.startLocked(plan,fake)
}

func (e *Executor) startLocked(plan Plan, prestartedFake Process) error {
	if err:=e.ensureStartableLocked();err!=nil{return err}
	commands:=plan.ProcessSequence()
	if prestartedFake!=nil{
		e.processes=append(e.processes,namedProcess{name:plan.FakeTCP.Name,proc:prestartedFake})
		if err:=waitProcessReady(plan.FakeTCP.Name,prestartedFake);err!=nil{e.rollbackProcessesLocked();return err}
		if len(commands)!=0{commands=commands[1:]}
	}
	for _,command:=range commands{
		proc,err:=e.runner.Start(command);if err!=nil{e.rollbackProcessesLocked();return fmt.Errorf("start %s: %w",command.Name,err)}
		e.processes=append(e.processes,namedProcess{name:command.Name,proc:proc})
		if err:=waitProcessReady(command.Name,proc);err!=nil{e.rollbackProcessesLocked();return err}
	}
	if err:=e.applyNetworkLocked(plan);err!=nil{return err}
	e.plan=plan;e.running=true;return nil
}

// StartMultiLane takes ownership of already-started FakeTCP lane processes after
// each one has completed its same-association Reality-like bootstrap. It then
// brings up lane-local DTLS and LINK strictly bottom-up. Only after every lane
// is healthy does it start the Game/race aggregator, the one Wintun transport,
// IPv6 fail-closed state and capture routes. A failure before route mutation
// rolls back every lane; no transport-wire semantics are changed here.
func (e *Executor) StartMultiLane(plan MultiLanePlan, prestartedFake map[int]Process) error {
	e.mu.Lock();defer e.mu.Unlock()
	if err:=e.ensureStartableLocked();err!=nil{return err}
	if len(plan.Lanes)==0{return errors.New("multi-lane plan requires at least one lane")}
	if len(prestartedFake)!=len(plan.Lanes){return fmt.Errorf("prestarted FakeTCP lanes=%d want=%d",len(prestartedFake),len(plan.Lanes))}

	for _,lane:=range plan.Lanes{
		fake:=prestartedFake[lane.ID]
		if fake==nil{e.rollbackProcessesLocked();return fmt.Errorf("prestarted FakeTCP lane %d is missing",lane.ID)}
		e.processes=append(e.processes,namedProcess{name:lane.FakeTCP.Name,proc:fake})
		if err:=waitProcessReady(lane.FakeTCP.Name,fake);err!=nil{e.rollbackProcessesLocked();return err}

		dtls,err:=e.runner.Start(lane.DTLS);if err!=nil{e.rollbackProcessesLocked();return fmt.Errorf("start %s: %w",lane.DTLS.Name,err)}
		e.processes=append(e.processes,namedProcess{name:lane.DTLS.Name,proc:dtls})
		if err:=waitProcessReady(lane.DTLS.Name,dtls);err!=nil{e.rollbackProcessesLocked();return err}

		link,err:=e.runner.Start(lane.Link);if err!=nil{e.rollbackProcessesLocked();return fmt.Errorf("start %s: %w",lane.Link.Name,err)}
		e.processes=append(e.processes,namedProcess{name:lane.Link.Name,proc:link})
		if err:=waitProcessReady(lane.Link.Name,link);err!=nil{e.rollbackProcessesLocked();return err}
	}

	game,err:=e.runner.Start(plan.Game);if err!=nil{e.rollbackProcessesLocked();return fmt.Errorf("start game: %w",err)}
	e.processes=append(e.processes,namedProcess{name:plan.Game.Name,proc:game})
	if err:=waitProcessReady(plan.Game.Name,game);err!=nil{e.rollbackProcessesLocked();return err}

	tun,err:=e.runner.Start(plan.TUN);if err!=nil{e.rollbackProcessesLocked();return fmt.Errorf("start tun: %w",err)}
	e.processes=append(e.processes,namedProcess{name:plan.TUN.Name,proc:tun})
	if err:=waitProcessReady(plan.TUN.Name,tun);err!=nil{e.rollbackProcessesLocked();return err}

	cleanupPlan:=Plan{TUN:plan.TUN,IPv6Apply:plan.IPv6Apply,RouteApply:plan.RouteApply,RouteCleanup:plan.RouteCleanup,IPv6Cleanup:plan.IPv6Cleanup}
	if err:=e.applyNetworkLocked(cleanupPlan);err!=nil{return err}
	e.plan=cleanupPlan;e.running=true;return nil
}

func (e *Executor) ensureStartableLocked() error {
	if e.cleanupPending{return errors.New("Windows runtime has pending network cleanup")}
	if e.running||len(e.processes)!=0{return errors.New("Windows runtime is already running")}
	return nil
}

func (e *Executor) applyNetworkLocked(plan Plan) error {
	if err:=e.runner.Run(plan.IPv6Apply);err!=nil{
		cleanupErr:=e.runner.Run(plan.IPv6Cleanup);e.rollbackProcessesLocked()
		if cleanupErr!=nil{e.plan=plan;e.cleanupPending=true;return fmt.Errorf("apply IPv6 kill switch: %w (cleanup: %v)",err,cleanupErr)}
		return fmt.Errorf("apply IPv6 kill switch: %w",err)
	}
	if err:=e.runner.Run(plan.RouteApply);err!=nil{
		routeErr:=e.runner.Run(plan.RouteCleanup);ipv6Err:=e.runner.Run(plan.IPv6Cleanup);e.rollbackProcessesLocked()
		if routeErr!=nil||ipv6Err!=nil{e.plan=plan;e.cleanupPending=true;return fmt.Errorf("apply capture routes: %w (route cleanup: %v; IPv6 cleanup: %v)",err,routeErr,ipv6Err)}
		return fmt.Errorf("apply capture routes: %w",err)
	}
	return nil
}

func waitProcessMarker(label string,proc Process,marker string,timeout time.Duration) error {
	if strings.TrimSpace(marker)==""||timeout<=0{return fmt.Errorf("wait %s: invalid readiness contract",label)}
	rp,ok:=proc.(readyProcess);if !ok{return fmt.Errorf("wait %s: process runner does not expose readiness",label)}
	if err:=rp.WaitReady(marker,timeout);err!=nil{return fmt.Errorf("wait %s: %w",label,err)}
	return nil
}

func waitProcessReady(name string,proc Process) error { spec,ok:=commandReadiness(name);if !ok{return nil};return waitProcessMarker(name+" ready",proc,spec.marker,spec.timeout) }

func (e *Executor) Stop() error {
	e.mu.Lock();defer e.mu.Unlock()
	if !e.running&&len(e.processes)==0{
		if !e.cleanupPending{return nil}
		var retryErrs []error
		if err:=e.runner.Run(e.plan.RouteCleanup);err!=nil{retryErrs=append(retryErrs,fmt.Errorf("cleanup capture routes: %w",err))}
		if err:=e.runner.Run(e.plan.IPv6Cleanup);err!=nil{retryErrs=append(retryErrs,fmt.Errorf("cleanup IPv6 kill switch: %w",err))}
		if len(retryErrs)!=0{return errors.Join(retryErrs...)}
		e.plan=Plan{};e.cleanupPending=false;return nil
	}
	var errs []error
	routeCleanupErr:=e.runner.Run(e.plan.RouteCleanup);if routeCleanupErr!=nil{errs=append(errs,fmt.Errorf("cleanup capture routes: %w",routeCleanupErr))}
	ipv6CleanupErr:=e.runner.Run(e.plan.IPv6Cleanup);if ipv6CleanupErr!=nil{errs=append(errs,fmt.Errorf("cleanup IPv6 kill switch: %w",ipv6CleanupErr))}
	for i:=len(e.processes)-1;i>=0;i--{if err:=e.processes[i].proc.Stop();err!=nil{errs=append(errs,fmt.Errorf("stop %s: %w",e.processes[i].name,err))}}
	e.processes=nil;e.running=false
	if routeCleanupErr!=nil||ipv6CleanupErr!=nil{e.cleanupPending=true}else{e.plan=Plan{};e.cleanupPending=false}
	return errors.Join(errs...)
}

func (e *Executor) Running() bool { e.mu.Lock();defer e.mu.Unlock();return e.running }
func (e *Executor) CleanupPending() bool { e.mu.Lock();defer e.mu.Unlock();return e.cleanupPending }
func (e *Executor) rollbackProcessesLocked(){for i:=len(e.processes)-1;i>=0;i--{_ = e.processes[i].proc.Stop()};e.processes=nil;e.running=false;if !e.cleanupPending{e.plan=Plan{}}}

type OSRunner struct{}
func (OSRunner) Run(command Command) error {cmd:=exec.Command(command.Path,command.Args...);cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()}
func (OSRunner) Start(command Command)(Process,error){cmd:=exec.Command(command.Path,command.Args...);out:=newProcessOutput();stdout:=&readyLineWriter{dst:os.Stdout,out:out};stderr:=&readyLineWriter{dst:os.Stderr,out:out};cmd.Stdout=stdout;cmd.Stderr=stderr;if err:=cmd.Start();err!=nil{return nil,err};p:=&osProcess{cmd:cmd,out:out,stdout:stdout,stderr:stderr,done:make(chan struct{})};go p.wait();return p,nil}

type processOutput struct{mu sync.Mutex;lines []string;notify chan struct{}}
func newProcessOutput()*processOutput{return &processOutput{notify:make(chan struct{},1)}}
func(o *processOutput)observe(line string){line=strings.TrimSpace(line);if line==""{return};o.mu.Lock();if len(o.lines)>=256{copy(o.lines,o.lines[len(o.lines)-128:]);o.lines=o.lines[:128]};o.lines=append(o.lines,line);o.mu.Unlock();select{case o.notify<-struct{}{}:default:}}
func(o *processOutput)contains(marker string)bool{o.mu.Lock();defer o.mu.Unlock();for _,line:=range o.lines{if strings.Contains(line,marker){return true}};return false}

type readyLineWriter struct{mu sync.Mutex;dst *os.File;out *processOutput;buf []byte}
func(w *readyLineWriter)Write(p []byte)(int,error){w.mu.Lock();defer w.mu.Unlock();if w.dst!=nil{_,_=w.dst.Write(p)};w.buf=append(w.buf,p...);for{i:=bytes.IndexByte(w.buf,'\n');if i<0{break};w.out.observe(string(w.buf[:i]));w.buf=append(w.buf[:0],w.buf[i+1:]...)};return len(p),nil}
func(w *readyLineWriter)flush(){w.mu.Lock();if len(w.buf)!=0{w.out.observe(string(w.buf));w.buf=w.buf[:0]};w.mu.Unlock()}

type osProcess struct{cmd *exec.Cmd;out *processOutput;stdout *readyLineWriter;stderr *readyLineWriter;done chan struct{};mu sync.Mutex;exited bool;err error}
func(p *osProcess)wait(){err:=p.cmd.Wait();p.stdout.flush();p.stderr.flush();p.mu.Lock();p.err=err;p.exited=true;p.mu.Unlock();close(p.done);select{case p.out.notify<-struct{}{}:default:}}
func(p *osProcess)WaitReady(marker string,timeout time.Duration)error{deadline:=time.NewTimer(timeout);defer deadline.Stop();for{if p.out.contains(marker){return nil};p.mu.Lock();exited,err:=p.exited,p.err;p.mu.Unlock();if exited{if err==nil{return fmt.Errorf("process exited before readiness marker %q",marker)};return fmt.Errorf("process exited before readiness marker %q: %w",marker,err)};select{case<-p.out.notify:case<-p.done:case<-deadline.C:return fmt.Errorf("timeout waiting for marker %q",marker)}}}
func(p *osProcess)Stop()error{p.mu.Lock();if p.exited{p.mu.Unlock();return nil};proc:=p.cmd.Process;p.mu.Unlock();if proc==nil{return nil};err:=proc.Kill();if err==nil{return nil};if isBenignProcessStopError(err){return nil};return err}
