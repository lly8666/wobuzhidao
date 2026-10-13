package windowsclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This guard asserts the native PowerShell owned-transaction invariants at
// source level. It is not a substitute for an elevated Windows Wintun Apply.
func TestN4OwnedWindowsScriptGuardsForeignIPv4AndRestoresOnlyOwnedDHCP(t *testing.T) {
	raw,err:=os.ReadFile(filepath.Join("..","..","scripts","windows_client_network.ps1"))
	if err!=nil {t.Fatal(err)}
	s:=string(raw)
	need:=[]string{
		"non-owned IPv4 addresses; refusing destructive takeover",
		"non-owned Wintun IPv4 address appeared during Apply; refusing deletion",
		"DHCPState = $null",
		"$state.DHCPState = [ordered]@{",
		"Previous=[string]$beforeMTU[0].Dhcp; Applied='Disabled'",
		"Restore-OwnedTunnelDHCP $State.DHCPState",
		"if ([string]$current[0].Dhcp -eq [string]$DHCPState.Applied)",
		"rollback incomplete, state journal retained",
	}
	for _,v:=range need {if !strings.Contains(s,v) {t.Fatalf("owned safety contract lost %q",v)}}
	if strings.Contains(s,"Remove-NetIPAddress -InterfaceIndex $ifIndex -IPAddress") {
		t.Fatal("Apply silently deletes unowned IPv4 from Wintun")
	}
	pref:=strings.Index(s,"$foreignIPv4 = @(")
	owned:=strings.Index(s,"$state = [ordered]@{")
	if pref<0||owned<0||pref>=owned {
		t.Fatal("unowned-address preflight must run before any owned mutation journal")
	}
	save:=strings.Index(s,"$state.DHCPState = [ordered]@{")
	disable:=strings.Index(s,"Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -Dhcp Disabled")
	if save<0||disable<0||save>=disable {
		t.Fatal("WBD must journal original DHCP before modifying it")
	}
}
