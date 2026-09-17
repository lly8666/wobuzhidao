package releasecontract

import "testing"

// The operator-visible Windows MTU is the outer connection ceiling. It must
// reach FakeTCP before DTLS so a WAN PMTU below the local NIC MTU cannot create
// full-interface-sized carrier fragments. Linux keeps its historical WBD_MTU
// as inner IP MTU and exposes a distinct outer connection ceiling.
func TestCarrierConnectionMTUReleaseContract(t *testing.T) {
	windowsPlan := readRepoFile(t, "internal/windowsruntime/plan.go")
	windowsLanes := readRepoFile(t, "internal/windowsruntime/multilane.go")
	client := readRepoFile(t, "cmd/wbd-faketcp/main.go")
	mux := readRepoFile(t, "cmd/wbd-faketcp-mux/main_linux.go")
	carrier := readRepoFile(t, "internal/faketcp/carrier_path.go")
	linuxManager := readRepoFile(t, "scripts/linux_server_manager.sh")

	requireContains(t, windowsPlan, `"--connection-mtu", strconv.Itoa(profile.MTU)`, "Windows diagnostic FakeTCP command must fence carrier MTU")
	requireContains(t, windowsPlan, `"--shadow-recovery", "legacy"`, "Windows diagnostic FakeTCP command must keep legacy recovery")
	requireContains(t, windowsLanes, `"--connection-mtu", strconv.Itoa(profile.MTU)`, "Windows product lane bootstrap must pass Profile.MTU to FakeTCP")
	requireContains(t, windowsLanes, `"--shadow-recovery", "legacy"`, "Windows product lane bootstrap must keep legacy recovery")
	requireContains(t, client, `faketcp.CarrierMTUForIPv4(rawLocal.IP, c.connectionMTU)`, "Windows FakeTCP must apply configured connection MTU")
	requireContains(t, mux, `faketcp.CarrierMTUForIPv4(la.IP, c.connectionMTU)`, "Linux mux must apply configured connection MTU")
	requireContains(t, carrier, `if configured < interfaceMTU`, "configured carrier ceiling may only shrink local interface MTU")
	requireContains(t, carrier, `return interfaceMTU, nil`, "configured carrier ceiling must never manufacture a larger local path")

	requireContains(t, linuxManager, `WBD_MTU=1360`, "Linux inner IP MTU compatibility setting")
	requireContains(t, linuxManager, `WBD_CONNECTION_MTU=1500`, "Linux explicit outer connection MTU setting")
	requireContains(t, linuxManager, `WBD_CONNECTION_MTU must be numeric`, "Linux outer connection MTU validation")
	requireContains(t, linuxManager, `WBD_CONNECTION_MTU must be 576..9000`, "Linux outer connection MTU range validation")
	requireContains(t, linuxManager, `--connection-mtu "$WBD_CONNECTION_MTU"`, "Linux manager must pass outer connection MTU to FakeTCP mux")
	requireContains(t, linuxManager, `WBD_MTU (inner IP MTU, 576..1460)`, "Linux WBD_MTU must remain inner IP MTU")
}
