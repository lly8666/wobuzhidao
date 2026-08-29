package windowsruntime

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type controllerRunner struct {
	events     []string
	ticketPath string
	fail       string
	failReady  string
	linkTicket string
}

func (r *controllerRunner) Run(command Command) error {
	r.events = append(r.events, "run:"+command.Name)
	if r.fail == command.Name { return errors.New("injected failure") }
	return nil
}
func (r *controllerRunner) Start(command Command) (Process, error) {
	r.events = append(r.events, "start:"+command.Name)
	if r.fail == command.Name { return nil, errors.New("injected failure") }
	if command.Name == "link" {
		for i := 0; i+1 < len(command.Args); i++ {
			if command.Args[i] == "-demo-reality-ticket" { r.linkTicket = command.Args[i+1] }
		}
	}
	return &controllerProcess{runner:r,name:command.Name}, nil
}
type controllerProcess struct { runner *controllerRunner; name string }
func (p *controllerProcess) Stop() error { p.runner.events=append(p.runner.events,"stop:"+p.name); return nil }
func (p *controllerProcess) WaitReady(marker string, timeout time.Duration) error {
	p.runner.events=append(p.runner.events,"ready:"+p.name)
	if p.runner.failReady == p.name { return errors.New("injected readiness failure") }
	if p.name == "faketcp" && p.runner.ticketPath != "" {
		ticket := strings.Repeat("ab",32)
		if err := os.WriteFile(p.runner.ticketPath, []byte(ticket+"\n"), 0o600); err != nil { return err }
	}
	if marker == "" || timeout <= 0 { return errors.New("invalid readiness contract") }
	return nil
}

type controllerTicketStore struct { runner *controllerRunner; clearErr error; readCalled bool }
func (s *controllerTicketStore) Clear(path string) error {
	s.runner.events=append(s.runner.events,"ticket:clear")
	if s.clearErr != nil { return s.clearErr }
	if err:=os.Remove(path); err!=nil && !errors.Is(err,os.ErrNotExist){return err}
	return nil
}
func (s *controllerTicketStore) Read(string)(string,error){s.readCalled=true;return "",errors.New("V3 Controller must not pre-read ticket")}

type controllerDiscoverer struct { runner *controllerRunner; underlay Underlay; err,preflightErr error; preflight bool }
func (d *controllerDiscoverer) Discover(Profile)(Underlay,error){d.runner.events=append(d.runner.events,"discover:underlay");if d.err!=nil{return Underlay{},d.err};return d.underlay,nil}
func (d *controllerDiscoverer) Preflight(Profile)error{if d.preflight{d.runner.events=append(d.runner.events,"preflight:dependencies")};return d.preflightErr}

func v3ControllerFixture(t *testing.T, preflight bool)(*Controller,*controllerRunner,*controllerTicketStore,Profile){
	t.Helper()
	p:=testProfile();p.TicketPath=t.TempDir()+"/ticket"
	r:=&controllerRunner{ticketPath:p.TicketPath}
	d:=&controllerDiscoverer{runner:r,underlay:testUnderlay(),preflight:preflight}
	ts:=&controllerTicketStore{runner:r}
	return NewController(r,d,ts),r,ts,p
}

func TestControllerConnectDisconnectUsesOnePublicFlowComposition(t *testing.T){
	c,r,ts,p:=v3ControllerFixture(t,false)
	if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	if ts.readCalled{t.Fatal("V3 Controller pre-read ticket before FakeTCP admission")}
	if r.linkTicket!=strings.Repeat("ab",32){t.Fatalf("LINK received ticket %q",r.linkTicket)}
	if got:=c.State();got!=RuntimeConnected{t.Fatalf("state after Connect=%s",got)}
	if err:=c.Disconnect();err!=nil{t.Fatal(err)}
	want:=[]string{"ticket:clear","discover:underlay","start:faketcp","ready:faketcp","start:dtls","ready:dtls","start:link","ready:link","start:tun","ready:tun","run:ipv6-apply","run:route-apply","run:route-cleanup","run:ipv6-cleanup","stop:tun","stop:link","stop:dtls","stop:faketcp"}
	if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
	for _,event:=range r.events{if strings.Contains(event,"reality-bootstrap"){t.Fatalf("separate Reality process observed: %v",r.events)}}
}

func TestControllerDependencyPreflightFailsBeforePublicFlow(t *testing.T){
	c,r,_,p:=v3ControllerFixture(t,true)
	d:=c.discoverer.(*controllerDiscoverer);d.preflightErr=errors.New("Npcap missing")
	if err:=c.Connect(p);err==nil{t.Fatal("expected preflight failure")}
	if want:=[]string{"preflight:dependencies"};!reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
}

func TestControllerUnderlayFailureNeverStartsPublicFlow(t *testing.T){
	c,r,_,p:=v3ControllerFixture(t,false)
	c.discoverer.(*controllerDiscoverer).err=errors.New("no neighbor")
	if err:=c.Connect(p);err==nil{t.Fatal("expected underlay failure")}
	want:=[]string{"ticket:clear","discover:underlay"}
	if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
}

func TestControllerFakeTCPAdmissionFailureStopsBeforeDTLSOrRoutes(t *testing.T){
	c,r,_,p:=v3ControllerFixture(t,false);r.failReady="faketcp"
	if err:=c.Connect(p);err==nil{t.Fatal("expected FakeTCP admission failure")}
	want:=[]string{"ticket:clear","discover:underlay","start:faketcp","ready:faketcp","stop:faketcp"}
	if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
}

func TestControllerRejectsSecondConnectWithoutTouchingRuntime(t *testing.T){
	c,r,_,p:=v3ControllerFixture(t,false);if err:=c.Connect(p);err!=nil{t.Fatal(err)}
	before:=append([]string(nil),r.events...)
	if err:=c.Connect(p);err==nil{t.Fatal("second Connect must fail")}
	if !reflect.DeepEqual(r.events,before){t.Fatalf("second Connect changed runtime events")}
	if err:=c.Disconnect();err!=nil{t.Fatal(err)}
}

func TestControllerTicketClearFailureStopsBeforePublicFlow(t *testing.T){
	c,r,ts,p:=v3ControllerFixture(t,false);ts.clearErr=errors.New("denied")
	if err:=c.Connect(p);err==nil{t.Fatal("expected ticket clear failure")}
	if want:=[]string{"ticket:clear"};!reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
}
