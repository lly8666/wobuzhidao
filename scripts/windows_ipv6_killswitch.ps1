param(
    [ValidateSet('Render','Apply','Cleanup')]
    [string]$Action = 'Render'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Group = 'WBD Runtime IPv6 Kill Switch'
$Outbound = 'WBD Block IPv6 Outbound'
$Inbound = 'WBD Block IPv6 Inbound'
$Description = 'wbd-owned-runtime-ipv6-killswitch/v1'
# Windows Firewall rejects the zero-length IPv6 prefix ::/0 on current
# Windows 11/Server NetSecurity. These two valid /1 prefixes are an exact
# partition of the full IPv6 address space and therefore preserve the
# device-wide fail-closed contract without touching IPv4.
$IPv6Universe = @('::/1', '8000::/1')

function Require-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'IPv6 kill-switch Apply/Cleanup requires administrator privileges'
    }
}

function Remove-WBDRules {
    if (-not (Get-Command Get-NetFirewallRule -ErrorAction SilentlyContinue) -or
        -not (Get-Command Remove-NetFirewallRule -ErrorAction SilentlyContinue)) {
        return
    }
    @(Get-NetFirewallRule -Group $Group -ErrorAction SilentlyContinue | Where-Object {
        $_.DisplayName -in @($Outbound, $Inbound) -and $_.Description -eq $Description
    }) | Remove-NetFirewallRule -ErrorAction SilentlyContinue
}

if ($Action -eq 'Render') {
    Write-Output 'WBD_WINDOWS_IPV6_KILLSWITCH_PLAN scope=device directions=inbound,outbound range=::/1,8000::/1 restore=remove_wbd_owned_rules'
    exit 0
}

Require-Admin

if ($Action -eq 'Cleanup') {
    Remove-WBDRules
    Write-Output 'WBD_WINDOWS_IPV6_KILLSWITCH_CLEANUP_PASS'
    exit 0
}

foreach ($cmd in @('New-NetFirewallRule','Get-NetFirewallRule','Remove-NetFirewallRule','Get-NetFirewallProfile')) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        throw "$cmd is unavailable; refusing to connect without a device-wide IPv6 kill switch"
    }
}

# WBD requires Windows Firewall enforcement while active. Do not silently turn
# a user-disabled profile on and later guess how to restore it; fail closed.
$disabled = @(Get-NetFirewallProfile -ErrorAction Stop | Where-Object { -not $_.Enabled })
if ($disabled.Count -gt 0) {
    $names = ($disabled | ForEach-Object { $_.Name }) -join ','
    throw "Windows Firewall profile(s) disabled: $names; refusing to connect because IPv6 could bypass WBD"
}

# Crash recovery: remove only exact WBD-owned rules from a prior interrupted run.
Remove-WBDRules

try {
    New-NetFirewallRule -DisplayName $Outbound -Group $Group -Description $Description `
        -Direction Outbound -Action Block -Enabled True -Profile Any -Protocol Any `
        -RemoteAddress $IPv6Universe | Out-Null
    New-NetFirewallRule -DisplayName $Inbound -Group $Group -Description $Description `
        -Direction Inbound -Action Block -Enabled True -Profile Any -Protocol Any `
        -LocalAddress $IPv6Universe | Out-Null

    $rules = @(Get-NetFirewallRule -Group $Group -ErrorAction Stop | Where-Object {
        $_.DisplayName -in @($Outbound, $Inbound) -and $_.Description -eq $Description -and $_.Enabled -eq 'True'
    })
    if ($rules.Count -ne 2) {
        throw "IPv6 kill-switch verification expected 2 WBD rules, found $($rules.Count)"
    }
    Write-Output 'WBD_WINDOWS_IPV6_KILLSWITCH_READY scope=device ipv6=blocked directions=inbound,outbound'
} catch {
    Remove-WBDRules
    throw
}
