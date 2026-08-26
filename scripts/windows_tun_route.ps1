param(
    [ValidateSet('Render','Apply','Cleanup')]
    [string]$Action = 'Render',
    [string]$AdapterAlias = 'WBD',
    [ValidateSet('Full','Split')]
    [string]$Mode = 'Full',
    [string]$TunnelAddress4 = '10.66.0.2/30',
    [string]$TunnelAddress6 = '',
    [string]$Underlay4 = '',
    [string]$Underlay6 = '',
    [string[]]$Prefix4 = @(),
    [string[]]$Prefix6 = @(),
    [ValidateRange(576,9000)]
    [int]$MTU = 1400,
    [string]$StatePath = "$env:ProgramData\WBD\windows-route-state.json"
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

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
        if ($adapter) {
            Write-Output "WBD_WINDOWS_TUN_ADAPTER_READY adapter=$Name ifindex=$($adapter.ifIndex)"
            return $adapter
        }
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

function Save-State($State) {
    $dir = Split-Path -Parent $StatePath
    if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $State | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $StatePath -Encoding UTF8
}

function Remove-Owned-State($State) {
    foreach ($route in @($State.CaptureRoutes)) {
        Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
    }
    foreach ($route in @($State.UnderlayRoutes)) {
        Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
    }
    foreach ($addr in @($State.Addresses)) {
        Remove-NetIPAddress -InterfaceIndex ([uint32]$addr.InterfaceIndex) -IPAddress $addr.IPAddress -Confirm:$false -ErrorAction SilentlyContinue
    }
    if ($State.PSObject.Properties.Name -contains 'MTU4' -and $null -ne $State.MTU4) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -NlMtuBytes ([uint32]$State.MTU4) -ErrorAction SilentlyContinue
    }
    if ($State.PSObject.Properties.Name -contains 'MTU6' -and $null -ne $State.MTU6) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv6 -NlMtuBytes ([uint32]$State.MTU6) -ErrorAction SilentlyContinue
    }
}

$addr4 = Parse-CIDR $TunnelAddress4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'TunnelAddress4'
$addr6 = Parse-CIDR $TunnelAddress6 ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'TunnelAddress6'
Assert-IP $Underlay4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'Underlay4'
Assert-IP $Underlay6 ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'Underlay6'
foreach ($p in $Prefix4) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetwork) 'Prefix4') }
foreach ($p in $Prefix6) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'Prefix6') }

if ($Mode -eq 'Full') {
    $capture4 = @('0.0.0.0/1','128.0.0.0/1')
    $capture6 = if ($addr6) { @('::/1','8000::/1') } else { @() }
} else {
    $capture4 = @($Prefix4)
    $capture6 = @($Prefix6)
    if ($capture4.Count -eq 0 -and $capture6.Count -eq 0) { throw 'Split mode requires Prefix4 and/or Prefix6' }
}

if ($Action -eq 'Render') {
    Write-Output "WBD_WINDOWS_TUN_PLAN mode=$Mode adapter=$AdapterAlias mtu=$MTU"
    if ($Underlay4) { Write-Output "01 ESCAPE IPv4 $Underlay4/32 through the pre-WBD best route before capture routes" }
    if ($Underlay6) { Write-Output "01 ESCAPE IPv6 $Underlay6/128 through the pre-WBD best route before capture routes" }
    if ($addr4) { Write-Output "02 ADDRESS IPv4 $($addr4.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    if ($addr6) { Write-Output "02 ADDRESS IPv6 $($addr6.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    Write-Output "02 MTU $MTU on $AdapterAlias"
    foreach ($p in $capture4) { Write-Output "03 CAPTURE IPv4 $p on $AdapterAlias" }
    foreach ($p in $capture6) { Write-Output "03 CAPTURE IPv6 $p on $AdapterAlias" }
    Write-Output '04 CLEANUP only WBD-owned routes/addresses and restore prior MTU'
    exit 0
}

Require-Admin

if ($Action -eq 'Cleanup') {
    if (-not (Test-Path -LiteralPath $StatePath)) {
        Write-Output "WBD_WINDOWS_TUN_CLEAN state=absent"
        exit 0
    }
    $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
    Remove-Owned-State $state
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    Write-Output 'WBD_WINDOWS_TUN_CLEANUP_PASS'
    exit 0
}

if (Test-Path -LiteralPath $StatePath) {
    throw "state already exists; run Cleanup first: $StatePath"
}

# wbd-tun creates/opens Wintun asynchronously from the GUI controller's point
# of view. Wait for the adapter itself instead of relying on a fixed sleep so
# capture routes can never race process startup.
$adapter = Wait-NetAdapterByName -Name $AdapterAlias
$ifIndex = [uint32]$adapter.ifIndex
$ipif4 = Get-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
$ipif6 = Get-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv6 -ErrorAction SilentlyContinue | Select-Object -First 1
$state = [ordered]@{
    Schema = 'wbd-windows-route-state/v1'
    AdapterAlias = $AdapterAlias
    AdapterInterfaceIndex = $ifIndex
    MTU4 = if ($ipif4) { [uint32]$ipif4.NlMtu } else { $null }
    MTU6 = if ($ipif6) { [uint32]$ipif6.NlMtu } else { $null }
    Addresses = @()
    UnderlayRoutes = @()
    CaptureRoutes = @()
}
Save-State $state

try {
    # Lock transport underlay to the pre-WBD best route before adding any
    # broad capture route. This is the Windows equivalent of OpenWrt escape.
    foreach ($item in @(@{IP=$Underlay4; Prefix=if ($Underlay4) { "$Underlay4/32" } else { '' }}, @{IP=$Underlay6; Prefix=if ($Underlay6) { "$Underlay6/128" } else { '' }})) {
        if (-not $item.IP) { continue }
        $found = @(Find-NetRoute -RemoteIPAddress $item.IP)
        $route = $found | Where-Object { $_.PSObject.Properties.Name -contains 'NextHop' } | Select-Object -First 1
        if (-not $route) { throw "no pre-WBD route found for underlay $($item.IP)" }
        if ([uint32]$route.InterfaceIndex -eq $ifIndex) { throw "underlay $($item.IP) already resolves through Wintun; refusing recursive capture" }
        $existing = Get-NetRoute -DestinationPrefix $item.Prefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop ([string]$route.NextHop) -PolicyStore ActiveStore -ErrorAction SilentlyContinue
        if (-not $existing) {
            New-NetRoute -DestinationPrefix $item.Prefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop ([string]$route.NextHop) -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
            $state.UnderlayRoutes += [ordered]@{ DestinationPrefix=$item.Prefix; InterfaceIndex=[uint32]$route.InterfaceIndex; NextHop=[string]$route.NextHop }
            Save-State $state
        }
    }

    if ($ipif4) { Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -NlMtuBytes $MTU }
    if ($ipif6) { Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv6 -NlMtuBytes $MTU }

    foreach ($a in @(@{Parsed=$addr4; Family='IPv4'}, @{Parsed=$addr6; Family='IPv6'})) {
        if (-not $a.Parsed) { continue }
        $existing = Get-NetIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -ErrorAction SilentlyContinue
        if (-not $existing) {
            New-NetIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -PrefixLength $a.Parsed.PrefixLength -AddressFamily $a.Family -SkipAsSource $false | Out-Null
            $state.Addresses += [ordered]@{ InterfaceIndex=$ifIndex; IPAddress=$a.Parsed.IP }
            Save-State $state
        }
        # New-NetIPAddress can return while Windows Duplicate Address Detection
        # still marks the address Tentative. A Tentative address is not usable
        # for communication and immediate application traffic can fail with
        # PING "General failure" before a packet ever reaches Wintun. Declare
        # WBD routing ready only after the configured address is Preferred.
        Wait-PreferredIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -Family $a.Family
    }

    foreach ($item in @(@{Family='IPv4'; Prefixes=$capture4; NextHop='0.0.0.0'}, @{Family='IPv6'; Prefixes=$capture6; NextHop='::'})) {
        foreach ($prefix in @($item.Prefixes)) {
            $existing = Get-NetRoute -DestinationPrefix $prefix -InterfaceIndex $ifIndex -NextHop $item.NextHop -PolicyStore ActiveStore -ErrorAction SilentlyContinue
            if (-not $existing) {
                New-NetRoute -DestinationPrefix $prefix -InterfaceIndex $ifIndex -NextHop $item.NextHop -RouteMetric 5 -PolicyStore ActiveStore | Out-Null
                $state.CaptureRoutes += [ordered]@{ DestinationPrefix=$prefix; InterfaceIndex=$ifIndex; NextHop=$item.NextHop }
                Save-State $state
            }
        }
    }

    Write-Output "WBD_WINDOWS_TUN_READY mode=$Mode adapter=$AdapterAlias ifindex=$ifIndex mtu=$MTU"
    if ($Underlay4) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY4_LOCKED $Underlay4" }
    if ($Underlay6) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY6_LOCKED $Underlay6" }
} catch {
    try { Remove-Owned-State ([pscustomobject]$state) } catch { }
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    throw
}
