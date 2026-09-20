package windowsclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPowerShellNetworkContractKeepsOwnershipAndIPv6FailClosed(t *testing.T) {
	body,err:=os.ReadFile(filepath.Join("..","..","scripts","windows_client_network.ps1"))
	if err!=nil { t.Fatal(err) }
	text:=string(body)
	for _,want:=range []string{
		"wbd-windows-client-state/v1",
		"Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -Dhcp Disabled",
		"Where-Object { $_.IPAddress -ne $lease.IP }",
		"$underlayPrefix = \"$Underlay4/32\"",
		"physical underlay interface resolved to Wintun; refusing recursive capture",
		"0.0.0.0/1","128.0.0.0/1",
		"Add-DnsClientNrptRule -Namespace '.'",
		"wbd-owned-runtime-dns/v1",
		"WBD Runtime IPv6 Kill Switch",
		"::/1","8000::/1",
		"Remove-OwnedState",
		"WBD_WINDOWS_CLIENT_READY",
	} {
		if !strings.Contains(text,want) { t.Fatalf("script lost %q",want) }
	}
}
