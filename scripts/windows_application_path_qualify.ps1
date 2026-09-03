param(
    [Parameter(Mandatory = $true)]
    [string]$QualifierExe,

    [Parameter(Mandatory = $true)]
    [string]$Profile,

    [Parameter(Mandatory = $true)]
    [string]$UdpEchoTarget,

    [Parameter(Mandatory = $true)]
    [string]$TcpEchoTarget,

    [Parameter(Mandatory = $true)]
    [string]$PortableDir,

    [Parameter(Mandatory = $true)]
    [string]$SourceSHA,

    [Parameter(Mandatory = $true)]
    [string]$ArtifactRunID,

    [Parameter(Mandatory = $true)]
    [string]$WbdSHA256,

    [ValidateRange(16, 4096)]
    [int]$Rounds = 128,

    [ValidateRange(64, 16384)]
    [int]$PayloadBytes = 4096,

    [string]$LogDir = ""
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Resolve-IPv4Target([string]$Value, [string]$Label) {
    if ($Value -notmatch '^(?<host>.+):(?<port>[0-9]{1,5})$') {
        throw "$Label must be host:port: $Value"
    }
    $hostName = $Matches.host.Trim()
    if ($hostName.StartsWith('[') -and $hostName.EndsWith(']')) {
        $hostName = $hostName.Substring(1, $hostName.Length - 2)
    }
    $port = [int]$Matches.port
    if ($port -lt 1 -or $port -gt 65535) {
        throw "$Label port must be 1..65535: $Value"
    }
    $addresses = @([System.Net.Dns]::GetHostAddresses($hostName) | Where-Object {
        $_.AddressFamily -eq [System.Net.Sockets.AddressFamily]::InterNetwork
    })
    if ($addresses.Count -eq 0) {
        throw "$Label resolved no IPv4 address: $hostName"
    }
    return [pscustomobject]@{
        Label = $Label
        Host = $hostName
        Address = $addresses[0]
        Port = $port
    }
}

function Assert-WBDRoute([object]$Target, [uint32]$WbdInterfaceIndex) {
    $found = @(Find-NetRoute -RemoteIPAddress $Target.Address.IPAddressToString)
    $route = $found | Where-Object {
        $_.PSObject.Properties.Name -contains 'DestinationPrefix'
    } | Select-Object -First 1
    if (-not $route) {
        throw "$($Target.Label) target has no selected Windows route: $($Target.Address)"
    }
    if ([uint32]$route.InterfaceIndex -ne $WbdInterfaceIndex) {
        throw "$($Target.Label) target bypasses WBD: target=$($Target.Address) selected_ifindex=$($route.InterfaceIndex) wbd_ifindex=$WbdInterfaceIndex prefix=$($route.DestinationPrefix)"
    }
    Write-Output "WBD_WINDOWS_APPLICATION_ROUTE_PASS protocol=$($Target.Label) target=$($Target.Address):$($Target.Port) ifindex=$WbdInterfaceIndex prefix=$($route.DestinationPrefix)"
}

function New-ProbePayload([int]$Round, [int]$Size, [byte]$Salt) {
    $payload = New-Object byte[] $Size
    for ($i = 0; $i -lt $Size; $i++) {
        $payload[$i] = [byte](($Round + $i + $Salt) % 251)
    }
    $marker = [System.Text.Encoding]::ASCII.GetBytes(("WBD{0:D6}" -f $Round))
    [Array]::Copy($marker, 0, $payload, 0, [Math]::Min($marker.Length, $payload.Length))
    return $payload
}

function Assert-BytesEqual([byte[]]$Want, [byte[]]$Got, [string]$Label) {
    if ($Want.Length -ne $Got.Length) {
        throw "$Label length mismatch: got=$($Got.Length) want=$($Want.Length)"
    }
    for ($i = 0; $i -lt $Want.Length; $i++) {
        if ($Want[$i] -ne $Got[$i]) {
            throw "$Label payload mismatch at byte $i"
        }
    }
}

function Invoke-UDPEcho([object]$Target, [int]$Count, [int]$Size) {
    $client = [System.Net.Sockets.UdpClient]::new([System.Net.Sockets.AddressFamily]::InterNetwork)
    try {
        $client.Client.SendTimeout = 5000
        $client.Client.ReceiveTimeout = 5000
        $client.Connect($Target.Address, $Target.Port)
        for ($round = 0; $round -lt $Count; $round++) {
            [byte[]]$payload = New-ProbePayload -Round $round -Size $Size -Salt 17
            $sent = $client.Send($payload, $payload.Length)
            if ($sent -ne $payload.Length) {
                throw "UDP echo short send: round=$round sent=$sent want=$($payload.Length)"
            }
            $peer = [System.Net.IPEndPoint]::new([System.Net.IPAddress]::Any, 0)
            [byte[]]$reply = $client.Receive([ref]$peer)
            if (-not $peer.Address.Equals($Target.Address) -or $peer.Port -ne $Target.Port) {
                throw "UDP echo reply peer mismatch: got=$peer want=$($Target.Address):$($Target.Port)"
            }
            Assert-BytesEqual -Want $payload -Got $reply -Label "UDP echo round $round"
        }
    }
    finally {
        $client.Dispose()
    }
    Write-Output "WBD_WINDOWS_APPLICATION_UDP_PASS round_trips=$Count payload_bytes=$Size"
}

function Read-TCPExactly([System.Net.Sockets.NetworkStream]$Stream, [int]$Size, [int]$Round) {
    [byte[]]$reply = New-Object byte[] $Size
    $offset = 0
    while ($offset -lt $Size) {
        $n = $Stream.Read($reply, $offset, $Size - $offset)
        if ($n -le 0) {
            throw "TCP echo closed before full reply: round=$Round bytes=$offset want=$Size"
        }
        $offset += $n
    }
    return $reply
}

function Invoke-TCPEcho([object]$Target, [int]$Count, [int]$Size) {
    $client = [System.Net.Sockets.TcpClient]::new([System.Net.Sockets.AddressFamily]::InterNetwork)
    try {
        $client.SendTimeout = 5000
        $client.ReceiveTimeout = 5000
        $client.NoDelay = $true
        $client.Connect($Target.Address, $Target.Port)
        $stream = $client.GetStream()
        for ($round = 0; $round -lt $Count; $round++) {
            [byte[]]$payload = New-ProbePayload -Round $round -Size $Size -Salt 83
            $stream.Write($payload, 0, $payload.Length)
            $stream.Flush()
            [byte[]]$reply = Read-TCPExactly -Stream $stream -Size $payload.Length -Round $round
            Assert-BytesEqual -Want $payload -Got $reply -Label "TCP echo round $round"
        }
    }
    finally {
        $client.Dispose()
    }
    Write-Output "WBD_WINDOWS_APPLICATION_TCP_PASS round_trips=$Count payload_bytes=$Size"
}

$QualifierExe = (Resolve-Path -LiteralPath $QualifierExe).Path
$Profile = (Resolve-Path -LiteralPath $Profile).Path
$PortableDir = (Resolve-Path -LiteralPath $PortableDir).Path
if ([string]::IsNullOrWhiteSpace($LogDir)) {
    $LogDir = Join-Path $env:TEMP 'WBD-application-qualification'
}
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

$stopFile = Join-Path $LogDir 'stop.signal'
$readyFile = Join-Path $LogDir 'ready.signal'
$routeState = Join-Path $env:ProgramData 'WBD\qualification\route-state.json'
$stdoutPath = Join-Path $LogDir 'qualifier.stdout.log'
$stderrPath = Join-Path $LogDir 'qualifier.stderr.log'
Remove-Item -LiteralPath $stopFile, $readyFile, $stdoutPath, $stderrPath -Force -ErrorAction SilentlyContinue

$udpTarget = Resolve-IPv4Target -Value $UdpEchoTarget -Label 'udp'
$tcpTarget = Resolve-IPv4Target -Value $TcpEchoTarget -Label 'tcp'

$psi = [System.Diagnostics.ProcessStartInfo]::new()
$psi.FileName = $QualifierExe
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.CreateNoWindow = $true
$psi.Environment['WBD_PORTABLE_DIR'] = $PortableDir
foreach ($arg in @('-profile', $Profile, '-run-for', '0', '-stop-file', $stopFile, '-ready-file', $readyFile)) {
    [void]$psi.ArgumentList.Add($arg)
}

$proc = [System.Diagnostics.Process]::new()
$proc.StartInfo = $psi
if (-not $proc.Start()) {
    throw 'failed to start wbd-windows-qualify.exe'
}
$stdoutTask = $proc.StandardOutput.ReadToEndAsync()
$stderrTask = $proc.StandardError.ReadToEndAsync()

$probeError = $null
$cleanupError = $null
try {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    while (-not (Test-Path -LiteralPath $readyFile)) {
        if ($proc.HasExited) {
            throw "WBD qualifier exited before ready-file: exit=$($proc.ExitCode)"
        }
        if ([DateTime]::UtcNow -ge $deadline) {
            throw 'WBD qualifier did not become connected within 60 seconds'
        }
        Start-Sleep -Milliseconds 100
    }

    if (-not (Test-Path -LiteralPath $routeState)) {
        throw "WBD route state missing after connected ready-file: $routeState"
    }
    $state = Get-Content -LiteralPath $routeState -Raw | ConvertFrom-Json
    $wbdInterfaceIndex = [uint32]$state.AdapterInterfaceIndex
    if ($wbdInterfaceIndex -eq 0) {
        throw 'WBD route state has invalid AdapterInterfaceIndex=0'
    }

    Assert-WBDRoute -Target $udpTarget -WbdInterfaceIndex $wbdInterfaceIndex
    Assert-WBDRoute -Target $tcpTarget -WbdInterfaceIndex $wbdInterfaceIndex
    Invoke-UDPEcho -Target $udpTarget -Count $Rounds -Size $PayloadBytes
    Invoke-TCPEcho -Target $tcpTarget -Count $Rounds -Size $PayloadBytes
}
catch {
    $probeError = $_.Exception.Message
}
finally {
    try {
        if (-not $proc.HasExited) {
            New-Item -ItemType File -Force -Path $stopFile | Out-Null
            if (-not $proc.WaitForExit(60000)) {
                try { $proc.Kill($true) } catch {}
                throw 'WBD qualifier did not stop within 60 seconds after stop-file'
            }
        }
    }
    catch {
        $cleanupError = $_.Exception.Message
    }

    $stdout = $stdoutTask.GetAwaiter().GetResult()
    $stderr = $stderrTask.GetAwaiter().GetResult()
    [System.IO.File]::WriteAllText($stdoutPath, $stdout)
    [System.IO.File]::WriteAllText($stderrPath, $stderr)

    if (-not $cleanupError -and $proc.HasExited) {
        if ($proc.ExitCode -ne 0) {
            $cleanupError = "WBD qualifier exited $($proc.ExitCode); see $stderrPath"
        } elseif ($stdout -notmatch '(?m)^WBD_WINDOWS_QUALIFY_CLEANUP_PASS routes=removed runtime=stopped\s*$') {
            $cleanupError = "WBD qualifier output missing cleanup PASS; see $stdoutPath"
        }
    }
    $proc.Dispose()
}

if ($probeError -and $cleanupError) {
    throw "application-path qualification failed: $probeError; cleanup also failed: $cleanupError"
}
if ($probeError) {
    throw $probeError
}
if ($cleanupError) {
    throw $cleanupError
}

$oneWayBytes = [int64]$Rounds * [int64]$PayloadBytes * 2
Write-Output "WBD_WINDOWS_APPLICATION_PATH_PASS source_sha=$SourceSHA artifact_run_id=$ArtifactRunID wbd_sha256=$WbdSHA256 route_fence=1 udp_round_trips=$Rounds tcp_round_trips=$Rounds payload_bytes=$PayloadBytes application_one_way_bytes=$oneWayBytes cleanup=1"
