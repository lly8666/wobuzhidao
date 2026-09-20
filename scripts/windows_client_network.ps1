param(
    [ValidateSet('Render','Apply','Cleanup')]
    [string]$Action = 'Render',
    [string]$AdapterAlias = 'WBD',
    [string]$TunnelAddress4 = '',
    [string]$Underlay4 = '',
    [uint32]$PhysicalInterfaceIndex = 0,
    [string]$PhysicalNextHop4 = '',
    [string]$DNSServer = '',
    [string]$DirectPrefix4 = '',
    [string]$StatePath = "$PSScriptRoot\windows-client-state.json"
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$StateSchema = 'wbd-windows-client-state/v1'
$NRPTDisplayName = 'WBD Runtime DNS'
$NRPTComment = 'wbd-owned-runtime-dns/v1'
$IPv6Group = 'WBD Runtime IPv6 Kill Switch'
$IPv6Outbound = 'WBD Block IPv6 Outbound'
$IPv6Inbound = 'WBD Block IPv6 Inbound'
$IPv6Description = 'wbd-owned-runtime-ipv6-killswitch/v1'
$IPv6Universe = @('::/1','8000::/1')

function Assert-IPv4([string]$Value, [string]$Label) {
    $ip = $null
    if ([string]::IsNullOrWhiteSpace($Value) -or
        -not [System.Net.IPAddress]::TryParse($Value, [ref]$ip) -or
        $ip.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
        throw "$Label must be IPv4: $Value"
    }
}

function Parse-IPv4CIDR([string]$Value, [string]$Label, [switch]$Require32) {
    $parts = $Value.Split('/')
    if ($parts.Count -ne 2) { throw "$Label must be IPv4 CIDR: $Value" }
    Assert-IPv4 $parts[0] $Label
    $prefix = 0
    if (-not [int]::TryParse($parts[1], [ref]$prefix) -or $prefix -lt 0 -or $prefix -gt 32) {
        throw "$Label has invalid prefix: $Value"
    }
    if ($Require32 -and $prefix -ne 32) { throw "$Label must be /32: $Value" }
    return [pscustomobject]@{ IP=$parts[0]; PrefixLength=$prefix; CIDR=$Value }
}

function Parse-CSV([string]$Value) {
    if ([string]::IsNullOrWhiteSpace($Value)) { return @() }
    return @($Value -split '[,;]' | ForEach-Object { $_.Trim() } | Where-Object { $_ } | Select-Object -Unique)
}

function Require-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Windows client network Apply/Cleanup requires administrator privileges'
    }
}

function Save-State($State) {
    $dir = Split-Path -Parent $StatePath
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $json = $State | ConvertTo-Json -Depth 10
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($StatePath, $json + [Environment]::NewLine, $utf8NoBom)
}

function Remove-WBDNRPTByName([string]$Name) {
    if ([string]::IsNullOrWhiteSpace($Name)) { return }
    if (-not (Get-Command Remove-DnsClientNrptRule -ErrorAction SilentlyContinue)) { return }
    Remove-DnsClientNrptRule -Name $Name -Force -Confirm:$false -ErrorAction SilentlyContinue
}

function Remove-StaleWBDNRPT {
    if (-not (Get-Command Get-DnsClientNrptRule -ErrorAction SilentlyContinue) -or
        -not (Get-Command Remove-DnsClientNrptRule -ErrorAction SilentlyContinue)) { return }
    @(Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object {
        [string]$_.DisplayName -eq $NRPTDisplayName -and [string]$_.Comment -eq $NRPTComment
    }) | ForEach-Object {
        Remove-DnsClientNrptRule -Name ([string]$_.Name) -Force -Confirm:$false -ErrorAction SilentlyContinue
    }
}

function Remove-WBDIPv6Rules {
    if (-not (Get-Command Get-NetFirewallRule -ErrorAction SilentlyContinue) -or
        -not (Get-Command Remove-NetFirewallRule -ErrorAction SilentlyContinue)) { return }
    @(Get-NetFirewallRule -Group $IPv6Group -ErrorAction SilentlyContinue | Where-Object {
        $_.DisplayName -in @($IPv6Outbound,$IPv6Inbound) -and $_.Description -eq $IPv6Description
    }) | Remove-NetFirewallRule -ErrorAction SilentlyContinue
}

function Remove-OwnedRoutes($Routes) {
    foreach ($route in @($Routes)) {
        if ($null -eq $route) { continue }
        Remove-NetRoute -DestinationPrefix ([string]$route.DestinationPrefix) `
            -InterfaceIndex ([uint32]$route.InterfaceIndex) `
            -NextHop ([string]$route.NextHop) `
            -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
    }
}

function Remove-OwnedState($State) {
    if ($State.PSObject.Properties.Name -contains 'NRPTRuleName') {
        Remove-WBDNRPTByName ([string]$State.NRPTRuleName)
    }
    if ($State.PSObject.Properties.Name -contains 'CaptureRoutes') { Remove-OwnedRoutes $State.CaptureRoutes }
    if ($State.PSObject.Properties.Name -contains 'DirectRoutes') { Remove-OwnedRoutes $State.DirectRoutes }
    if ($State.PSObject.Properties.Name -contains 'UnderlayRoutes') { Remove-OwnedRoutes $State.UnderlayRoutes }
    if ($State.PSObject.Properties.Name -contains 'Addresses') {
        foreach ($addr in @($State.Addresses)) {
            Remove-NetIPAddress -InterfaceIndex ([uint32]$addr.InterfaceIndex) `
                -IPAddress ([string]$addr.IPAddress) -Confirm:$false -ErrorAction SilentlyContinue
        }
    }
    Remove-WBDIPv6Rules
}

$lease = Parse-IPv4CIDR $TunnelAddress4 'TunnelAddress4' -Require32
Assert-IPv4 $Underlay4 'Underlay4'
Assert-IPv4 $PhysicalNextHop4 'PhysicalNextHop4'
if ($PhysicalInterfaceIndex -eq 0) { throw 'PhysicalInterfaceIndex must be non-zero' }

$dnsServers = Parse-CSV $DNSServer
foreach ($dns in $dnsServers) { Assert-IPv4 $dns 'DNSServer' }
$directPrefixes = Parse-CSV $DirectPrefix4
foreach ($prefix in $directPrefixes) { [void](Parse-IPv4CIDR $prefix 'DirectPrefix4') }

$capturePrefixes = @('0.0.0.0/1','128.0.0.0/1') + @($dnsServers | ForEach-Object { "$_/32" })
$capturePrefixes = @($capturePrefixes | Select-Object -Unique)

if ($Action -eq 'Render') {
    Write-Output "WBD_WINDOWS_CLIENT_PLAN schema=$StateSchema adapter=$AdapterAlias lease=$($lease.CIDR)"
    Write-Output "01 UNDERLAY $Underlay4/32 ifindex=$PhysicalInterfaceIndex nexthop=$PhysicalNextHop4"
    foreach ($prefix in $directPrefixes) { Write-Output "02 DIRECT $prefix ifindex=$PhysicalInterfaceIndex nexthop=$PhysicalNextHop4" }
    Write-Output "03 ADDRESS_EXCLUSIVE $($lease.CIDR) adapter=$AdapterAlias dhcp=disabled"
    foreach ($prefix in $capturePrefixes) { Write-Output "04 CAPTURE $prefix adapter=$AdapterAlias" }
    if ($dnsServers.Count -gt 0) { Write-Output "05 DNS_NRPT namespace=. servers=$($dnsServers -join ',')" }
    Write-Output '06 IPV6_FAIL_CLOSED directions=inbound,outbound ranges=::/1,8000::/1'
    Write-Output '07 CLEANUP state_owned_only=1'
    exit 0
}

Require-Admin

if ($Action -eq 'Cleanup') {
    if (Test-Path -LiteralPath $StatePath) {
        $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
        Remove-OwnedState $state
        Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    } else {
        Remove-StaleWBDNRPT
        Remove-WBDIPv6Rules
    }
    Write-Output 'WBD_WINDOWS_CLIENT_CLEANUP_PASS'
    exit 0
}

foreach ($cmd in @(
    'Get-NetAdapter','Get-NetRoute','New-NetRoute','Remove-NetRoute',
    'Get-NetIPAddress','New-NetIPAddress','Remove-NetIPAddress','Set-NetIPInterface',
    'Get-NetFirewallProfile','Get-NetFirewallRule','New-NetFirewallRule','Remove-NetFirewallRule'
)) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "$cmd is unavailable" }
}
$disabledProfiles = @(Get-NetFirewallProfile -ErrorAction Stop | Where-Object { -not $_.Enabled })
if ($disabledProfiles.Count -gt 0) {
    throw "Windows Firewall profile disabled; refusing to connect because IPv6 could bypass WBD"
}

if (Test-Path -LiteralPath $StatePath) {
    $stale = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
    Remove-OwnedState $stale
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction Stop
}
Remove-StaleWBDNRPT
Remove-WBDIPv6Rules

$adapter = Get-NetAdapter -Name $AdapterAlias -ErrorAction Stop | Select-Object -First 1
$ifIndex = [uint32]$adapter.ifIndex
if ($ifIndex -eq $PhysicalInterfaceIndex) {
    throw 'physical underlay interface resolved to Wintun; refusing recursive capture'
}

$state = [ordered]@{
    Schema = $StateSchema
    AdapterAlias = $AdapterAlias
    AdapterInterfaceIndex = $ifIndex
    Addresses = @()
    UnderlayRoutes = @()
    DirectRoutes = @()
    CaptureRoutes = @()
    NRPTRuleName = ''
}
Save-State $state

try {
    $underlayPrefix = "$Underlay4/32"
    $existingUnderlay = Get-NetRoute -DestinationPrefix $underlayPrefix `
        -InterfaceIndex $PhysicalInterfaceIndex -NextHop $PhysicalNextHop4 `
        -PolicyStore ActiveStore -ErrorAction SilentlyContinue
    if (-not $existingUnderlay) {
        $state.UnderlayRoutes += [ordered]@{
            DestinationPrefix=$underlayPrefix
            InterfaceIndex=$PhysicalInterfaceIndex
            NextHop=$PhysicalNextHop4
        }
        Save-State $state
        New-NetRoute -DestinationPrefix $underlayPrefix -InterfaceIndex $PhysicalInterfaceIndex `
            -NextHop $PhysicalNextHop4 -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
    }

    foreach ($prefix in $directPrefixes) {
        $existing = Get-NetRoute -DestinationPrefix $prefix -InterfaceIndex $PhysicalInterfaceIndex `
            -NextHop $PhysicalNextHop4 -PolicyStore ActiveStore -ErrorAction SilentlyContinue
        if (-not $existing) {
            $state.DirectRoutes += [ordered]@{
                DestinationPrefix=$prefix
                InterfaceIndex=$PhysicalInterfaceIndex
                NextHop=$PhysicalNextHop4
            }
            Save-State $state
            New-NetRoute -DestinationPrefix $prefix -InterfaceIndex $PhysicalInterfaceIndex `
                -NextHop $PhysicalNextHop4 -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
        }
    }

    Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -Dhcp Disabled -ErrorAction Stop
    @(Get-NetIPAddress -InterfaceIndex $ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -ne $lease.IP }) | ForEach-Object {
            Remove-NetIPAddress -InterfaceIndex $ifIndex -IPAddress ([string]$_.IPAddress) `
                -Confirm:$false -ErrorAction Stop
        }
    $existingLease = Get-NetIPAddress -InterfaceIndex $ifIndex -IPAddress $lease.IP `
        -AddressFamily IPv4 -ErrorAction SilentlyContinue
    if (-not $existingLease) {
        $state.Addresses += [ordered]@{ InterfaceIndex=$ifIndex; IPAddress=$lease.IP }
        Save-State $state
        New-NetIPAddress -InterfaceIndex $ifIndex -IPAddress $lease.IP -PrefixLength 32 `
            -AddressFamily IPv4 -SkipAsSource $false | Out-Null
    }

    foreach ($prefix in $capturePrefixes) {
        $existing = Get-NetRoute -DestinationPrefix $prefix -InterfaceIndex $ifIndex `
            -NextHop '0.0.0.0' -PolicyStore ActiveStore -ErrorAction SilentlyContinue
        if (-not $existing) {
            $state.CaptureRoutes += [ordered]@{
                DestinationPrefix=$prefix
                InterfaceIndex=$ifIndex
                NextHop='0.0.0.0'
            }
            Save-State $state
            New-NetRoute -DestinationPrefix $prefix -InterfaceIndex $ifIndex -NextHop '0.0.0.0' `
                -RouteMetric 5 -PolicyStore ActiveStore | Out-Null
        }
    }

    if ($dnsServers.Count -gt 0) {
        foreach ($cmd in @('Add-DnsClientNrptRule','Get-DnsClientNrptRule','Remove-DnsClientNrptRule')) {
            if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "$cmd is unavailable" }
        }
        $rule = Add-DnsClientNrptRule -Namespace '.' -NameServers $dnsServers `
            -DisplayName $NRPTDisplayName -Comment $NRPTComment -PassThru -ErrorAction Stop
        if (-not $rule -or -not $rule.Name) { throw 'NRPT rule creation returned no rule id' }
        $state.NRPTRuleName = [string]$rule.Name
        Save-State $state
    }

    New-NetFirewallRule -DisplayName $IPv6Outbound -Group $IPv6Group -Description $IPv6Description `
        -Direction Outbound -Action Block -Enabled True -Profile Any -Protocol Any `
        -RemoteAddress $IPv6Universe | Out-Null
    New-NetFirewallRule -DisplayName $IPv6Inbound -Group $IPv6Group -Description $IPv6Description `
        -Direction Inbound -Action Block -Enabled True -Profile Any -Protocol Any `
        -LocalAddress $IPv6Universe | Out-Null

    Write-Output "WBD_WINDOWS_CLIENT_READY adapter=$AdapterAlias ifindex=$ifIndex lease=$($lease.CIDR) dns=$($dnsServers.Count) direct=$($directPrefixes.Count)"
    Write-Output "WBD_WINDOWS_CLIENT_UNDERLAY_LOCKED server=$Underlay4 ifindex=$PhysicalInterfaceIndex nexthop=$PhysicalNextHop4"
    Write-Output 'WBD_WINDOWS_CLIENT_IPV6_FAIL_CLOSED ready=1'
} catch {
    try { Remove-OwnedState ([pscustomobject]$state) } catch { }
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    throw
}
