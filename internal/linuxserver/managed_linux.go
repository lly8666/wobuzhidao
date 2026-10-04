//go:build linux

package linuxserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Persistent state here describes host network changes, never client IP leases.
type networkJournal struct {
	Version int
	BootID string
	Plan NetworkPlan
	Backend FirewallBackend
	Saved map[string]string
	Forward [3]string
	OwnsForward bool
	ListenIP string
	Port uint16
}

type ManagedRuntime struct {
	*Runtime
	lock *os.File
	state string
	ip string
	port uint16
}

func journalWrite(path string,j networkJournal) error {
	f,err:=os.CreateTemp(filepath.Dir(path),".network-*");if err!=nil{return err};name:=f.Name();defer os.Remove(name)
	if err=f.Chmod(0600);err==nil{err=json.NewEncoder(f).Encode(j)};if err==nil{err=f.Sync()};closeErr:=f.Close();if err==nil{err=closeErr};if err!=nil{return err};return os.Rename(name,path)
}

func lockNetwork(path string)(*os.File,error){
	if !filepath.IsAbs(path){return nil,errors.New("managed state must use an absolute path")}
	if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return nil,err}
	f,err:=os.OpenFile(path+".lock",os.O_CREATE|os.O_RDWR,0600);if err!=nil{return nil,err}
	if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{f.Close();return nil,errors.New("another WBD server owns this network state")};return f,nil
}

func rstArgs(ip string,port uint16,action string)[]string{return []string{"-w",action,"OUTPUT","-p","tcp","-s",ip,"--sport",strconv.Itoa(int(port)),"--tcp-flags","RST","RST","-m","comment","--comment","wbd-server-rst","-j","DROP"}}

func rstChange(j networkJournal,add bool)error{
	if j.Port==0{return nil}
	if j.Backend==FirewallIPTables{
		for runCommandQuiet("iptables",rstArgs(j.ListenIP,j.Port,"-C")...)==nil{if err:=runCommand("iptables",rstArgs(j.ListenIP,j.Port,"-D")...);err!=nil{return err}}
		if add{return runCommand("iptables",rstArgs(j.ListenIP,j.Port,"-I")...)};return nil
	}
	if !add{return nil} // owned nft table deletion also removes its output chain
	if err:=runNFTScript("add chain inet "+nftOwnedTable+" output { type filter hook output priority -150; policy accept; }");err!=nil{return err}
	return runCommand("nft","add","rule","inet",nftOwnedTable,"output","ip","saddr",j.ListenIP,"tcp","sport",strconv.Itoa(int(j.Port)),"tcp","flags","&","rst","==","rst","drop","comment","wbd-server-rst")
}

func recoverJournal(path string)error{
	raw,err:=os.ReadFile(path);if errors.Is(err,os.ErrNotExist){return nil};if err!=nil{return err};if len(raw)>1<<20{return errors.New("network journal too large")}
	var j networkJournal;if json.Unmarshal(raw,&j)!=nil || j.Version!=1{return errors.New("invalid network journal")}
	if _,err:=BuildNetworkPlan(j.Plan.TUNName,j.Plan.LeasePrefix,j.Plan.MTU,j.Plan.Firewall.Backend,j.Plan.Firewall.NFTForward);err!=nil{return err}
	if j.Backend!=FirewallNFT && j.Backend!=FirewallIPTables{return ErrNetworkPlan}
	if runCommandQuiet("ip","link","show","dev",j.Plan.TUNName)==nil{return errors.New("journal TUN still exists; refusing recovery while another owner may hold it")}
	if j.Port!=0{ip,err:=netip.ParseAddr(j.ListenIP);if err!=nil || !ip.Is4(){return errors.New("invalid journal listener")}}
	r:=&Runtime{plan:j.Plan,backend:j.Backend,savedSysctls:map[string]string{},nftForward:j.Forward,nftOwnsForward:j.OwnsForward}
	// Restore a saved global value only if it still equals our installed value.
	// Per-TUN sysctls disappear with a crashed nonpersistent TUN.
	boot,_:=os.ReadFile("/proc/sys/kernel/random/boot_id")
	for _,change:=range j.Plan.Sysctls{if j.BootID!=strings.TrimSpace(string(boot)){continue};current,err:=readSysctl(change.Key);if err!=nil{if errors.Is(err,os.ErrNotExist){continue};return err};if current==change.Value{if old,ok:=j.Saved[change.Key];ok{r.savedSysctls[change.Key]=old}}}
	if err:=errors.Join(rstChange(j,false),r.Close());err!=nil{return err};return os.Remove(path)
}

func RecoverManagedNetwork(path string)error{lock,err:=lockNetwork(path);if err!=nil{return err};defer lock.Close();return recoverJournal(path)}

func OpenManagedRuntime(plan NetworkPlan,path string,ip netip.Addr,port uint16)(*ManagedRuntime,error){
	if !ip.Is4() || port==0{return nil,ErrNetworkPlan}
	lock,err:=lockNetwork(path);if err!=nil{return nil,err}
	success:=false;defer func(){if !success{lock.Close()}}()
	if err:=recoverJournal(path);err!=nil{return nil,err}
	if runCommandQuiet("ip","link","show","dev",plan.TUNName)==nil{return nil,errors.New("TUN name already exists; refusing to attach a foreign interface")}
	if out,err:=osExecOutput("ip","route","show","exact",plan.LeasePrefix.String());err!=nil || len(out)!=0{return nil,errors.New("lease-pool route already exists or cannot be inspected")}
	backend,err:=selectFirewallBackend(plan.Firewall.Backend);if err!=nil{return nil,err}
	if backend==FirewallNFT && runCommandQuiet("nft","list","table","inet",nftOwnedTable)==nil{return nil,errors.New("WBD table exists without an ownership journal")}
	tun,err:=OpenTUN(plan.TUNName);if err!=nil{return nil,err}
	r:=&Runtime{plan:plan,tun:tun,backend:backend,savedSysctls:map[string]string{}}
	j:=networkJournal{Version:1,Plan:plan,Backend:backend,Saved:r.savedSysctls,ListenIP:ip.String(),Port:port}
	boot,err:=os.ReadFile("/proc/sys/kernel/random/boot_id");if err!=nil{tun.Close();return nil,err};j.BootID=strings.TrimSpace(string(boot))
	for _,change:=range plan.Sysctls{value,err:=readSysctl(change.Key);if err!=nil{tun.Close();return nil,err};r.savedSysctls[change.Key]=value}
	if backend==FirewallNFT{j.Forward,j.OwnsForward,err=resolveNFTForward(plan.Firewall.NFTForward);if err!=nil{tun.Close();return nil,err}}
	// Write-ahead ownership is durable before the first sysctl/route/firewall change.
	if err:=journalWrite(path,j);err!=nil{tun.Close();return nil,err}
	if err:=r.apply();err!=nil{cleanupErr:=r.Close();if cleanupErr==nil{_ = os.Remove(path)};return nil,errors.Join(err,cleanupErr)}
	if err:=rstChange(j,true);err!=nil{cleanupErr:=errors.Join(rstChange(j,false),r.Close());if cleanupErr==nil{_ = os.Remove(path)};return nil,errors.Join(err,cleanupErr)}
	m:=&ManagedRuntime{Runtime:r,lock:lock,state:path,ip:ip.String(),port:port};success=true;return m,nil
}

func (m *ManagedRuntime)Close()error{
	if m==nil || m.lock==nil{return nil}
	j:=networkJournal{Backend:m.backend,ListenIP:m.ip,Port:m.port}
	err:=errors.Join(rstChange(j,false),m.Runtime.Close());if err==nil{if removeErr:=os.Remove(m.state);removeErr!=nil && !errors.Is(removeErr,os.ErrNotExist){err=removeErr}}
	lockErr:=m.lock.Close();m.lock=nil;return errors.Join(err,lockErr)
}

func osExecOutput(name string,args ...string)(string,error){out,err:=exec.Command(name,args...).CombinedOutput();if err!=nil{return "",fmt.Errorf("%s failed: %w",name,err)};return strings.TrimSpace(string(out)),nil}
