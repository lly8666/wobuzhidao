param(
    [Parameter(Mandatory=$true)]
    [ValidatePattern('^[01]{3}$')]
    [string]$Policy
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest

# Routing policy is no longer encoded as thousands of host routes. Every policy
# uses the same small host capture plan; proxy_lan/proxy_china/proxy_other are
# consumed by wbd-tun after packets enter Wintun.
$routeScript = Join-Path $PSScriptRoot 'windows_tun_route.ps1'
$args = @(
    '-NoProfile','-ExecutionPolicy','Bypass','-File',$routeScript,
    '-Action','Render',
    '-Mode','Full',
    '-AdapterAlias','WBD',
    '-TunnelAddress4','10.66.0.2/30',
    '-Underlay4','198.51.100.10',
    '-MTU','1400',
    '-CaptureLAN'
)
$lines = @(& powershell.exe @args)
if ($LASTEXITCODE -ne 0) { throw "route render failed policy=$Policy" }
$plan = ($lines | Where-Object { $_ -like 'WBD_WINDOWS_TUN_PLAN*' } | Select-Object -First 1)
if (-not $plan -or $plan -notmatch 'mode=Full' -or $plan -notmatch 'capture_lan=1') {
    throw "policy=$Policy host capture plan mismatch: $plan"
}
foreach ($prefix in @('0.0.0.0/1','128.0.0.0/1','10.0.0.0/8','172.16.0.0/12','192.168.0.0/16')) {
    if (-not ($lines | Where-Object { $_ -eq "03 CAPTURE IPv4 $prefix on WBD" })) {
        throw "policy=$Policy missing all-traffic capture $prefix"
    }
}
if ($lines | Where-Object { $_ -like '01 DIRECT IPv4 1.2.*' }) {
    throw "policy=$Policy leaked CN classification into host route table"
}
Write-Output "WBD_WINDOWS_ROUTING_POLICY_TEST_PASS policy=$Policy host_mode=Full capture_lan=1 split=in-tun"
