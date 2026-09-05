package releasecontract

import (
	"strings"
	"testing"
)

func TestWindowsTunnelL3BoundaryContract(t *testing.T) {
	route := readRepoFile(t, "scripts/windows_tun_route.ps1")
	for _, want := range []string{
		`Set-NetIPInterface -InterfaceIndex $InterfaceIndex -AddressFamily IPv4 -Dhcp Disabled`,
		`Where-Object { $_.IPAddress -ne $IPAddress }`,
		`Remove-NetIPAddress -InterfaceIndex $InterfaceIndex -IPAddress ([string]$entry.IPAddress)`,
		`WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE`,
	} {
		requireContains(t, route, want, "Windows Wintun leased-address exclusivity")
	}
	if call := strings.Index(route, "Set-ExclusiveTunnelIPv4 -InterfaceIndex $ifIndex -IPAddress $addr4.IP"); call < 0 {
		t.Fatal("Windows route Apply must enforce exclusive server-assigned IPv4 after address readiness")
	}

	tun := readRepoFile(t, "internal/tunnel/tun_windows.go")
	for _, want := range []string{
		`func isIPv4Packet(packet []byte) bool`,
		`if !isIPv4Packet(packetBytes)`,
		`WBD_TUN_WINDOWS_NON_IPV4_DROP fail_closed=1`,
	} {
		requireContains(t, tun, want, "Windows TUN IPv4-only fail-closed boundary")
	}
	receive := strings.Index(tun, "wintunReceivePacket.Call")
	filter := strings.Index(tun, "if !isIPv4Packet(packetBytes)")
	copyPacket := strings.Index(tun, "copy(p[:packetSize], packetBytes)")
	if receive < 0 || filter <= receive || copyPacket <= filter {
		t.Fatalf("Windows TUN must validate packet family after Wintun receive and before forwarding copy: receive=%d filter=%d copy=%d", receive, filter, copyPacket)
	}
}

func TestWindowsTunCrossCompiles(t *testing.T) {
	crossCompileWindowsCommand(t, "./cmd/wbd-tun", "wbd-tun.exe")
}
