package windowsruntime

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
)

const makeBeforeBreakCandidateSlot = 5

var ErrOverlappingPublicFlow = errors.New("global single-flow product forbids overlapping candidate public transport")

// BuildCandidateLaneBootstrap is retained only as a compatibility surface for
// older callers. ADR-0015 forbids starting a candidate FakeTCP association while
// another public transport may still be active, so shipping code must use
// break-before-make Disconnect/cleanup -> Connect instead.
func BuildCandidateLaneBootstrap(profile Profile, base Underlay, laneID int) (LaneBootstrap, error) {
	return LaneBootstrap{}, ErrOverlappingPublicFlow
}

func BuildCandidateLaneBootstrapSlot(profile Profile, base Underlay, laneID, slot int) (LaneBootstrap, error) {
	return LaneBootstrap{}, ErrOverlappingPublicFlow
}

func BuildAuthenticatedLanePlan(profile Profile, bootstrap LaneBootstrap) (LanePlan,error) {
	return buildLanePlanForSlot(profile,bootstrap,bootstrap.ID,false)
}
func BuildCandidateLanePlan(profile Profile, bootstrap LaneBootstrap) (LanePlan,error){
	return LanePlan{}, ErrOverlappingPublicFlow
}
func BuildCandidateLanePlanSlot(profile Profile, bootstrap LaneBootstrap, slot int)(LanePlan,error){
	return LanePlan{}, ErrOverlappingPublicFlow
}

func buildLanePlanForSlot(profile Profile, bootstrap LaneBootstrap, slot int, candidate bool)(LanePlan,error){
	profile=profile.normalized()
	if candidate{return LanePlan{},ErrOverlappingPublicFlow}
	if err:=profile.Validate();err!=nil{return LanePlan{},err}
	if err:=bootstrap.ValidateAuthenticated(nil);err!=nil{return LanePlan{},err}
	if bootstrap.ID!=1{return LanePlan{},ErrOverlappingPublicFlow}
	if _,err:=transportSlotPort(0,slot);err!=nil{return LanePlan{},err}
	fakePort,err:=transportSlotPort(defaultFakeTCPLocalPort,slot);if err!=nil{return LanePlan{},err}
	dtlsPort,err:=transportSlotPort(defaultDTLSPlainPort,slot);if err!=nil{return LanePlan{},err}
	dtlsPlain,err:=transportSlotLoopback(defaultDTLSPlainPort,slot);if err!=nil{return LanePlan{},err}
	linkListen,err:=transportSlotLoopback(defaultLinkListenPort,slot);if err!=nil{return LanePlan{},err}
	bin:=func(name string)string{return filepath.Join(profile.BinDir,name)}
	suffix:=strconv.Itoa(bootstrap.ID)
	fake:=bootstrap.FakeTCP
	return LanePlan{ID:bootstrap.ID,Slot:slot,FakeTCP:fake,
		DTLS:Command{Name:"dtls-"+suffix,Path:bin("wbd_dtls_shim.exe"),Args:[]string{"client",strconv.Itoa(dtlsPort),"127.0.0.1",strconv.Itoa(fakePort),"none","none"}},
		Link:Command{Name:"link-"+suffix,Path:bin("wbd-link-proxy.exe"),Args:[]string{"-mode","client","-listen",linkListen,"-dtls",dtlsPlain,"-fec",profile.FEC,"-mtu",strconv.Itoa(profile.MTU),"-lanes","1","-demo-reality-ticket",strings.TrimSpace(bootstrap.Ticket)}},
	},nil
}

// These helpers remain for decoding historical research plans only. They do not
// authorize a candidate public association in the shipping runtime.
func NextReplacementSlot(current LanePlan) int {
	if current.Slot==makeBeforeBreakCandidateSlot{return current.ID}
	return makeBeforeBreakCandidateSlot
}

func LaneGameTarget(plan LanePlan)(string,error){
	if plan.ID!=1{return "",ErrOverlappingPublicFlow}
	if plan.Slot==0{plan.Slot=plan.ID}
	return transportSlotLoopback(defaultLinkListenPort,plan.Slot)
}

// Keep imports used by historical generated diffs stable while candidate
// bootstrap construction is disabled. They are referenced by the normal plan
// builder above through shared types and path construction.
var _ = netip.AddrPort{}
var _ = fmt.Sprintf
