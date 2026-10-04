//go:build linux

package linuxserver

import (
	"net/netip"
	"os"
	"testing"
)

// Each phase runs as an independent process in one Actions-only network namespace.
func TestManagedNetworkProcessPhase(t *testing.T) {
	phase:=os.Getenv("WBD_MANAGED_PHASE");if phase==""{t.Skip("Actions isolated root namespace only")}
	if os.Geteuid()!=0{t.Fatal("requires root")}
	path:=os.Getenv("WBD_MANAGED_STATE");backend:=FirewallBackend(os.Getenv("WBD_MANAGED_BACKEND"))
	if phase=="recover"{
		if err:=RecoverManagedNetwork(path);err!=nil{t.Fatal(err)}
		if runCommandQuiet("ip","link","show","dev","wbdg0")==nil{t.Fatal("TUN survived")}
		value,err:=readSysctl("net.ipv4.ip_forward");if err!=nil || value!="0"{t.Fatal("forward not restored",err)}
		return
	}
	plan,err:=BuildNetworkPlan("wbdg0",netip.MustParsePrefix("10.66.0.0/24"),1400,backend,"");if err!=nil{t.Fatal(err)}
	m,err:=OpenManagedRuntime(plan,path,netip.MustParseAddr("127.0.0.2"),24443);if err!=nil{t.Fatal(err)}
	if _,err:=OpenManagedRuntime(plan,path,netip.MustParseAddr("127.0.0.2"),24443);err==nil{t.Fatal("overlapping instance allowed")}
	if phase=="crash"{os.Exit(0)} // no Go defers; kernel closes the nonpersistent TUN
	if err:=m.Close();err!=nil{t.Fatal(err)}
	if _,err:=os.Stat(path);!os.IsNotExist(err){t.Fatal("clean close left journal")}
}
