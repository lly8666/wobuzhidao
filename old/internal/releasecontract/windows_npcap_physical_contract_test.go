package releasecontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsNpcapPhysicalApplicationPathContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/windows-npcap-physical.yml")
	script := readRepoFile(t, "scripts/windows_application_path_qualify.ps1")
	harness := readRepoFile(t, "cmd/wbd-windows-qualify/main_windows.go")

	for _, want := range []string{
		"udp_echo_target:",
		"tcp_echo_target:",
		"probe_rounds:",
		"probe_payload_bytes:",
		"wbd-physical-localappdata",
		"go build -trimpath -o $qualifier .\\cmd\\wbd-windows-qualify",
		"Run routed UDP TCP and moderate sustained application qualification",
		"WBD_WINDOWS_APPLICATION_QUALIFIER_BUILD_PASS",
	} {
		requireContains(t, workflow, want, "Windows physical Npcap workflow")
	}
	for _, want := range []string{
		"Find-NetRoute -RemoteIPAddress",
		"AdapterInterfaceIndex",
		"Invoke-UDPEcho",
		"Invoke-TCPEcho",
		"WBD_WINDOWS_APPLICATION_ROUTE_PASS",
		"WBD_WINDOWS_APPLICATION_UDP_PASS",
		"WBD_WINDOWS_APPLICATION_TCP_PASS",
		"WBD_WINDOWS_APPLICATION_PATH_PASS",
		"route_fence=1",
		"application_one_way_bytes=",
		"cleanup=1",
	} {
		requireContains(t, script, want, "Windows application-path qualification script")
	}

	connectAt := strings.Index(harness, "if err := controller.Connect(profile); err != nil")
	readyAt := strings.Index(harness, "os.WriteFile(*readyFile")
	cleanupAt := strings.Index(harness, "WBD_WINDOWS_QUALIFY_CLEANUP_PASS")
	if connectAt < 0 || readyAt < 0 || readyAt <= connectAt {
		t.Fatal("Windows qualifier ready-file must be written only after Controller.Connect succeeds")
	}
	if cleanupAt < 0 || cleanupAt <= readyAt {
		t.Fatal("Windows qualifier cleanup PASS must occur after connected ready-file")
	}
	requireContains(t, harness, `flag.String("ready-file"`, "Windows connected qualification harness")
	requireContains(t, harness, `defer os.Remove(*readyFile)`, "Windows connected qualification harness")

	if strings.Contains(workflow, "-UninstallNpcapAfter") {
		t.Fatal("physical application qualification must not uninstall runner-owned Npcap before application-path probes")
	}
	requireContains(t, workflow, "uninstall_after must remain false", "runner-owned Npcap lifecycle fence")
}

func TestWindowsNpcapPhysicalHandshakeReceiptContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/windows-npcap-physical.yml")
	npcap := readRepoFile(t, "cmd/wbd-faketcp/main_windows.go")

	for _, want := range []string{
		"WBD_FAKETCP_WINDOWS_RAW_SYN_TX",
		"WBD_FAKETCP_WINDOWS_RAW_SYNACK_RX",
		"flowTCPFlags",
	} {
		requireContains(t, npcap, want, "Windows Npcap handshake receipt source")
	}
	for _, want := range []string{
		"WBD_WINDOWS_NPCAP_HANDSHAKE_RECEIPT_PASS",
		"raw_syn_tx=1",
		"raw_synack_rx=1",
		`'^WBD_FAKETCP_WINDOWS_RAW_SYN_TX\b'`,
		`'^WBD_FAKETCP_WINDOWS_RAW_SYNACK_RX\b'`,
	} {
		requireContains(t, workflow, want, "Windows physical Npcap handshake receipt gate")
	}

	sendAt := strings.Index(npcap, "r.pcapSendPacket.Call")
	synAt := strings.Index(npcap, "WBD_FAKETCP_WINDOWS_RAW_SYN_TX")
	if sendAt < 0 || synAt < 0 || synAt <= sendAt {
		t.Fatal("raw SYN receipt must be emitted only after pcap_sendpacket is called successfully")
	}
	filterAt := strings.Index(npcap, "if !r.matchesInboundFlow(packet)")
	synAckAt := strings.Index(npcap, "WBD_FAKETCP_WINDOWS_RAW_SYNACK_RX")
	if filterAt < 0 || synAckAt < 0 || synAckAt <= filterAt {
		t.Fatal("raw SYNACK receipt must be emitted only after exact inbound Npcap flow filtering")
	}
}

func crossCompileWindowsCommand(t *testing.T, pkg, output string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), output)
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, pkg)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-compile %s: %v\n%s", pkg, err, b)
	}
}

func TestWindowsPhysicalQualifierCrossCompiles(t *testing.T) {
	crossCompileWindowsCommand(t, "./cmd/wbd-windows-qualify", "wbd-windows-qualify.exe")
}

func TestWindowsNpcapFakeTCPCrossCompiles(t *testing.T) {
	crossCompileWindowsCommand(t, "./cmd/wbd-faketcp", "wbd-faketcp.exe")
}
