package windowsruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const (
	singleFlowBootstrapReadyMarker = "WBD_SINGLE_FLOW_BOOTSTRAP_READY"
	singleFlowBootstrapWait        = 15 * time.Second
)

type RuntimeState string

const (
	RuntimeDisconnected RuntimeState = "disconnected"
	RuntimeConnecting    RuntimeState = "connecting"
	RuntimeConnected     RuntimeState = "connected"
	RuntimeDormant       RuntimeState = "dormant"
	RuntimeDisconnecting RuntimeState = "disconnecting"
)

type UnderlayDiscoverer interface { Discover(Profile) (Underlay, error) }
type RuntimePreflighter interface { Preflight(Profile) error }

type TicketStore interface {
	Clear(path string) error
	Read(path string) (string, error)
}

type Controller struct {
	mu sync.Mutex
	state RuntimeState
	runner Runner
	executor *Executor
	discoverer UnderlayDiscoverer
	tickets TicketStore

	// The fields below are Logical Tunnel control-plane state. They never own or
	// mutate FakeTCP/TCP-like recovery semantics. They survive DORMANT so the one
	// shared TUN/routes and authenticated tunnel identity can wake new lanes.
	profile Profile
	baseUnderlay Underlay
	tunnelConfig logicaltunnel.TunnelConfig
	gameControl string
	lanePlans map[int]LanePlan
	lifecycle *logicaltunnel.LaneLifecycle
}

func NewController(runner Runner,discoverer UnderlayDiscoverer,tickets TicketStore)*Controller{
	if runner==nil{runner=OSRunner{}}
	if discoverer==nil{discoverer=PowerShellUnderlayDiscoverer{}}
	if tickets==nil{tickets=FileTicketStore{}}
	return &Controller{state:RuntimeDisconnected,runner:runner,executor:NewExecutor(runner),discoverer:discoverer,tickets:tickets}
}

func(c *Controller)State()RuntimeState{c.mu.Lock();defer c.mu.Unlock();return c.state}

func startupRecoveryCommands(profile Profile)[]Command{
	bin:=func(name string)string{return filepath.Join(profile.BinDir,name)}
	return []Command{
		{Name:"route-cleanup",Path:"powershell.exe",Args:[]string{"-NoProfile","-ExecutionPolicy","Bypass","-File",bin("windows_tun_route.ps1"),"-Action","Cleanup","-StatePath",profile.RouteState}},
		{Name:"ipv6-cleanup",Path:"powershell.exe",Args:[]string{"-NoProfile","-ExecutionPolicy","Bypass","-File",bin("windows_ipv6_killswitch.ps1"),"-Action","Cleanup"}},
	}
}

func(c *Controller)recoverStaleNetworkState(profile Profile)error{for _,command:=range startupRecoveryCommands(profile){if err:=c.runner.Run(command);err!=nil{return fmt.Errorf("startup %s: %w",command.Name,err)}};return nil}

func decodeAuthenticatedTunnelConfig(raw string)(logicaltunnel.TunnelConfig,error){
	var cfg logicaltunnel.TunnelConfig
	dec:=json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)));dec.DisallowUnknownFields()
	if err:=dec.Decode(&cfg);err!=nil{return cfg,fmt.Errorf("decode authenticated tunnel config: %w",err)}
	if err:=cfg.Validate();err!=nil{return cfg,fmt.Errorf("validate authenticated tunnel config: %w",err)}
	return cfg,nil
}

func(c *Controller)Connect(profile Profile)error{
	profile=profile.normalized();if err:=profile.Validate();err!=nil{return err}
	if c.executor.CleanupPending(){return errors.New("Windows runtime has pending route cleanup; retry Disconnect before Connect")}

	c.mu.Lock();if c.state!=RuntimeDisconnected{state:=c.state;c.mu.Unlock();return fmt.Errorf("Windows runtime cannot connect while %s",state)};c.state=RuntimeConnecting;c.mu.Unlock()
	connected:=false
	defer func(){if connected{return};c.mu.Lock();c.state=RuntimeDisconnected;c.clearRuntimeContextLocked();c.mu.Unlock()}()

	if preflight,ok:=c.discoverer.(RuntimePreflighter);ok{if err:=preflight.Preflight(profile);err!=nil{return fmt.Errorf("Windows runtime dependency preflight: %w",err)}}
	if err:=c.recoverStaleNetworkState(profile);err!=nil{return fmt.Errorf("recover stale Windows network state: %w",err)}

	baseUnderlay,err:=c.discoverer.Discover(profile);if err!=nil{return fmt.Errorf("discover Windows FakeTCP underlay: %w",err)}

	bootstraps:=make([]LaneBootstrap,0,profile.Lanes)
	prestarted:=make(map[int]Process,profile.Lanes)
	owned:=true
	defer func(){
		if !owned{return}
		for laneID:=profile.Lanes;laneID>=1;laneID--{if proc:=prestarted[laneID];proc!=nil{_ = proc.Stop()}}
	}()

	for laneID:=1;laneID<=profile.Lanes;laneID++{
		laneUnderlay:=baseUnderlay;laneUnderlay.SourcePort=nextFakeTCPSourcePort()
		bootstrap,err:=BuildLaneBootstrap(profile,laneUnderlay,laneID);if err!=nil{return fmt.Errorf("build lane %d bootstrap: %w",laneID,err)}
		if err:=c.tickets.Clear(bootstrap.TicketPath);err!=nil{return fmt.Errorf("clear lane %d Reality ticket: %w",laneID,err)}
		if err:=c.tickets.Clear(bootstrap.TunnelConfigPath);err!=nil{return fmt.Errorf("clear lane %d authenticated tunnel config: %w",laneID,err)}

		proc,err:=c.runner.Start(bootstrap.FakeTCP);if err!=nil{return fmt.Errorf("start lane %d same-flow FakeTCP: %w",laneID,err)}
		prestarted[laneID]=proc
		if err:=waitProcessMarker(fmt.Sprintf("lane %d Reality bootstrap",laneID),proc,singleFlowBootstrapReadyMarker,singleFlowBootstrapWait);err!=nil{return err}

		ticket,err:=c.tickets.Read(bootstrap.TicketPath);if err!=nil{return fmt.Errorf("read lane %d Reality ticket after bootstrap readiness: %w",laneID,err)}
		rawTunnelConfig,err:=c.tickets.Read(bootstrap.TunnelConfigPath);if err!=nil{return fmt.Errorf("read lane %d authenticated tunnel config after bootstrap readiness: %w",laneID,err)}
		tunnelConfig,err:=decodeAuthenticatedTunnelConfig(rawTunnelConfig);if err!=nil{return fmt.Errorf("lane %d: %w",laneID,err)}
		bootstrap.Ticket=ticket;bootstrap.TunnelConfig=tunnelConfig
		if laneID>1{if err:=bootstrap.ValidateAuthenticated(&bootstraps[0].TunnelConfig);err!=nil{return err}}
		bootstraps=append(bootstraps,bootstrap)
	}

	multi,err:=BuildMultiLanePlan(profile,bootstraps);if err!=nil{return fmt.Errorf("build Windows multi-lane runtime plan: %w",err)}
	profile.TunnelIPv4=multi.TunnelConfig.Address4

	owned=false
	if err:=c.executor.StartMultiLane(multi,prestarted);err!=nil{return err}
	lifecycle,err:=logicaltunnel.NewLaneLifecycle(profile.Lanes);if err!=nil{_ = c.executor.Stop();return err}
	plans:=make(map[int]LanePlan,len(multi.Lanes))
	for _,lane:=range multi.Lanes{
		if _,err:=lifecycle.AttachInitial(uint8(lane.ID));err!=nil{_ = c.executor.Stop();return err}
		plans[lane.ID]=lane
	}

	c.mu.Lock()
	c.profile=profile;c.baseUnderlay=baseUnderlay;c.tunnelConfig=multi.TunnelConfig;c.gameControl=multi.GameControl;c.lanePlans=plans;c.lifecycle=lifecycle;c.state=RuntimeConnected
	c.mu.Unlock();connected=true;return nil
}

func(c *Controller)Disconnect()error{
	c.mu.Lock()
	switch c.state{
	case RuntimeDisconnected:
		c.mu.Unlock();return c.executor.Stop()
	case RuntimeConnected,RuntimeDormant:
		c.state=RuntimeDisconnecting
	default:
		state:=c.state;c.mu.Unlock();return fmt.Errorf("Windows runtime cannot disconnect while %s",state)
	}
	c.mu.Unlock()
	err:=c.executor.Stop()
	c.mu.Lock();c.state=RuntimeDisconnected;c.clearRuntimeContextLocked();c.mu.Unlock();return err
}

func(c *Controller)clearRuntimeContextLocked(){
	c.profile=Profile{};c.baseUnderlay=Underlay{};c.tunnelConfig=logicaltunnel.TunnelConfig{};c.gameControl="";c.lanePlans=nil;c.lifecycle=nil
}

type FileTicketStore struct{}
func(FileTicketStore)Clear(path string)error{err:=os.Remove(path);if errors.Is(err,os.ErrNotExist){return nil};return err}
func(FileTicketStore)Read(path string)(string,error){deadline:=time.Now().Add(15*time.Second);var last error;for{b,err:=os.ReadFile(path);if err==nil{body:=strings.TrimSpace(string(b));if body!=""{return body,nil};last=errors.New("state file is empty")}else{last=err};if time.Now().After(deadline){return "",fmt.Errorf("state readiness timeout: %w",last)};time.Sleep(25*time.Millisecond)}}

type PowerShellUnderlayDiscoverer struct{}
func(PowerShellUnderlayDiscoverer)Preflight(profile Profile)error{
	profile=profile.normalized();if err:=profile.Validate();err!=nil{return err};if err:=ValidateRoutingAssets(profile);err!=nil{return err}
	script:=filepath.Join(profile.BinDir,"windows_npcap_prepare.ps1");cmd:=exec.Command("powershell.exe","-NoProfile","-ExecutionPolicy","Bypass","-File",script,"-Action","Status");output,err:=cmd.CombinedOutput();if err!=nil{text:=strings.TrimSpace(string(output));if text==""{text=err.Error()};return fmt.Errorf("Npcap runtime is not ready: %s; run %s -Action Install",text,script)};return nil
}
func(PowerShellUnderlayDiscoverer)Discover(profile Profile)(Underlay,error){
	profile=profile.normalized();if err:=profile.Validate();err!=nil{return Underlay{},err};raw,_:=netip.ParseAddrPort(profile.ServerRaw);script:=filepath.Join(profile.BinDir,"windows_faketcp_underlay.ps1");cmd:=exec.Command("powershell.exe","-NoProfile","-ExecutionPolicy","Bypass","-File",script,"-RemoteIPAddress",raw.Addr().String());output,err:=cmd.CombinedOutput();if err!=nil{return Underlay{},fmt.Errorf("%v: %s",err,strings.TrimSpace(string(output)))}
	var jsonLine string;for _,line:=range strings.Split(string(output),"\n"){line=strings.TrimSpace(line);if strings.HasPrefix(line,"{"){jsonLine=line;break}};if jsonLine==""{return Underlay{},fmt.Errorf("underlay discovery returned no JSON: %s",strings.TrimSpace(string(output)))}
	var result struct{SourceIP string `json:"source_ip"`;PacketDevice string `json:"packet_device"`;SourceMAC string `json:"source_mac"`;NextHopMAC string `json:"next_hop_mac"`};if err:=json.Unmarshal([]byte(jsonLine),&result);err!=nil{return Underlay{},fmt.Errorf("decode underlay discovery: %w",err)};underlay:=Underlay{SourceIP:result.SourceIP,PacketDevice:result.PacketDevice,SourceMAC:result.SourceMAC,NextHopMAC:result.NextHopMAC};if err:=underlay.Validate();err!=nil{return Underlay{},err};return underlay,nil
}
