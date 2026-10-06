param(
    [Parameter(Mandatory=$true)][string]$Bundle,
    [Parameter(Mandatory=$true)][ValidateRange(1,65535)][int]$PhysicalInterfaceIndex,
    [Parameter(Mandatory=$true)][string]$PhysicalIPv4,
    [ValidateSet('off','on')][string]$ExpectedGuard='on',
    [ValidatePattern('^[A-Za-z0-9-]+$')][string]$Name='dns-guard-probe'
)
# Qualified native fixture only. Two known public DNS queries; no retained
# packet contents, DNS names from other programs, keys, or per-packet telemetry.
$ErrorActionPreference='Stop'
function New-GuardDNSQuery {
    # ID=0xA73B, RD, one A/IN question for www.cloudflare.com.
    [byte[]]$q=@(0xa7,0x3b,1,0,0,1,0,0,0,0,0,0)
    foreach($label in @('www','cloudflare','com')){
        $q+= [byte]$label.Length
        $q+= [Text.Encoding]::ASCII.GetBytes($label)
    }
    $q+= [byte[]]@(0,0,1,0,1)
    return ,$q
}
function Test-GuardDNSReply([byte[]]$Bytes,[int]$Count) {
    return $Count -ge 12 -and $Count -le $Bytes.Length -and $Bytes[0] -eq 0xa7 -and $Bytes[1] -eq 0x3b -and ($Bytes[2] -band 0x80) -ne 0
}
function Read-GuardDNSExact($Socket,[int]$Count,$Watch) {
    if($Count -lt 1 -or $Count -gt 4096){throw 'Bounded DNS response size exceeded'}
    $bytes=[byte[]]::new($Count);$at=0
    while($at -lt $Count){
        $remaining=1500-[int]$Watch.ElapsedMilliseconds
        if($remaining -le 0){throw [Net.Sockets.SocketException]::new(10060)}
        $Socket.ReceiveTimeout=$remaining
        $n=$Socket.Receive($bytes,$at,$Count-$at,[Net.Sockets.SocketFlags]::None)
        if($n -le 0){throw 'DNS stream closed'}
        $at+=$n
    }
    return ,$bytes
}
function Invoke-GuardDNSProbe([string]$Protocol,[Net.IPAddress]$Local,[int]$Index) {
    $watch=[Diagnostics.Stopwatch]::StartNew();$socket=$null;$async=$null
    $result=[ordered]@{protocol=$Protocol;received_valid_reply=$false;socket_error=$null;elapsed_ms=0;forced_interface=$Index}
    try{
        $type=if($Protocol -eq 'UDP'){[Net.Sockets.SocketType]::Dgram}else{[Net.Sockets.SocketType]::Stream}
        $proto=if($Protocol -eq 'UDP'){[Net.Sockets.ProtocolType]::Udp}else{[Net.Sockets.ProtocolType]::Tcp}
        $socket=[Net.Sockets.Socket]::new([Net.Sockets.AddressFamily]::InterNetwork,$type,$proto)
        $socket.Bind([Net.IPEndPoint]::new($Local,0))
        # Windows IP_UNICAST_IF takes the interface index in network byte order.
        $socket.SetSocketOption([Net.Sockets.SocketOptionLevel]::IP,[Net.Sockets.SocketOptionName]31,[Net.IPAddress]::HostToNetworkOrder($Index))
        $socket.SendTimeout=1500;$socket.ReceiveTimeout=1500
        $endpoint=[Net.IPEndPoint]::new([Net.IPAddress]::Parse('1.1.1.1'),53)
        $async=$socket.BeginConnect($endpoint,$null,$null)
        if(-not $async.AsyncWaitHandle.WaitOne(1500)){throw [Net.Sockets.SocketException]::new(10060)}
        $socket.EndConnect($async)
        $q=New-GuardDNSQuery
        if($Protocol -eq 'UDP'){
            if($socket.Send($q) -ne $q.Length){throw 'Short DNS datagram send'}
            $reply=[byte[]]::new(4096);$n=$socket.Receive($reply)
            $result.received_valid_reply=Test-GuardDNSReply $reply $n
        }else{
            $frame=[byte[]]@([byte]($q.Length -shr 8),[byte]($q.Length -band 255))+$q
            $at=0
            while($at -lt $frame.Length){
                if($watch.ElapsedMilliseconds -ge 1500){throw [Net.Sockets.SocketException]::new(10060)}
                $n=$socket.Send($frame,$at,$frame.Length-$at,[Net.Sockets.SocketFlags]::None)
                if($n -le 0){throw 'Short DNS stream send'}
                $at+=$n
            }
            $length=Read-GuardDNSExact $socket 2 $watch
            $reply=Read-GuardDNSExact $socket (([int]$length[0] -shl 8)+[int]$length[1]) $watch
            $result.received_valid_reply=Test-GuardDNSReply $reply $reply.Length
        }
    }catch{
        $errorObject=$_.Exception
        while($errorObject.InnerException){$errorObject=$errorObject.InnerException}
        $result.socket_error=if($errorObject -is [Net.Sockets.SocketException]){[int]$errorObject.SocketErrorCode}else{$errorObject.GetType().Name}
    }finally{
        if($socket){$socket.Dispose()}
        if($async){$async.AsyncWaitHandle.Dispose()}
        $result.elapsed_ms=$watch.Elapsed.TotalMilliseconds
    }
    return [pscustomobject]$result
}
$address=[Net.IPAddress]::Parse($PhysicalIPv4)
if($address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork){throw 'Physical IPv4 required'}
$actual=@(Get-NetIPAddress -InterfaceIndex $PhysicalInterfaceIndex -AddressFamily IPv4 -ErrorAction Stop | Where-Object IPAddress -eq $PhysicalIPv4)
if($actual.Count -ne 1){throw 'Physical source/interface mismatch'}
$own=@(Get-NetFirewallRule -Group 'WBD Runtime DNS Underlay Guard' -ErrorAction SilentlyContinue | Where-Object Description -eq 'wbd-owned-runtime-dns-underlay-guard/v1')
if(($ExpectedGuard -eq 'on' -and $own.Count -ne 2) -or ($ExpectedGuard -eq 'off' -and $own.Count -ne 0)){throw 'Actual DNS guard count differs from expected'}
$attempts=@('UDP','TCP'|ForEach-Object{Invoke-GuardDNSProbe $_ $address $PhysicalInterfaceIndex})
$version=(& (Join-Path $Bundle 'wbd-client.exe') --version)-join ' '
$receipt=[ordered]@{source_version=$version;expected_guard=$ExpectedGuard;client_unix_ms=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds();owned_rules=$own.Count;attempts=$attempts;no_payload_stored=$true;scope='Forced selected underlay ordinary DNS only; combine reachable off-control and independent physical metadata observer to prove on-guard'}
$output=Join-Path (Join-Path $Bundle 'data') ($Name+'.json')
[IO.File]::WriteAllText($output,($receipt|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
$receipt|ConvertTo-Json -Depth 6
