param(
    [ValidateSet('Render','Apply','Cleanup')]
    [string]$Action = 'Render',
    [string]$AdapterAlias = 'WBD',
    [ValidateSet('Full','Split','None')]
    [string]$Mode = 'Full',
    [string]$TunnelAddress4 = '10.66.0.2/30',
    [string]$TunnelAddress6 = '',
    [string]$Underlay4 = '',
    [string]$Underlay6 = '',
    [string[]]$Prefix4 = @(),
    [string[]]$Prefix6 = @(),
    [string]$PrefixFile4 = '',
    [string[]]$DirectPrefix4 = @(),
    [string]$DirectPrefixFile4 = '',
    [switch]$CaptureLAN,
    # Comma/semicolon separated by design: powershell.exe -File is launched by
    # the Go controller and scalar CLI transport is deterministic across Windows
    # PowerShell versions, unlike repeated external string[] argument binding.
    [string]$DNSServer = '',
    [ValidateRange(576,9000)]
    [int]$MTU = 1400,
    [string]$StatePath = "$PSScriptRoot\route-state.json"
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$NRPTDisplayName = 'WBD Runtime DNS'
$NRPTComment = 'wbd-owned-runtime-dns/v1'

function Assert-IP([string]$Value, [System.Net.Sockets.AddressFamily]$Family, [string]$Label) {
    if ([string]::IsNullOrWhiteSpace($Value)) { return }
    $ip = $null
    if (-not [System.Net.IPAddress]::TryParse($Value, [ref]$ip) -or $ip.AddressFamily -ne $Family) {
        throw "$Label must be a valid $Family address: $Value"
    }
}

function Parse-CIDR([string]$CIDR, [System.Net.Sockets.AddressFamily]$Family, [string]$Label) {
    if ([string]::IsNullOrWhiteSpace($CIDR)) { return $null }
    $parts = $CIDR.Split('/')
    if ($parts.Count -ne 2) { throw "$Label must be CIDR: $CIDR" }
    Assert-IP $parts[0] $Family $Label
    $prefix = 0
    if (-not [int]::TryParse($parts[1], [ref]$prefix)) { throw "$Label has invalid prefix: $CIDR" }
    $max = if ($Family -eq [System.Net.Sockets.AddressFamily]::InterNetwork) { 32 } else { 128 }
    if ($prefix -lt 0 -or $prefix -gt $max) { throw "$Label has invalid prefix: $CIDR" }
    return [pscustomobject]@{ IP = $parts[0]; PrefixLength = $prefix; CIDR = $CIDR }
}

function Test-RFC1918Prefix([string]$CIDR) {
    $parsed = Parse-CIDR $CIDR ([System.Net.Sockets.AddressFamily]::InterNetwork) 'RFC1918 route'
    if (-not $parsed) { return $false }
    $octets = @($parsed.IP.Split('.') | ForEach-Object { [int]$_ })
    if ($octets[0] -eq 10) { return $parsed.PrefixLength -ge 8 }
    if ($octets[0] -eq 172 -and $octets[1] -ge 16 -and $octets[1] -le 31) { return $parsed.PrefixLength -ge 12 }
    if ($octets[0] -eq 192 -and $octets[1] -eq 168) { return $parsed.PrefixLength -ge 16 }
    return $false
}

function Test-RFC1918Address([string]$IPAddress) {
    if ([string]::IsNullOrWhiteSpace($IPAddress) -or $IPAddress -eq '0.0.0.0') { return $false }
    return Test-RFC1918Prefix "$IPAddress/32"
}

function Get-PhysicalRFC1918RoutePrefixes([uint32]$TunnelInterfaceIndex) {
    $out = @()
    foreach ($route in @(Get-NetRoute -AddressFamily IPv4 -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
        if ([uint32]$route.InterfaceIndex -eq $TunnelInterfaceIndex) { continue }
        $prefix = [string]$route.DestinationPrefix
        if (Test-RFC1918Prefix $prefix) { $out += $prefix }
    }
    return @($out | Select-Object -Unique)
}

function Read-PrefixFile([string]$Path, [System.Net.Sockets.AddressFamily]$Family, [string]$Label) {
    if ([string]::IsNullOrWhiteSpace($Path)) { return }
    if (-not (Test-Path -LiteralPath $Path)) { throw "$Label not found: $Path" }
    foreach ($raw in Get-Content -LiteralPath $Path) {
        $line = ([string]$raw).Trim()
        if (-not $line -or $line.StartsWith('#')) { continue }
        [void](Parse-CIDR $line $Family $Label)
        Write-Output $line
    }
}

function Require-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Apply/Cleanup requires an elevated PowerShell session'
    }
}

function Wait-NetAdapterByName([string]$Name, [int]$TimeoutMilliseconds = 10000) {
    $deadline = [DateTime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    do {
        $adapter = Get-NetAdapter -Name $Name -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($adapter) { return $adapter }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Wintun adapter $Name did not appear within ${TimeoutMilliseconds}ms"
}

function Wait-PreferredIPAddress([uint32]$InterfaceIndex, [string]$IPAddress, [string]$Family, [int]$TimeoutMilliseconds = 10000) {
    $deadline = [DateTime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    $lastState = 'Missing'
    do {
        $row = Get-NetIPAddress -InterfaceIndex $InterfaceIndex -IPAddress $IPAddress -AddressFamily $Family -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($row) {
            $lastState = [string]$row.AddressState
            if ($lastState -eq 'Preferred') {
                Write-Output "WBD_WINDOWS_TUN_ADDRESS_READY family=$Family ip=$IPAddress state=Preferred"
                return
            }
            if ($lastState -in @('Duplicate','Invalid')) {
                throw "WBD tunnel address $IPAddress entered unusable DAD state $lastState"
            }
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "WBD tunnel address $IPAddress did not become Preferred within ${TimeoutMilliseconds}ms; last_state=$lastState"
}

function Set-ExclusiveTunnelIPv4([uint32]$InterfaceIndex, [string]$IPAddress, [int]$TimeoutMilliseconds = 3000) {
    # This adapter is WBD-owned and its IPv4 identity is the server-assigned
    # Logical Tunnel lease. It must never retain DHCP/APIPA or another stale
    # address because Windows source selection could otherwise bypass the lease
    # identity and be correctly rejected by the server anti-spoof boundary.
    Set-NetIPInterface -InterfaceIndex $InterfaceIndex -AddressFamily IPv4 -Dhcp Disabled -ErrorAction Stop
    $removed = 0
    $otherAddresses = @(Get-NetIPAddress -InterfaceIndex $InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -ne $IPAddress })
    foreach ($entry in $otherAddresses) {
        Remove-NetIPAddress -InterfaceIndex $InterfaceIndex -IPAddress ([string]$entry.IPAddress) -Confirm:$false -ErrorAction Stop
        $removed++
    }

    # Re-read for a bounded interval so an automatic address racing the static
    # lease cannot turn a transiently-clean adapter into a false qualification.
    $deadline = [DateTime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    do {
        $unexpected = @(Get-NetIPAddress -InterfaceIndex $InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
            Where-Object { $_.IPAddress -ne $IPAddress })
        if ($unexpected.Count -eq 0) {
            Write-Output "WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE ifindex=$InterfaceIndex address4=$IPAddress removed_nonlease=$removed dhcp=disabled"
            return
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "WBD-owned adapter retained non-lease IPv4 addresses: $($unexpected.IPAddress -join ',')"
}

function Save-State($State) {
    $dir = Split-Path -Parent $StatePath
    if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $json = $State | ConvertTo-Json -Depth 10
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($StatePath, $json + [Environment]::NewLine, $utf8NoBom)
}

function Remove-WBDNRPTRuleByName([string]$Name) {
    if ([string]::IsNullOrWhiteSpace($Name)) { return }
    if (-not (Get-Command Remove-DnsClientNrptRule -ErrorAction SilentlyContinue)) { return }
    try {
        Remove-DnsClientNrptRule -Name $Name -Force -Confirm:$false -ErrorAction Stop
    } catch {
        $missing = ($_.CategoryInfo.Category -eq [System.Management.Automation.ErrorCategory]::ObjectNotFound) -or
                   ([string]$_.FullyQualifiedErrorId -match '1168|ObjectNotFound')
        if ($missing) {
            Write-Output "WBD_WINDOWS_TUN_NRPT_ALREADY_ABSENT name=$Name"
            return
        }
        throw
    }
}

function Remove-StaleWBDNRPT {
    if (-not (Get-Command Get-DnsClientNrptRule -ErrorAction SilentlyContinue) -or
        -not (Get-Command Remove-DnsClientNrptRule -ErrorAction SilentlyContinue)) {
        return
    }
    $rules = @(Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object {
        [string]$_.DisplayName -eq $NRPTDisplayName -and [string]$_.Comment -eq $NRPTComment
    })
    foreach ($rule in $rules) {
        Remove-WBDNRPTRuleByName ([string]$rule.Name)
    }
}

function Remove-OwnedRoutes($Routes, [string]$Label) {
    $groups = @{}
    foreach ($route in @($Routes)) {
        if ($null -eq $route) { continue }
        $ifIndex = [uint32]$route.InterfaceIndex
        $nextHop = [string]$route.NextHop
        $key = "$ifIndex|$nextHop"
        if (-not $groups.ContainsKey($key)) {
            $groups[$key] = [pscustomobject]@{
                InterfaceIndex = $ifIndex
                NextHop = $nextHop
                Prefixes = [System.Collections.Generic.List[string]]::new()
            }
        }
        [void]$groups[$key].Prefixes.Add([string]$route.DestinationPrefix)
    }
    $removed = 0
    foreach ($batch in $groups.Values) {
        $prefixes = @($batch.Prefixes)
        if ($prefixes.Count -eq 0) { continue }
        Remove-NetRoute -DestinationPrefix $prefixes -InterfaceIndex ([uint32]$batch.InterfaceIndex) -NextHop ([string]$batch.NextHop) -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
        $removed += $prefixes.Count
    }
    if ($removed -gt 0) { Write-Output "WBD_WINDOWS_TUN_ROUTE_CLEAN_BATCH label=$Label routes=$removed groups=$($groups.Count)" }
}

function Remove-Owned-State($State) {
    # Stop steering new ordinary DNS queries first. Then remove WBD routes while
    # Wintun/LINK/DTLS/FakeTCP are still alive; process teardown remains outside
    # this script and is strictly after route cleanup in Executor.Stop().
    if ($State.PSObject.Properties.Name -contains 'NRPTRuleName' -and $State.NRPTRuleName) {
        Remove-WBDNRPTRuleByName ([string]$State.NRPTRuleName)
    } elseif ($State.PSObject.Properties.Name -contains 'DNSConfigured' -and $State.DNSConfigured) {
        Remove-StaleWBDNRPT
    }
    if ($State.PSObject.Properties.Name -contains 'CaptureRoutes') {
        Remove-OwnedRoutes $State.CaptureRoutes 'capture'
    }
    if ($State.PSObject.Properties.Name -contains 'DirectRoutes') {
        Remove-OwnedRoutes $State.DirectRoutes 'direct'
    }
    if ($State.PSObject.Properties.Name -contains 'UnderlayRoutes') {
        Remove-OwnedRoutes $State.UnderlayRoutes 'underlay'
    }
    if ($State.PSObject.Properties.Name -contains 'Addresses') {
        foreach ($addr in @($State.Addresses)) {
            Remove-NetIPAddress -InterfaceIndex ([uint32]$addr.InterfaceIndex) -IPAddress $addr.IPAddress -Confirm:$false -ErrorAction SilentlyContinue
        }
    }
    if ($State.PSObject.Properties.Name -contains 'MTU4' -and $null -ne $State.MTU4) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -NlMtuBytes ([uint32]$State.MTU4) -ErrorAction SilentlyContinue
    }
    if ($State.PSObject.Properties.Name -contains 'InterfaceMetric4' -and $null -ne $State.InterfaceMetric4) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -InterfaceMetric ([uint32]$State.InterfaceMetric4) -ErrorAction SilentlyContinue
    }
    if ($State.PSObject.Properties.Name -contains 'MTU6' -and $null -ne $State.MTU6) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv6 -NlMtuBytes ([uint32]$State.MTU6) -ErrorAction SilentlyContinue
    }
}

$Prefix4 = @($Prefix4) + @(Read-PrefixFile $PrefixFile4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'PrefixFile4')
$DirectPrefix4 = @($DirectPrefix4) + @(Read-PrefixFile $DirectPrefixFile4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'DirectPrefixFile4')
$Prefix4 = @($Prefix4 | Select-Object -Unique)
$DirectPrefix4 = @($DirectPrefix4 | Select-Object -Unique)
$DNSServers = @()
if (-not [string]::IsNullOrWhiteSpace($DNSServer)) {
    $DNSServers = @($DNSServer -split '[,;]' | ForEach-Object { $_.Trim() } | Where-Object { $_ } | Select-Object -Unique)
}

$addr4 = Parse-CIDR $TunnelAddress4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'TunnelAddress4'
$addr6 = Parse-CIDR $TunnelAddress6 ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'TunnelAddress6'
Assert-IP $Underlay4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'Underlay4'
Assert-IP $Underlay6 ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'Underlay6'
foreach ($p in $Prefix4) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetwork) 'Prefix4') }
foreach ($p in $Prefix6) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'Prefix6') }
foreach ($p in $DirectPrefix4) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetwork) 'DirectPrefix4') }
foreach ($dns in $DNSServers) { Assert-IP $dns ([System.Net.Sockets.AddressFamily]::InterNetwork) 'DNSServer' }
if ($DirectPrefix4.Count -gt 0 -and -not $Underlay4) { throw 'DirectPrefix4 requires Underlay4 so the pre-WBD physical route is known' }

if ($Mode -eq 'Full') {
    $capture4 = @('0.0.0.0/1','128.0.0.0/1')
    $capture6 = if ($addr6) { @('::/1','8000::/1') } else { @() }
} elseif ($Mode -eq 'Split') {
    $capture4 = @($Prefix4)
    $capture6 = @($Prefix6)
} else {
    $capture4 = @()
    $capture6 = @()
}
if ($CaptureLAN) { $capture4 = @($capture4) + @('10.0.0.0/8','172.16.0.0/12','192.168.0.0/16') }
if ($Mode -eq 'Split' -and $capture4.Count -eq 0 -and $capture6.Count -eq 0 -and $DNSServers.Count -eq 0) { throw 'Split mode requires Prefix4/Prefix6, CaptureLAN and/or DNSServer' }
# Every configured DNS upstream is explicitly captured through WBD, even when a
# more-specific domestic direct route would otherwise match it.
$capture4 = @($capture4) + @($DNSServers | ForEach-Object { "$_/32" })
$capture4 = @($capture4 | Select-Object -Unique)
$configureIPv6 = ($null -ne $addr6) -or (@($capture6).Count -gt 0)

if ($Action -eq 'Render') {
    Write-Output "WBD_WINDOWS_TUN_PLAN mode=$Mode adapter=$AdapterAlias mtu=$MTU capture_lan=$([int]$CaptureLAN.IsPresent)"
    if ($Underlay4) { Write-Output "01 ESCAPE IPv4 $Underlay4/32 through the pre-WBD best route before capture routes" }
    if ($Underlay6) { Write-Output "01 ESCAPE IPv6 $Underlay6/128 through the pre-WBD best route before capture routes" }
    foreach ($p in $DirectPrefix4) { Write-Output "01 DIRECT IPv4 $p through the pre-WBD physical route" }
    if ($addr4) { Write-Output "02 ADDRESS IPv4 $($addr4.CIDR) exclusively on $AdapterAlias and wait for DAD Preferred state" }
    if ($addr6) { Write-Output "02 ADDRESS IPv6 $($addr6.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    Write-Output "02 MTU $MTU already owned and verified by wbd-tun; route script does not mutate interface MTU"
    if ($DNSServers.Count -gt 0) { Write-Output "02 DNS NRPT namespace=. servers=$($DNSServers -join ',') capture_resolvers_through_wbd=1" }
    foreach ($p in $capture4) { Write-Output "03 CAPTURE IPv4 $p on $AdapterAlias" }
    foreach ($p in $capture6) { Write-Output "03 CAPTURE IPv6 $p on $AdapterAlias" }
    Write-Output '04 CLEANUP remove WBD NRPT rule and WBD-owned routes before reverse runtime teardown'
    exit 0
}

Require-Admin

if ($Action -eq 'Cleanup') {
    if (-not (Test-Path -LiteralPath $StatePath)) {
        Remove-StaleWBDNRPT
        Write-Output "WBD_WINDOWS_TUN_CLEAN state=absent stale_nrpt_removed=1"
        exit 0
    }
    $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
    Remove-Owned-State $state
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    Write-Output 'WBD_WINDOWS_TUN_CLEANUP_PASS'
    exit 0
}

# Crash/restart recovery is part of Apply rather than a manual prerequisite.
# The state file contains only WBD-owned objects, so replaying its inverse is
# precise and safe. A previously deleted NRPT rule is explicitly idempotent.
if (Test-Path -LiteralPath $StatePath) {
    $staleState = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
    Remove-Owned-State $staleState
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction Stop
    Write-Output 'WBD_WINDOWS_TUN_STALE_STATE_RECOVERED'
}
# Crash recovery for the narrow interval after NRPT creation but before its rule
# id can be persisted. Only the exact WBD display/comment pair is removed.
Remove-StaleWBDNRPT

$adapter = Wait-NetAdapterByName -Name $AdapterAlias
$ifIndex = [uint32]$adapter.ifIndex
Write-Output "WBD_WINDOWS_TUN_ADAPTER_READY adapter=$AdapterAlias ifindex=$ifIndex"
if ($CaptureLAN) {
    $physicalLAN = @(Get-PhysicalRFC1918RoutePrefixes -TunnelInterfaceIndex $ifIndex)
    $capture4 = @($capture4 + $physicalLAN | Select-Object -Unique)
    Write-Output "WBD_WINDOWS_TUN_LAN_CAPTURE_PLAN physical_prefixes=$($physicalLAN.Count) total_capture4=$($capture4.Count)"
}

# wbd-tun already owns and verifies the Wintun IPv4 MTU before emitting
# WBD_TUN_READY. Do not call Get/Set-NetIPInterface here: on fresh Wintun
# adapters the NetTCPIP provider can block while rows are materializing, which
# used to stall every route-capture mode before route-state was even persisted.
# Route application owns only addresses, routes and NRPT state.
$state = [ordered]@{
    Schema = 'wbd-windows-route-state/v4'
    AdapterAlias = $AdapterAlias
    AdapterInterfaceIndex = $ifIndex
    PhysicalInterfaceIndex = [uint32]0
    PhysicalNextHop4 = ''
    DNSConfigured = $false
    NRPTRuleName = ''
    Addresses = @()
    UnderlayRoutes = @()
    DirectRoutes = @()
    CaptureRoutes = @()
}
Save-State $state
Write-Output "WBD_WINDOWS_TUN_ROUTE_STATE_READY path=$StatePath ifindex=$ifIndex"

try {
    $underlayRoute4 = $null
    foreach ($item in @(@{IP=$Underlay4; Prefix=if ($Underlay4) { "$Underlay4/32" } else { '' }; Family='IPv4'}, @{IP=$Underlay6; Prefix=if ($Underlay6) { "$Underlay6/128" } else { '' }; Family='IPv6'})) {
        if (-not $item.IP) { continue }
        $found = @(Find-NetRoute -RemoteIPAddress $item.IP)
        $route = $found | Where-Object { $_.PSObject.Properties.Name -contains 'NextHop' } | Select-Object -First 1
        if (-not $route) { throw "no pre-WBD route found for underlay $($item.IP)" }
        if ([uint32]$route.InterfaceIndex -eq $ifIndex) { throw "underlay $($item.IP) already resolves through Wintun; refusing recursive capture" }
        if ($item.Family -eq 'IPv4') { $underlayRoute4 = $route }
        $existing = Get-NetRoute -DestinationPrefix $item.Prefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop ([string]$route.NextHop) -PolicyStore ActiveStore -ErrorAction SilentlyContinue
        if (-not $existing) {
            $state.UnderlayRoutes += [ordered]@{ DestinationPrefix=$item.Prefix; InterfaceIndex=[uint32]$route.InterfaceIndex; NextHop=[string]$route.NextHop }
            Save-State $state
            New-NetRoute -DestinationPrefix $item.Prefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop ([string]$route.NextHop) -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
        }
    }

    if ($underlayRoute4) {
        # Current physical-path identity is runtime metadata, not cleanup
        # ownership. Persist it even when the exact server /32 already existed
        # and therefore was deliberately not added to UnderlayRoutes.
        $state.PhysicalInterfaceIndex = [uint32]$underlayRoute4.InterfaceIndex
        $state.PhysicalNextHop4 = [string]$underlayRoute4.NextHop
        Save-State $state
    }

    Write-Output "WBD_WINDOWS_TUN_UNDERLAY_ROUTES_READY underlay4=$([int](-not [string]::IsNullOrWhiteSpace($Underlay4))) underlay6=$([int](-not [string]::IsNullOrWhiteSpace($Underlay6)))"

    if ($DirectPrefix4.Count -gt 0 -or $CaptureLAN) {
        if (-not $underlayRoute4) { throw 'pre-WBD IPv4 route is unavailable for direct-prefix/LAN routing' }
        $directCreate = [System.Collections.Generic.List[object]]::new()
        # Query the physical route table once. CN split mode can contain thousands
        # of prefixes; one Get-NetRoute CIM call per prefix made startup scale
        # catastrophically and could exceed the product readiness timeout.
        $existingDirect = @{}
        foreach ($existingRoute in @(Get-NetRoute -AddressFamily IPv4 -InterfaceIndex ([uint32]$underlayRoute4.InterfaceIndex) -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
            $key = "$([string]$existingRoute.DestinationPrefix)|$([string]$existingRoute.NextHop)"
            $existingDirect[$key] = $true
        }
        $physicalNextHop4 = [string]$underlayRoute4.NextHop
        foreach ($prefix in $DirectPrefix4) {
            $key = "$prefix|$physicalNextHop4"
            if (-not $existingDirect.ContainsKey($key)) {
                [void]$directCreate.Add([ordered]@{ DestinationPrefix=$prefix; InterfaceIndex=[uint32]$underlayRoute4.InterfaceIndex; NextHop=$physicalNextHop4 })
            }
        }
        if ($CaptureLAN -and $underlayRoute4 -and (Test-RFC1918Address $physicalNextHop4)) {
            $gatewayPrefix = "$physicalNextHop4/32"
            $key = "$gatewayPrefix|$physicalNextHop4"
            if (-not $existingDirect.ContainsKey($key)) {
                [void]$directCreate.Add([ordered]@{ DestinationPrefix=$gatewayPrefix; InterfaceIndex=[uint32]$underlayRoute4.InterfaceIndex; NextHop=$physicalNextHop4 })
            }
        }
        $state.DirectRoutes = @($directCreate)
        Save-State $state
        Write-Output "WBD_WINDOWS_TUN_DIRECT_ROUTES_PLAN total=$($directCreate.Count)"
        $directDone = 0
        foreach ($route in $directCreate) {
            New-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
            $directDone++
            if (($directDone % 500) -eq 0) { Write-Output "WBD_WINDOWS_TUN_DIRECT_ROUTES_PROGRESS done=$directDone total=$($directCreate.Count)" }
        }
        Write-Output "WBD_WINDOWS_TUN_DIRECT_ROUTES_READY total=$directDone"
    }

    foreach ($a in @(@{Parsed=$addr4; Family='IPv4'}, @{Parsed=$addr6; Family='IPv6'})) {
        if (-not $a.Parsed) { continue }
        $existing = Get-NetIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -ErrorAction SilentlyContinue
        if (-not $existing) {
            New-NetIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -PrefixLength $a.Parsed.PrefixLength -AddressFamily $a.Family -SkipAsSource $false | Out-Null
            $state.Addresses += [ordered]@{ InterfaceIndex=$ifIndex; IPAddress=$a.Parsed.IP }
            Save-State $state
        }
        Wait-PreferredIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -Family $a.Family
    }
    if ($addr4) {
        Set-ExclusiveTunnelIPv4 -InterfaceIndex $ifIndex -IPAddress $addr4.IP
    }
    Write-Output "WBD_WINDOWS_TUN_ADDRESSES_READY ipv4=$([int]($null -ne $addr4)) ipv6=$([int]($null -ne $addr6))"

    # Plan and persist all WBD-owned capture routes before creating the batch so
    # cleanup after a partial New-NetRoute failure is complete and never removes
    # pre-existing user routes.
    $captureCreate = [System.Collections.Generic.List[object]]::new()
    # Same rule for Wintun capture routes: snapshot once, compare in memory, then
    # create only WBD-owned missing entries. Avoid thousands of CIM round trips.
    $existingCapture = @{}
    foreach ($existingRoute in @(Get-NetRoute -InterfaceIndex $ifIndex -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
        $key = "$([string]$existingRoute.DestinationPrefix)|$([string]$existingRoute.NextHop)"
        $existingCapture[$key] = $true
    }
    foreach ($item in @(@{Family='IPv4'; Prefixes=$capture4; NextHop='0.0.0.0'}, @{Family='IPv6'; Prefixes=$capture6; NextHop='::'})) {
        foreach ($prefix in @($item.Prefixes)) {
            $key = "$prefix|$($item.NextHop)"
            if (-not $existingCapture.ContainsKey($key)) {
                [void]$captureCreate.Add([ordered]@{ DestinationPrefix=$prefix; InterfaceIndex=$ifIndex; NextHop=$item.NextHop })
            }
        }
    }
    $state.CaptureRoutes = @($captureCreate)
    Save-State $state
    Write-Output "WBD_WINDOWS_TUN_CAPTURE_ROUTES_PLAN total=$($captureCreate.Count)"
    $captureDone = 0
    foreach ($route in $captureCreate) {
        New-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -RouteMetric 5 -PolicyStore ActiveStore | Out-Null
        $captureDone++
        if (($captureDone % 500) -eq 0) { Write-Output "WBD_WINDOWS_TUN_CAPTURE_ROUTES_PROGRESS done=$captureDone total=$($captureCreate.Count)" }
    }
    Write-Output "WBD_WINDOWS_TUN_CAPTURE_ROUTES_READY total=$captureDone"

    # Install the Any-namespace NRPT rule only after each DNS resolver /32 is
    # already captured through WBD. Existing adapter DNS settings are untouched.
    if ($DNSServers.Count -gt 0) {
        foreach ($cmd in @('Add-DnsClientNrptRule','Get-DnsClientNrptRule','Remove-DnsClientNrptRule')) {
            if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "$cmd is unavailable" }
        }
        $rule = Add-DnsClientNrptRule -Namespace '.' -NameServers $DNSServers -DisplayName $NRPTDisplayName -Comment $NRPTComment -PassThru -ErrorAction Stop
        if (-not $rule -or -not $rule.Name) { throw 'NRPT rule creation returned no rule id' }
        $state.DNSConfigured = $true
        $state.NRPTRuleName = [string]$rule.Name
        Save-State $state
    }

    Write-Output "WBD_WINDOWS_TUN_READY mode=$Mode adapter=$AdapterAlias ifindex=$ifIndex mtu=$MTU direct4=$($DirectPrefix4.Count) capture4=$($capture4.Count) capture_lan=$([int]$CaptureLAN.IsPresent) dns=$($DNSServers.Count)"
    if ($Underlay4) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY4_LOCKED $Underlay4" }
    if ($Underlay6) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY6_LOCKED $Underlay6" }
    if ($DNSServers.Count -gt 0) { Write-Output "WBD_WINDOWS_DNS_READY mode=nrpt namespace=. servers=$($DNSServers -join ',') via_wbd=1 rule=$($state.NRPTRuleName)" }
} catch {
    try { Remove-Owned-State ([pscustomobject]$state) } catch { }
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    throw
}