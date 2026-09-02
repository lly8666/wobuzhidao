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
$ifIndex = [uint32]$route.InterfaceIndex
if ([uint32]$ip.InterfaceIndex -ne $ifIndex) {
    throw "Find-NetRoute returned mismatched source/route interfaces: source=$($ip.InterfaceIndex) route=$ifIndex"
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
}
$result | ConvertTo-Json -Compress
Write-Output "WBD_WINDOWS_FAKETCP_UNDERLAY_PASS remote=$RemoteIPAddress ifindex=$ifIndex next_hop=$nextHop"
