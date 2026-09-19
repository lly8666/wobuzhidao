//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const applyTunnelMTUPowerShell = `$ErrorActionPreference = 'Stop'
$name = $env:WBD_TUN_ADAPTER_NAME
$want = [uint32]$env:WBD_TUN_INTERFACE_MTU
$deadline = [DateTime]::UtcNow.AddSeconds(10)
$adapter = $null
$row = $null
do {
    $adapter = Get-NetAdapter -Name $name -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($adapter) {
        $row = Get-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
    }
    if (-not $row) { Start-Sleep -Milliseconds 100 }
} while (-not $row -and [DateTime]::UtcNow -lt $deadline)
if (-not $adapter -or -not $row) { throw "IPv4 interface row for $name did not become ready" }
$old = [uint32]$row.NlMtu
Set-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -NlMtuBytes $want -ErrorAction Stop
$verify = Get-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -ErrorAction Stop | Select-Object -First 1
if (-not $verify -or [uint32]$verify.NlMtu -ne $want) {
    try { Set-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -NlMtuBytes $old -ErrorAction SilentlyContinue } catch { }
    throw "IPv4 MTU readback mismatch: got=$([uint32]$verify.NlMtu) want=$want"
}
Write-Output $old
`

const restoreTunnelMTUPowerShell = `$ErrorActionPreference = 'Stop'
$name = $env:WBD_TUN_ADAPTER_NAME
$old = [uint32]$env:WBD_TUN_INTERFACE_MTU_RESTORE
$adapter = Get-NetAdapter -Name $name -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $adapter) { exit 0 }
$row = Get-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $row) { exit 0 }
Set-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -NlMtuBytes $old -ErrorAction Stop
$verify = Get-NetIPInterface -InterfaceIndex ([uint32]$adapter.ifIndex) -AddressFamily IPv4 -ErrorAction Stop | Select-Object -First 1
if (-not $verify -or [uint32]$verify.NlMtu -ne $old) { throw "IPv4 MTU restore readback mismatch" }
`

func configureTunnelInterfaceMTU(name string, mtu int) (func() error, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("Windows tunnel adapter name is required")
	}
	if mtu < 576 || mtu > 9000 {
		return nil, fmt.Errorf("invalid Windows tunnel MTU %d", mtu)
	}
	out, err := runTunnelMTUPowerShell(applyTunnelMTUPowerShell, map[string]string{
		"WBD_TUN_ADAPTER_NAME": name,
		"WBD_TUN_INTERFACE_MTU": strconv.Itoa(mtu),
	})
	if err != nil {
		return nil, fmt.Errorf("apply Windows tunnel MTU %d: %w: %s", mtu, err, strings.TrimSpace(string(out)))
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return nil, fmt.Errorf("apply Windows tunnel MTU %d returned no previous MTU", mtu)
	}
	previous, err := strconv.Atoi(lines[len(lines)-1])
	if err != nil || previous < 576 || previous > 65535 {
		return nil, fmt.Errorf("apply Windows tunnel MTU %d returned invalid previous MTU %q", mtu, lines[len(lines)-1])
	}
	fmt.Fprintf(os.Stderr, "WBD_TUN_WINDOWS_MTU_READY ifname=%s mtu=%d previous=%d verified=1\n", name, mtu, previous)

	restored := false
	return func() error {
		if restored {
			return nil
		}
		restored = true
		out, err := runTunnelMTUPowerShell(restoreTunnelMTUPowerShell, map[string]string{
			"WBD_TUN_ADAPTER_NAME": name,
			"WBD_TUN_INTERFACE_MTU_RESTORE": strconv.Itoa(previous),
		})
		if err != nil {
			return fmt.Errorf("restore Windows tunnel MTU %d: %w: %s", previous, err, strings.TrimSpace(string(out)))
		}
		fmt.Fprintf(os.Stderr, "WBD_TUN_WINDOWS_MTU_RESTORED ifname=%s mtu=%d verified=1\n", name, previous)
		return nil
	}, nil
}

func runTunnelMTUPowerShell(script string, env map[string]string) ([]byte, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd.CombinedOutput()
}
