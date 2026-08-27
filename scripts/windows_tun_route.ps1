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
    [string]$PrefixFile4 = '',
    [string[]]$DirectPrefix4 = @(),
    [string]$DirectPrefixFile4 = '',
    [string[]]$DNSServer = @(),
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

function Save-State($State) {
    $dir = Split-Path -Parent $StatePath
    if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $State | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $StatePath -Encoding UTF8
}

function Remove-Owned-State($State) {
    # Capture/direct routes disappear before DNS/address/MTU restoration. This
    # preserves the frozen Exit rule: never leave broad traffic pointed at a
    # tunnel that is about to be stopped.
    if ($State.PSObject.Properties.Name -contains 'CaptureRoutes') {
        foreach ($route in @($State.CaptureRoutes)) {
            Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
        }
    }
    if ($State.PSObject.Properties.Name -contains 'DirectRoutes') {
        foreach ($route in @($State.DirectRoutes)) {
            Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
        }
    }
    if ($State.PSObject.Properties.Name -contains 'UnderlayRoutes') {
        foreach ($route in @($State.UnderlayRoutes)) {
            Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
        }
    }
    if ($State.PSObject.Properties.Name -contains 'DNSConfigured' -and $State.DNSConfigured) {
        $previous = @()
        if ($State.PSObject.Properties.Name -contains 'DNSPrevious') { $previous = @($State.DNSPrevious) }
        if ($previous.Count -gt 0) {
            Set-DnsClientServerAddress -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -ServerAddresses $previous -ErrorAction SilentlyContinue
        } else {
            Set-DnsClientServerAddress -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -ResetServerAddresses -ErrorAction SilentlyContinue
        }
    }
    if ($State.PSObject.Properties.Name -contains 'Addresses') {
        foreach ($addr in @($State.Addresses)) {
            Remove-NetIPAddress -InterfaceIndex ([uint32]$addr.InterfaceIndex) -IPAddress $addr.IPAddress -Confirm:$false -ErrorAction SilentlyContinue
        }
    }
    if ($State.PSObject.Properties.Name -contains 'MTU4' -and $null -ne $State.MTU4) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -NlMtuBytes ([uint32]$State.MTU4) -ErrorAction SilentlyContinue
    }
    if ($State.PSObject.Properties.Name -contains 'MTU6' -and $null -ne $State.MTU6) {
        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv6 -NlMtuBytes ([uint32]$State.MTU6) -ErrorAction SilentlyContinue
    }
}

$Prefix4 = @($Prefix4) + @(Read-PrefixFile $PrefixFile4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'PrefixFile4')
$DirectPrefix4 = @($DirectPrefix4) + @(Read-PrefixFile $DirectPrefixFile4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'DirectPrefixFile4')
$Prefix4 = @($Prefix4 | Select-Object -Unique)
$DirectPrefix4 = @($DirectPrefix4 | Select-Object -Unique)

$addr4 = Parse-CIDR $TunnelAddress4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'TunnelAddress4'
$addr6 = Parse-CIDR $TunnelAddress6 ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'TunnelAddress6'
Assert-IP $Underlay4 ([System.Net.Sockets.AddressFamily]::InterNetwork) 'Underlay4'
Assert-IP $Underlay6 ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'Underlay6'
foreach ($p in $Prefix4) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetwork) 'Prefix4') }
foreach ($p in $Prefix6) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetworkV6) 'Prefix6') }
foreach ($p in $DirectPrefix4) { [void](Parse-CIDR $p ([System.Net.Sockets.AddressFamily]::InterNetwork) 'DirectPrefix4') }
foreach ($dns in $DNSServer) { Assert-IP $dns ([System.Net.Sockets.AddressFamily]::InterNetwork) 'DNSServer' }
if ($DirectPrefix4.Count -gt 0 -and -not $Underlay4) { throw 'DirectPrefix4 requires Underlay4 so the pre-WBD physical route is known' }

if ($Mode -eq 'Full') {
    $capture4 = @('0.0.0.0/1','128.0.0.0/1')
    $capture6 = if ($addr6) { @('::/1','8000::/1') } else { @() }
} else {
    $capture4 = @($Prefix4)
    $capture6 = @($Prefix6)
    if ($capture4.Count -eq 0 -and $capture6.Count -eq 0 -and $DNSServer.Count -eq 0) { throw 'Split mode requires Prefix4/Prefix6 and/or DNSServer' }
}
# Any explicitly configured DNS upstream is forced through WBD even when the
# selected split policy would otherwise send that address direct.
$capture4 = @($capture4) + @($DNSServer | ForEach-Object { "$_/32" })
$capture4 = @($capture4 | Select-Object -Unique)

if ($Action -eq 'Render') {
    Write-Output "WBD_WINDOWS_TUN_PLAN mode=$Mode adapter=$AdapterAlias mtu=$MTU"
    if ($Underlay4) { Write-Output "01 ESCAPE IPv4 $Underlay4/32 through the pre-WBD best route before capture routes" }
    if ($Underlay6) { Write-Output "01 ESCAPE IPv6 $Underlay6/128 through the pre-WBD best route before capture routes" }
    foreach ($p in $DirectPrefix4) { Write-Output "01 DIRECT IPv4 $p through the pre-WBD physical route" }
    if ($addr4) { Write-Output "02 ADDRESS IPv4 $($addr4.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    if ($addr6) { Write-Output "02 ADDRESS IPv6 $($addr6.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    Write-Output "02 MTU $MTU on $AdapterAlias"
    foreach ($dns in $DNSServer) { Write-Output "02 DNS IPv4 $dns on $AdapterAlias and capture its /32 through WBD" }
    foreach ($p in $capture4) { Write-Output "03 CAPTURE IPv4 $p on $AdapterAlias" }
    foreach ($p in $capture6) { Write-Output "03 CAPTURE IPv6 $p on $AdapterAlias" }
    Write-Output '04 CLEANUP WBD-owned routes first, then restore DNS/addresses/MTU'
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

$adapter = Wait-NetAdapterByName -Name $AdapterAlias
$ifIndex = [uint32]$adapter.ifIndex
Write-Output "WBD_WINDOWS_TUN_ADAPTER_READY adapter=$AdapterAlias ifindex=$ifIndex"
$ipif4 = Get-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
$ipif6 = Get-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv6 -ErrorAction SilentlyContinue | Select-Object -First 1
$state = [ordered]@{
    Schema = 'wbd-windows-route-state/v2'
    AdapterAlias = $AdapterAlias
    AdapterInterfaceIndex = $ifIndex
    MTU4 = if ($ipif4) { [uint32]$ipif4.NlMtu } else { $null }
    MTU6 = if ($ipif6) { [uint32]$ipif6.NlMtu } else { $null }
    DNSConfigured = $false
    DNSPrevious = @()
    Addresses = @()
    UnderlayRoutes = @()
    DirectRoutes = @()
    CaptureRoutes = @()
}
Save-State $state

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

    if ($DirectPrefix4.Count -gt 0) {
        if (-not $underlayRoute4) { throw 'pre-WBD IPv4 route is unavailable for direct-prefix routing' }
        $directCreate = @()
        foreach ($prefix in $DirectPrefix4) {
            $existing = Get-NetRoute -DestinationPrefix $prefix -InterfaceIndex ([uint32]$underlayRoute4.InterfaceIndex) -NextHop ([string]$underlayRoute4.NextHop) -PolicyStore ActiveStore -ErrorAction SilentlyContinue
            if (-not $existing) {
                $directCreate += [ordered]@{ DestinationPrefix=$prefix; InterfaceIndex=[uint32]$underlayRoute4.InterfaceIndex; NextHop=[string]$underlayRoute4.NextHop }
            }
        }
        $state.DirectRoutes = @($directCreate)
        Save-State $state
        foreach ($route in $directCreate) {
            New-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
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
        Wait-PreferredIPAddress -InterfaceIndex $ifIndex -IPAddress $a.Parsed.IP -Family $a.Family
    }

    if ($DNSServer.Count -gt 0) {
        if (-not (Get-Command Set-DnsClientServerAddress -ErrorAction SilentlyContinue)) { throw 'Set-DnsClientServerAddress is unavailable' }
        $dnsPrevious = Get-DnsClientServerAddress -InterfaceIndex $ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
        $state.DNSConfigured = $true
        $state.DNSPrevious = if ($dnsPrevious) { @($dnsPrevious.ServerAddresses) } else { @() }
        Save-State $state
        Set-DnsClientServerAddress -InterfaceIndex $ifIndex -ServerAddresses $DNSServer
    }

    $captureCreate = @()
    foreach ($item in @(@{Family='IPv4'; Prefixes=$capture4; NextHop='0.0.0.0'}, @{Family='IPv6'; Prefixes=$capture6; NextHop='::'})) {
        foreach ($prefix in @($item.Prefixes)) {
            $existing = Get-NetRoute -DestinationPrefix $prefix -InterfaceIndex $ifIndex -NextHop $item.NextHop -PolicyStore ActiveStore -ErrorAction SilentlyContinue
            if (-not $existing) {
                $captureCreate += [ordered]@{ DestinationPrefix=$prefix; InterfaceIndex=$ifIndex; NextHop=$item.NextHop }
            }
        }
    }
    $state.CaptureRoutes = @($captureCreate)
    Save-State $state
    foreach ($route in $captureCreate) {
        New-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -RouteMetric 5 -PolicyStore ActiveStore | Out-Null
    }

    Write-Output "WBD_WINDOWS_TUN_READY mode=$Mode adapter=$AdapterAlias ifindex=$ifIndex mtu=$MTU direct4=$($DirectPrefix4.Count) capture4=$($capture4.Count) dns=$($DNSServer.Count)"
    if ($Underlay4) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY4_LOCKED $Underlay4" }
    if ($Underlay6) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY6_LOCKED $Underlay6" }
    if ($DNSServer.Count -gt 0) { Write-Output "WBD_WINDOWS_DNS_READY servers=$($DNSServer -join ',') via_wbd=1" }
} catch {
    try { Remove-Owned-State ([pscustomobject]$state) } catch { }
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    throw
}
