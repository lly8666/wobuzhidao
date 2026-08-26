param(
    [Parameter(Mandatory=$true)]
    [string]$RemoteIPAddress
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$remote = $null
if (-not [System.Net.IPAddress]::TryParse($RemoteIPAddress, [ref]$remote) -or $remote.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
    throw "RemoteIPAddress must be IPv4: $RemoteIPAddress"
}

$best = Find-NetRoute -RemoteIPAddress $RemoteIPAddress -ErrorAction Stop | Select-Object -First 1
if (-not $best) { throw "no route to $RemoteIPAddress" }
$ifIndex = [uint32]$best.InterfaceIndex
$adapter = Get-NetAdapter -InterfaceIndex $ifIndex -ErrorAction Stop
$ip = Get-NetIPAddress -InterfaceIndex $ifIndex -AddressFamily IPv4 -ErrorAction Stop |
    Where-Object { $_.IPAddress -notlike '169.254.*' -and $_.AddressState -ne 'Duplicate' } |
    Sort-Object @{Expression={ if ($_.AddressState -eq 'Preferred') { 0 } else { 1 } }}, PrefixLength -Descending |
    Select-Object -First 1
if (-not $ip) { throw "no usable IPv4 source address on interface $ifIndex" }

$nextHop = [string]$best.NextHop
if ([string]::IsNullOrWhiteSpace($nextHop) -or $nextHop -eq '0.0.0.0') {
    $nextHop = $RemoteIPAddress
}

# Trigger neighbor resolution. Lack of an ICMP reply is harmless; ARP/NDP work
# happens before ping decides whether the peer answered.
& "$env:SystemRoot\System32\PING.EXE" -n 1 -w 750 $nextHop *> $null
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
    source_ip = [string]$ip.IPAddress
    interface_index = $ifIndex
    interface_alias = [string]$adapter.Name
    interface_guid = $guid
    packet_device = $packetDevice
    source_mac = $sourceMAC
    next_hop_ip = $nextHop
    next_hop_mac = $nextHopMAC
}
$result | ConvertTo-Json -Compress
Write-Output "WBD_WINDOWS_FAKETCP_UNDERLAY_PASS remote=$RemoteIPAddress ifindex=$ifIndex next_hop=$nextHop"
