package windowsruntime

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordingTicketStore struct { runner *recordingRunner; ticket string; clearErr,errorRead error }
func (s *recordingTicketStore) Clear(string) error { s.runner.events=append(s.runner.events,"ticket:clear");return s.clearErr }
func (s *recordingTicketStore) Read(string)(string,error){s.runner.events=append(s.runner.events,"ticket:read");if s.errorRead!=nil{return "",s.errorRead};return s.ticket,nil}

type recordingUnderlayDiscoverer struct { runner *recordingRunner; underlay Underlay; err error }
func (d *recordingUnderlayDiscoverer) Discover(Profile)(Underlay,error){d.runner.events=append(d.runner.events,"discover:underlay");if d.err!=nil{return Underlay{},d.err};return d.underlay,nil}
type recordingPreflightDiscoverer struct { *recordingUnderlayDiscoverer; preflightErr error }
func (d *recordingPreflightDiscoverer) Preflight(Profile)error{d.runner.events=append(d.runner.events,"preflight:dependencies");return d.preflightErr}

func testController(r *recordingRunner)*Controller{return NewController(r,&recordingUnderlayDiscoverer{runner:r,underlay:testUnderlay()},&recordingTicketStore{runner:r,ticket:strings.Repeat("ab",32)})}

func TestControllerConnectDisconnectUsesOnePublicFakeTCPFlow(t *testing.T){
	r:=&recordingRunner{};c:=testController(r);if err:=c.Connect(testProfile());err!=nil{t.Fatal(err)};if got:=c.State();got!=RuntimeConnected{t.Fatalf("state after Connect = %s",got)};if err:=c.Disconnect();err!=nil{t.Fatal(err)};if got:=c.State();got!=RuntimeDisconnected{t.Fatalf("state after Disconnect = %s",got)}
	want:=[]string{"ticket:clear","discover:underlay","start:faketcp","ticket:read","ready:faketcp","start:dtls","ready:dtls","start:link","ready:link","start:tun","ready:tun","run:ipv6-apply","run:route-apply","run:route-cleanup","run:ipv6-cleanup","stop:tun","stop:link","stop:dtls","stop:faketcp"}
	if !reflect.DeepEqual(r.events,want){t.Fatalf("controller events = %v want %v",r.events,want)}
	for _,e:=range r.events{if e=="run:reality-bootstrap"||e=="start:reality-bootstrap"{t.Fatalf("separate public Reality bootstrap reintroduced: %v",r.events)}}
}

func TestControllerDependencyPreflightFailsBeforePublicFlow(t *testing.T){
	r:=&recordingRunner{};d:=&recordingPreflightDiscoverer{recordingUnderlayDiscoverer:&recordingUnderlayDiscoverer{runner:r,underlay:testUnderlay()},preflightErr:errors.New("Npcap missing")};c:=NewController(r,d,&recordingTicketStore{runner:r,ticket:strings.Repeat("ef",32)});if err:=c.Connect(testProfile());err==nil{t.Fatal("expected dependency preflight failure")};want:=[]string{"preflight:dependencies"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)};if got:=c.State();got!=RuntimeDisconnected{t.Fatalf("failed preflight state=%s",got)}
}

func TestControllerFakeTCPStartFailureNeverStartsDTLSOrCapture(t *testing.T){
	r:=&recordingRunner{fail:"faketcp"};c:=testController(r);if err:=c.Connect(testProfile());err==nil{t.Fatal("expected FakeTCP failure")};want:=[]string{"ticket:clear","discover:underlay","start:faketcp"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)};if got:=c.State();got!=RuntimeDisconnected{t.Fatalf("failed Connect state=%s",got)}
}

func TestControllerTicketFailureStopsOnlyPublicFlow(t *testing.T){
	r:=&recordingRunner{};tickets:=&recordingTicketStore{runner:r,ticket:strings.Repeat("cd",32),errorRead:errors.New("TLS auth failed")};c:=NewController(r,&recordingUnderlayDiscoverer{runner:r,underlay:testUnderlay()},tickets);if err:=c.Connect(testProfile());err==nil{t.Fatal("expected ticket failure")};want:=[]string{"ticket:clear","discover:underlay","start:faketcp","ticket:read","stop:faketcp"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
}

func TestControllerUnderlayFailureNeverStartsPublicFlow(t *testing.T){
	r:=&recordingRunner{};tickets:=&recordingTicketStore{runner:r,ticket:strings.Repeat("cd",32)};d:=&recordingUnderlayDiscoverer{runner:r,err:errors.New("no neighbor")};c:=NewController(r,d,tickets);if err:=c.Connect(testProfile());err==nil{t.Fatal("expected underlay failure")};want:=[]string{"ticket:clear","discover:underlay"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}
}

func TestControllerRejectsSecondConnectWithoutTouchingRuntime(t *testing.T){r:=&recordingRunner{};c:=testController(r);if err:=c.Connect(testProfile());err!=nil{t.Fatal(err)};before:=append([]string(nil),r.events...);if err:=c.Connect(testProfile());err==nil{t.Fatal("second Connect must fail while connected")};if !reflect.DeepEqual(r.events,before){t.Fatalf("second Connect changed runtime events")};if err:=c.Disconnect();err!=nil{t.Fatal(err)}}
func TestControllerTicketClearFailureStopsBeforePublicFlow(t *testing.T){r:=&recordingRunner{};c:=NewController(r,&recordingUnderlayDiscoverer{runner:r,underlay:testUnderlay()},&recordingTicketStore{runner:r,clearErr:errors.New("denied")});if err:=c.Connect(testProfile());err==nil{t.Fatal("expected ticket clear failure")};want:=[]string{"ticket:clear"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}}
func TestControllerDisconnectedDisconnectRetriesPendingNetworkCleanup(t *testing.T){r:=&recordingRunner{};c:=testController(r);if err:=c.Connect(testProfile());err!=nil{t.Fatal(err)};r.failOnce="route-cleanup";if err:=c.Disconnect();err==nil{t.Fatal("expected first cleanup failure")};if got:=c.State();got!=RuntimeDisconnected{t.Fatalf("state after failed cleanup=%s",got)};if err:=c.Connect(testProfile());err==nil{t.Fatal("Connect must remain blocked until pending cleanup succeeds")};if err:=c.Disconnect();err!=nil{t.Fatalf("disconnected Disconnect must retry cleanup: %v",err)};if err:=c.Connect(testProfile());err!=nil{t.Fatalf("Connect after cleanup retry: %v",err)};if err:=c.Disconnect();err!=nil{t.Fatal(err)}}
