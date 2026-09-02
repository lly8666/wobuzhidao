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
    # Comma/semicolon separated by design: powershell.exe -File is launched by
    # the Go controller and scalar CLI transport is deterministic across Windows
    # PowerShell versions, unlike repeated external string[] argument binding.
    [string]$DNSServer = '',
    [ValidateRange(576,9000)]
    [int]$MTU = 1400,
    [string]$StatePath = "$env:ProgramData\WBD\windows-route-state.json"
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
} else {
    $capture4 = @($Prefix4)
    $capture6 = @($Prefix6)
    if ($capture4.Count -eq 0 -and $capture6.Count -eq 0 -and $DNSServers.Count -eq 0) { throw 'Split mode requires Prefix4/Prefix6 and/or DNSServer' }
}
# Every configured DNS upstream is explicitly captured through WBD, even when a
# more-specific domestic direct route would otherwise match it.
$capture4 = @($capture4) + @($DNSServers | ForEach-Object { "$_/32" })
$capture4 = @($capture4 | Select-Object -Unique)

if ($Action -eq 'Render') {
    Write-Output "WBD_WINDOWS_TUN_PLAN mode=$Mode adapter=$AdapterAlias mtu=$MTU"
    if ($Underlay4) { Write-Output "01 ESCAPE IPv4 $Underlay4/32 through the pre-WBD best route before capture routes" }
    if ($Underlay6) { Write-Output "01 ESCAPE IPv6 $Underlay6/128 through the pre-WBD best route before capture routes" }
    foreach ($p in $DirectPrefix4) { Write-Output "01 DIRECT IPv4 $p through the pre-WBD physical route" }
    if ($addr4) { Write-Output "02 ADDRESS IPv4 $($addr4.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    if ($addr6) { Write-Output "02 ADDRESS IPv6 $($addr6.CIDR) on $AdapterAlias and wait for DAD Preferred state" }
    Write-Output "02 MTU $MTU on $AdapterAlias"
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
$ipif4 = Get-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
$ipif6 = Get-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv6 -ErrorAction SilentlyContinue | Select-Object -First 1
$state = [ordered]@{
    Schema = 'wbd-windows-route-state/v3'
    AdapterAlias = $AdapterAlias
    AdapterInterfaceIndex = $ifIndex
    MTU4 = if ($ipif4) { [uint32]$ipif4.NlMtu } else { $null }
    MTU6 = if ($ipif6) { [uint32]$ipif6.NlMtu } else { $null }
    DNSConfigured = $false
    NRPTRuleName = ''
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

    # Plan and persist all WBD-owned capture routes before creating the batch so
    # cleanup after a partial New-NetRoute failure is complete and never removes
    # pre-existing user routes.
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

    Write-Output "WBD_WINDOWS_TUN_READY mode=$Mode adapter=$AdapterAlias ifindex=$ifIndex mtu=$MTU direct4=$($DirectPrefix4.Count) capture4=$($capture4.Count) dns=$($DNSServers.Count)"
    if ($Underlay4) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY4_LOCKED $Underlay4" }
    if ($Underlay6) { Write-Output "WBD_WINDOWS_TUN_UNDERLAY6_LOCKED $Underlay6" }
    if ($DNSServers.Count -gt 0) { Write-Output "WBD_WINDOWS_DNS_READY mode=nrpt namespace=. servers=$($DNSServers -join ',') via_wbd=1 rule=$($state.NRPTRuleName)" }
} catch {
    try { Remove-Owned-State ([pscustomobject]$state) } catch { }
    Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
    throw
}
