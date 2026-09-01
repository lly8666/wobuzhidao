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

// BuildCandidateLaneBootstrap creates a fresh same-flow public association for
// an existing Logical LaneID while using private slot 5 for local UDP/DTLS/LINK
// ports. The candidate is not part of Game/race until it is fully healthy.
func BuildCandidateLaneBootstrap(profile Profile, base Underlay, laneID int) (LaneBootstrap, error) {
	profile=profile.normalized()
	if err:=profile.Validate();err!=nil{return LaneBootstrap{},err}
	if _,err:=lanePort(0,laneID);err!=nil{return LaneBootstrap{},err}
	if err:=base.Validate();err!=nil{return LaneBootstrap{},err}
	if base.SourcePort==0{return LaneBootstrap{},errors.New("candidate lane requires an assigned dynamic FakeTCP source port")}
	localUDP,err:=transportSlotLoopback(defaultFakeTCPLocalPort,makeBeforeBreakCandidateSlot);if err!=nil{return LaneBootstrap{},err}
	raw,_:=netip.ParseAddrPort(profile.ServerRaw)
	ticketPath:=fmt.Sprintf("%s.lane%d.candidate",profile.TicketPath,laneID)
	configPath:=fmt.Sprintf("%s.lane%d.candidate",profile.TunnelConfigPath,laneID)
	args:=[]string{
		"client","--local-udp",localUDP,
		"--source",netip.AddrPortFrom(netip.MustParseAddr(base.SourceIP),base.SourcePort).String(),
		"--remote",raw.String(),"--shadow-recovery","legacy",
		"--packet-device",base.PacketDevice,"--source-mac",base.SourceMAC,"--next-hop-mac",base.NextHopMAC,
		"--reality-server-name",profile.ServerName,"--reality-route-key",profile.RouteKey,
		"--reality-username",profile.Username,"--reality-password",profile.Password,
		"--reality-ticket-out",ticketPath,"--reality-installation-id",profile.InstallationID,
		"--reality-tunnel-config-out",configPath,"--reality-verify-server="+strconv.FormatBool(profile.VerifyServer),
	}
	return LaneBootstrap{ID:laneID,Underlay:base,FakeTCP:Command{Name:fmt.Sprintf("faketcp-%d-candidate",laneID),Path:filepath.Join(profile.BinDir,"wbd-faketcp.exe"),Args:args},TicketPath:ticketPath,TunnelConfigPath:configPath},nil
}

func BuildCandidateLanePlan(profile Profile, bootstrap LaneBootstrap) (LanePlan,error){
	profile=profile.normalized()
	if err:=bootstrap.ValidateAuthenticated(nil);err!=nil{return LanePlan{},err}
	fakePort,err:=transportSlotPort(defaultFakeTCPLocalPort,makeBeforeBreakCandidateSlot);if err!=nil{return LanePlan{},err}
	dtlsPort,err:=transportSlotPort(defaultDTLSPlainPort,makeBeforeBreakCandidateSlot);if err!=nil{return LanePlan{},err}
	dtlsPlain,err:=transportSlotLoopback(defaultDTLSPlainPort,makeBeforeBreakCandidateSlot);if err!=nil{return LanePlan{},err}
	linkListen,err:=transportSlotLoopback(defaultLinkListenPort,makeBeforeBreakCandidateSlot);if err!=nil{return LanePlan{},err}
	bin:=func(name string)string{return filepath.Join(profile.BinDir,name)}
	suffix:=fmt.Sprintf("%d-candidate",bootstrap.ID)
	return LanePlan{ID:bootstrap.ID,Slot:makeBeforeBreakCandidateSlot,FakeTCP:bootstrap.FakeTCP,
		DTLS:Command{Name:"dtls-"+suffix,Path:bin("wbd_dtls_shim.exe"),Args:[]string{"client",strconv.Itoa(dtlsPort),"127.0.0.1",strconv.Itoa(fakePort),"none","none"}},
		Link:Command{Name:"link-"+suffix,Path:bin("wbd-link-proxy.exe"),Args:[]string{"-mode","client","-listen",linkListen,"-dtls",dtlsPlain,"-fec",profile.FEC,"-mtu",strconv.Itoa(profile.MTU),"-lanes","1","-demo-reality-ticket",strings.TrimSpace(bootstrap.Ticket)}},
	},nil
}

func LaneGameTarget(plan LanePlan)(string,error){
	if plan.ID<1||plan.ID>4{return "",errors.New("logical lane id must be 1..4")}
	if plan.Slot==0{plan.Slot=plan.ID}
	return transportSlotLoopback(defaultLinkListenPort,plan.Slot)
}
