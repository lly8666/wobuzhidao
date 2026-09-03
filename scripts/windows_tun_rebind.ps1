param(
    [Parameter(Mandatory=$true)]
    [string]$Underlay4,
    [Parameter(Mandatory=$true)]
    [uint32]$ExpectedPhysicalInterfaceIndex,
    [Parameter(Mandatory=$true)]
    [string]$ExpectedPhysicalNextHop4,
    [string[]]$DirectPrefix4 = @(),
    [string]$DirectPrefixFile4 = '',
    [Parameter(Mandatory=$true)]
    [string]$StatePath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-IPv4([string]$Value, [string]$Label) {
    $ip = $null
    if (-not [System.Net.IPAddress]::TryParse($Value, [ref]$ip) -or $ip.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
        throw "$Label must be a valid IPv4 address: $Value"
    }
    return $ip.ToString()
}

function Parse-IPv4CIDR([string]$CIDR, [string]$Label) {
    $parts = $CIDR.Split('/')
    if ($parts.Count -ne 2) { throw "$Label must be CIDR: $CIDR" }
    [void](Assert-IPv4 $parts[0] $Label)
    $prefix = 0
    if (-not [int]::TryParse($parts[1], [ref]$prefix) -or $prefix -lt 0 -or $prefix -gt 32) {
        throw "$Label has invalid prefix: $CIDR"
    }
    return $CIDR
}

function Read-PrefixFile([string]$Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) { return }
    if (-not (Test-Path -LiteralPath $Path)) { throw "DirectPrefixFile4 not found: $Path" }
    foreach ($raw in Get-Content -LiteralPath $Path) {
        $line = ([string]$raw).Trim()
        if (-not $line -or $line.StartsWith('#')) { continue }
        [void](Parse-IPv4CIDR $line 'DirectPrefixFile4')
        Write-Output $line
    }
}

function Require-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'route rebind requires an elevated PowerShell session'
    }
}

function Save-State($State) {
    $dir = Split-Path -Parent $StatePath
    if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $json = $State | ConvertTo-Json -Depth 10
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($StatePath, $json + [Environment]::NewLine, $utf8NoBom)
}

function Route-Key($Route) {
    return ('{0}|{1}|{2}' -f ([string]$Route.DestinationPrefix).ToLowerInvariant(), [uint32]$Route.InterfaceIndex, ([string]$Route.NextHop).ToLowerInvariant())
}

function Route-Spec([string]$Prefix, [uint32]$InterfaceIndex, [string]$NextHop) {
    return [ordered]@{ DestinationPrefix=$Prefix; InterfaceIndex=$InterfaceIndex; NextHop=$NextHop }
}

function Route-Exists($Route) {
    $existing = @(Get-NetRoute -DestinationPrefix $Route.DestinationPrefix -InterfaceIndex ([uint32]$Route.InterfaceIndex) -NextHop $Route.NextHop -PolicyStore ActiveStore -ErrorAction SilentlyContinue)
    return $existing.Count -gt 0
}

function Merge-OwnedRoutes($First, $Second) {
    $seen = @{}
    $out = @()
    foreach ($route in @($First) + @($Second)) {
        $key = Route-Key $route
        if ($seen.ContainsKey($key)) { continue }
        $seen[$key] = $true
        $out += Route-Spec ([string]$route.DestinationPrefix) ([uint32]$route.InterfaceIndex) ([string]$route.NextHop)
    }
    return @($out)
}

Require-Admin
if (-not (Test-Path -LiteralPath $StatePath)) {
    throw "WBD route state is required for rebind: $StatePath"
}

$Underlay4 = Assert-IPv4 $Underlay4 'Underlay4'
$ExpectedPhysicalNextHop4 = Assert-IPv4 $ExpectedPhysicalNextHop4 'ExpectedPhysicalNextHop4'
if ($ExpectedPhysicalInterfaceIndex -eq 0) { throw 'ExpectedPhysicalInterfaceIndex must be non-zero' }

$DirectPrefix4 = @($DirectPrefix4) + @(Read-PrefixFile $DirectPrefixFile4)
$DirectPrefix4 = @($DirectPrefix4 | ForEach-Object { Parse-IPv4CIDR ([string]$_) 'DirectPrefix4' } | Select-Object -Unique)

$state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
foreach ($required in @('AdapterInterfaceIndex','UnderlayRoutes','DirectRoutes')) {
    if (-not ($state.PSObject.Properties.Name -contains $required)) {
        throw "WBD route state is missing $required"
    }
}
if ([uint32]$state.AdapterInterfaceIndex -eq $ExpectedPhysicalInterfaceIndex) {
    throw 'refusing to rebind physical routes through the WBD tunnel adapter'
}

# Reuse the same connected-state physical-path authority as lane migration. It
# excludes WBD's existing pinned escape route and Wintun capture routes. The
# expected values fence the small observation->mutation window: if the default
# route changes again, leave old WBD-owned routes intact and let the controller
# retry from a fresh observation.
$underlayScript = Join-Path $PSScriptRoot 'windows_faketcp_underlay.ps1'
$monitorOutput = @(& $underlayScript -RemoteIPAddress $Underlay4 -MonitorPhysicalPath -StatePath $StatePath)
$jsonLine = $monitorOutput | Where-Object { ([string]$_).Trim().StartsWith('{') } | Select-Object -First 1
if (-not $jsonLine) { throw 'physical-path monitor returned no JSON during route rebind' }
$observed = ([string]$jsonLine) | ConvertFrom-Json
$observedIfIndex = [uint32]$observed.interface_index
$observedNextHop = Assert-IPv4 ([string]$observed.next_hop_ip) 'observed next_hop_ip'
if ($observedIfIndex -ne $ExpectedPhysicalInterfaceIndex -or $observedNextHop -ne $ExpectedPhysicalNextHop4) {
    throw "physical path changed during route rebind: expected_if=$ExpectedPhysicalInterfaceIndex expected_next_hop=$ExpectedPhysicalNextHop4 observed_if=$observedIfIndex observed_next_hop=$observedNextHop"
}

$oldUnderlay = @($state.UnderlayRoutes)
$oldDirect = @($state.DirectRoutes)
$oldOwned = @{}
foreach ($route in @($oldUnderlay) + @($oldDirect)) { $oldOwned[(Route-Key $route)] = $true }

$newUnderlayOwned = @()
$newDirectOwned = @()
$createUnderlay = @()
$createDirect = @()
$underlayPrefix = "$Underlay4/32"

# Preserve any non-IPv4-underlay ownership (currently normally empty) without
# mutating it. The IPv4 server escape route is always ensured on the observed
# physical path so WBD capture routes cannot recursively capture the server.
foreach ($route in $oldUnderlay) {
    if ([string]$route.DestinationPrefix -ne $underlayPrefix) {
        $newUnderlayOwned += Route-Spec ([string]$route.DestinationPrefix) ([uint32]$route.InterfaceIndex) ([string]$route.NextHop)
    }
}
$underlayTarget = Route-Spec $underlayPrefix $ExpectedPhysicalInterfaceIndex $ExpectedPhysicalNextHop4
$underlayKey = Route-Key $underlayTarget
if ($oldOwned.ContainsKey($underlayKey)) {
    $newUnderlayOwned += $underlayTarget
} elseif (-not (Route-Exists $underlayTarget)) {
    $newUnderlayOwned += $underlayTarget
    $createUnderlay += $underlayTarget
}

foreach ($prefix in $DirectPrefix4) {
    $target = Route-Spec ([string]$prefix) $ExpectedPhysicalInterfaceIndex $ExpectedPhysicalNextHop4
    $key = Route-Key $target
    if ($oldOwned.ContainsKey($key)) {
        $newDirectOwned += $target
    } elseif (-not (Route-Exists $target)) {
        $newDirectOwned += $target
        $createDirect += $target
    }
    # If the exact target route already exists but WBD did not create it, leave
    # it user/system-owned and deliberately omit it from cleanup ownership.
}

# Persist the union before creating anything. Existing Cleanup therefore knows
# every old route plus every route this transaction may create, including after
# a crash in the middle of New-NetRoute or old-route retirement.
$state.UnderlayRoutes = @(Merge-OwnedRoutes $oldUnderlay $createUnderlay)
$state.DirectRoutes = @(Merge-OwnedRoutes $oldDirect $createDirect)
Save-State $state

$created = @()
try {
    foreach ($route in @($createUnderlay) + @($createDirect)) {
        New-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -RouteMetric 1 -PolicyStore ActiveStore | Out-Null
        $created += $route
    }
} catch {
    foreach ($route in $created) {
        Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue
    }
    $state.UnderlayRoutes = @($oldUnderlay)
    $state.DirectRoutes = @($oldDirect)
    Save-State $state
    throw
}

$newOwned = @{}
foreach ($route in @($newUnderlayOwned) + @($newDirectOwned)) { $newOwned[(Route-Key $route)] = $true }
$retired = 0
foreach ($route in @($oldUnderlay) + @($oldDirect)) {
    $key = Route-Key $route
    if ($newOwned.ContainsKey($key)) { continue }
    if (Route-Exists $route) {
        Remove-NetRoute -DestinationPrefix $route.DestinationPrefix -InterfaceIndex ([uint32]$route.InterfaceIndex) -NextHop $route.NextHop -PolicyStore ActiveStore -Confirm:$false -ErrorAction Stop
    }
    $retired++
}

$state.UnderlayRoutes = @($newUnderlayOwned)
$state.DirectRoutes = @($newDirectOwned)
Save-State $state
Write-Output "WBD_WINDOWS_TUN_REBIND_PASS underlay4=$Underlay4 ifindex=$ExpectedPhysicalInterfaceIndex next_hop=$ExpectedPhysicalNextHop4 direct4=$($DirectPrefix4.Count) created=$($created.Count) retired=$retired"
