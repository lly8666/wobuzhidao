param(
    [Parameter(Mandatory=$true)]
    [ValidatePattern('^[01]{3}$')]
    [string]$Policy
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$proxyLAN = $Policy[0] -eq '1'
$proxyChina = $Policy[1] -eq '1'
$proxyOther = $Policy[2] -eq '1'
$mode = if ($proxyOther) { 'Full' } elseif ($proxyChina -or $proxyLAN) { 'Split' } else { 'None' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ("wbd-routing-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $cn = Join-Path $temp 'cn4.txt'
    [IO.File]::WriteAllText($cn, "1.2.0.0/16`n", (New-Object Text.UTF8Encoding($false)))
    $routeScript = Join-Path $PSScriptRoot 'windows_tun_route.ps1'
    $args = @('-NoProfile','-ExecutionPolicy','Bypass','-File',$routeScript,'-Action','Render','-Mode',$mode,'-AdapterAlias','WBD','-TunnelAddress4','10.66.0.2/30','-Underlay4','198.51.100.10','-MTU','1400')
    if ($proxyChina -ne $proxyOther) {
        if ($proxyChina) { $args += @('-PrefixFile4',$cn) } else { $args += @('-DirectPrefixFile4',$cn) }
    }
    if ($proxyLAN) { $args += '-CaptureLAN' }
    $lines = @(& powershell.exe @args)
    if ($LASTEXITCODE -ne 0) { throw "route render failed policy=$Policy" }
    $plan = ($lines | Where-Object { $_ -like 'WBD_WINDOWS_TUN_PLAN*' } | Select-Object -First 1)
    if (-not $plan -or $plan -notmatch "mode=$mode") { throw "policy=$Policy mode mismatch: $plan" }
    $wantLAN = if ($proxyLAN) { 1 } else { 0 }
    if ($plan -notmatch "capture_lan=$wantLAN") { throw "policy=$Policy LAN marker mismatch: $plan" }
    if ($proxyLAN) {
        foreach ($prefix in @('10.0.0.0/8','172.16.0.0/12','192.168.0.0/16')) {
            if (-not ($lines | Where-Object { $_ -eq "03 CAPTURE IPv4 $prefix on WBD" })) { throw "policy=$Policy missing LAN capture $prefix" }
        }
    }
    if ($proxyChina -ne $proxyOther) {
        if ($proxyChina) {
            if (-not ($lines | Where-Object { $_ -eq '03 CAPTURE IPv4 1.2.0.0/16 on WBD' })) { throw "policy=$Policy CN prefix not captured" }
        } else {
            if (-not ($lines | Where-Object { $_ -eq '01 DIRECT IPv4 1.2.0.0/16 through the pre-WBD physical route' })) { throw "policy=$Policy CN prefix not direct" }
        }
    }
    if ($mode -eq 'None' -and ($lines | Where-Object { $_ -like '03 CAPTURE IPv4*' })) { throw "policy=$Policy None mode unexpectedly captured IPv4" }
    Write-Output "WBD_WINDOWS_ROUTING_POLICY_TEST_PASS policy=$Policy mode=$mode lan=$proxyLAN china=$proxyChina other=$proxyOther"
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}
