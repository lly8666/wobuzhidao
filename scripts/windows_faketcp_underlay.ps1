param(
    [Parameter(Mandatory=$true)]
    [string]$RemoteIPAddress,
    [switch]$MonitorPhysicalPath,
    [string]$StatePath = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$remote = $null
if (-not [System.Net.IPAddress]::TryParse($RemoteIPAddress, [ref]$remote) -or $remote.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
    throw "RemoteIPAddress must be IPv4: $RemoteIPAddress"
}

function Test-IPv4InPrefix([string]$Address, [string]$Prefix) {
    $parts = $Prefix.Split('/')
    if ($parts.Count -ne 2) { return $false }
    $bits = 0
    if (-not [int]::TryParse($parts[1], [ref]$bits) -or $bits -lt 0 -or $bits -gt 32) { return $false }
    $addressIP = $null
    $networkIP = $null
    if (-not [System.Net.IPAddress]::TryParse($Address, [ref]$addressIP)) { return $false }
    if (-not [System.Net.IPAddress]::TryParse($parts[0], [ref]$networkIP)) { return $false }
    $addressBytes = $addressIP.GetAddressBytes()
    $networkBytes = $networkIP.GetAddressBytes()
    for ($i = 0; $i -lt 4; $i++) {
        $remaining = $bits - ($i * 8)
        if ($remaining -le 0) { break }
        $mask = if ($remaining -ge 8) { 255 } else { 256 - [int][math]::Pow(2, 8 - $remaining) }
        if (($addressBytes[$i] -band $mask) -ne ($networkBytes[$i] -band $mask)) { return $false }
    }
    return $true
}

function Select-PhysicalRoute([string]$Remote, [string]$RouteStatePath) {
    if ([string]::IsNullOrWhiteSpace($RouteStatePath) -or -not (Test-Path -LiteralPath $RouteStatePath)) {
        throw 'connected physical-path monitoring requires the WBD route state file'
    }
    $state = Get-Content -LiteralPath $RouteStatePath -Raw | ConvertFrom-Json
    $wbdIfIndex = if ($state.PSObject.Properties.Name -contains 'AdapterInterfaceIndex') { [uint32]$state.AdapterInterfaceIndex } else { [uint32]0 }
    $owned = @{}
    if ($state.PSObject.Properties.Name -contains 'UnderlayRoutes') {
        foreach ($item in @($state.UnderlayRoutes)) {
            $key = ('{0}|{1}|{2}' -f ([string]$item.DestinationPrefix).ToLowerInvariant(), [uint32]$item.InterfaceIndex, ([string]$item.NextHop).ToLowerInvariant())
            $owned[$key] = $true
        }
    }

    $candidates = @()
    foreach ($route in @(Get-NetRoute -AddressFamily IPv4 -PolicyStore ActiveStore -ErrorAction Stop)) {
        $prefix = [string]$route.DestinationPrefix
        if (-not (Test-IPv4InPrefix $Remote $prefix)) { continue }
        if ($wbdIfIndex -ne 0 -and [uint32]$route.InterfaceIndex -eq $wbdIfIndex) { continue }
        $key = ('{0}|{1}|{2}' -f $prefix.ToLowerInvariant(), [uint32]$route.InterfaceIndex, ([string]$route.NextHop).ToLowerInvariant())
        if ($owned.ContainsKey($key)) { continue }

        $ipif = Get-NetIPInterface -InterfaceIndex ([uint32]$route.InterfaceIndex) -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $ipif -or [string]$ipif.ConnectionState -ne 'Connected') { continue }
        $source = Get-NetIPAddress -InterfaceIndex ([uint32]$route.InterfaceIndex) -AddressFamily IPv4 -ErrorAction SilentlyContinue |
            Where-Object {
                [string]$_.AddressState -eq 'Preferred' -and
                -not [bool]$_.SkipAsSource -and
                -not ([string]$_.IPAddress).StartsWith('169.254.')
            } |
            Sort-Object PrefixLength -Descending |
            Select-Object -First 1
        if (-not $source) { continue }
        $prefixLength = [int]($prefix.Split('/')[1])
        $metric = [int64]$route.RouteMetric + [int64]$ipif.InterfaceMetric
        $candidates += [pscustomobject]@{
            Route = $route
            IP = $source
            PrefixLength = $prefixLength
            Metric = $metric
            InterfaceIndex = [uint32]$route.InterfaceIndex
        }
    }
    $selected = $candidates |
        Sort-Object @{Expression='PrefixLength'; Descending=$true}, @{Expression='Metric'; Ascending=$true}, @{Expression='InterfaceIndex'; Ascending=$true} |
        Select-Object -First 1
    if (-not $selected) {
        throw "no usable physical IPv4 route remains for $Remote after excluding WBD-owned capture/escape routes"
    }
    return $selected
}

if ($MonitorPhysicalPath) {
    $selected = Select-PhysicalRoute -Remote $RemoteIPAddress -RouteStatePath $StatePath
    $ip = $selected.IP
    $route = $selected.Route
} else {
    # Find-NetRoute intentionally returns TWO objects: the selected NetIPAddress
    # followed by the selected NetRoute. Do not Select-Object -First 1 and assume
    # route properties exist; Windows Server 2025 exposes NextHop only on the
    # NetRoute object.
    $found = @(Find-NetRoute -RemoteIPAddress $RemoteIPAddress -ErrorAction Stop)
    $ip = $found | Where-Object {
        $_.PSObject.Properties.Name -contains 'IPAddress' -and
        $_.PSObject.Properties.Name -contains 'InterfaceIndex'
    } | Select-Object -First 1
    $route = $found | Where-Object {
        $_.PSObject.Properties.Name -contains 'NextHop' -and
        $_.PSObject.Properties.Name -contains 'DestinationPrefix' -and
        $_.PSObject.Properties.Name -contains 'InterfaceIndex'
    } | Select-Object -First 1
    if (-not $ip -or -not $route) {
        $shapes = $found | ForEach-Object { ($_.PSObject.Properties.Name | Sort-Object) -join ',' }
        throw "Find-NetRoute did not return both NetIPAddress and NetRoute objects: $($shapes -join ' | ')"
    }
}

$ifIndex = [uint32]$route.InterfaceIndex
if ([uint32]$ip.InterfaceIndex -ne $ifIndex) {
    throw "underlay discovery returned mismatched source/route interfaces: source=$($ip.InterfaceIndex) route=$ifIndex"
}
$adapter = Get-NetAdapter -InterfaceIndex $ifIndex -ErrorAction Stop
$sourceIP = [string]$ip.IPAddress
if ([string]::IsNullOrWhiteSpace($sourceIP) -or $sourceIP -like '169.254.*') {
    throw "no usable IPv4 source address on interface $ifIndex"
}

$nextHop = [string]$route.NextHop
if ([string]::IsNullOrWhiteSpace($nextHop) -or $nextHop -eq '0.0.0.0') {
    $nextHop = $RemoteIPAddress
}

# Trigger neighbor resolution. Lack of an ICMP reply is harmless; ARP work
# happens before ping decides whether the peer answered. PING.EXE is therefore
# deliberately best-effort. Clear its native exit status immediately so a
# reachable ARP neighbor plus an unanswered ICMP echo cannot poison the caller's
# PowerShell process exit code after all route/MAC assertions succeeded.
& "$env:SystemRoot\System32\PING.EXE" -n 1 -w 750 $nextHop *> $null
$global:LASTEXITCODE = 0
$neighbor = Get-NetNeighbor -InterfaceIndex $ifIndex -AddressFamily IPv4 -IPAddress $nextHop -ErrorAction SilentlyContinue |
    Where-Object { $_.State -notin @('Incomplete','Unreachable') -and $_.LinkLayerAddress -and $_.LinkLayerAddress -ne '00-00-00-00-00-00' } |
    Select-Object -First 1
if (-not $neighbor) {
    throw "could not resolve next-hop MAC for $nextHop on interface $ifIndex"
}

$sourceMAC = ([string]$adapter.MacAddress).Replace('-', ':').ToLowerInvariant()
$nextHopMAC = ([string]$neighbor.LinkLayerAddress).Replace('-', ':').ToLowerInvariant()
$guid = ([guid]$adapter.InterfaceGuid).ToString().ToUpperInvariant()
$packetDevice = "\Device\NPF_{$guid}"

$result = [ordered]@{
    remote_ip = $RemoteIPAddress
    source_ip = $sourceIP
    interface_index = $ifIndex
    interface_alias = [string]$adapter.Name
    interface_guid = $guid
    packet_device = $packetDevice
    source_mac = $sourceMAC
    next_hop_ip = $nextHop
    next_hop_mac = $nextHopMAC
    monitor_physical_path = [bool]$MonitorPhysicalPath
}
$result | ConvertTo-Json -Compress
Write-Output "WBD_WINDOWS_FAKETCP_UNDERLAY_PASS remote=$RemoteIPAddress ifindex=$ifIndex next_hop=$nextHop monitor=$([int][bool]$MonitorPhysicalPath)"
