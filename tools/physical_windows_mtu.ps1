param([Parameter(Mandatory=$true)][string]$Bundle,[ValidateRange(1,300)][int]$Seconds=300,[string]$Name='m01-mtu1400-300s')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
$data=Join-Path $Bundle 'data';$state=Get-Content (Join-Path $data 'p7-status.json') -Raw|ConvertFrom-Json
if($state.State -ne 'RUNNING' -or -not $state.Ready){throw 'Client not ready'}
$s=[Net.Sockets.UdpClient]::new();$s.Connect('198.18.0.1',18446);$s.Client.ReceiveTimeout=1000
$results=@{};$sequence=0;$watch=[Diagnostics.Stopwatch]::StartNew()
$interfaces=@(Get-NetIPInterface -AddressFamily IPv4 | Select-Object InterfaceIndex,InterfaceAlias,NlMtu)
function Probe([int]$length,[bool]$df){
    $key="$length/$df";if(-not $results.ContainsKey($key)){$results[$key]=[pscustomobject]@{UDPPayload=$length;IPv4Total=$length+28;DontFragment=$df;Sent=0;ReceivedExact=0;MessageSizeError=0;Timeout=0;OtherSendError=0;BadPayload=0;LateResponses=0}}
    $r=$results[$key];$s.Client.DontFragment=$df
    $b=[byte[]]::new($length);$b[0]=80;$b[1]=55;$b[2]=77;$b[3]=49
    [Array]::Copy([BitConverter]::GetBytes([int]$sequence),0,$b,4,4)
    for($i=8;$i -lt $length;$i++){$b[$i]=[byte](($i-8)%256)}
    $script:sequence++
    try{$null=$s.Send($b,$b.Length);$r.Sent++}catch{
        $e=$_.Exception;while($e.InnerException){$e=$e.InnerException}
        if($e -is [Net.Sockets.SocketException] -and $e.NativeErrorCode -eq 10040){$r.MessageSizeError++}else{$r.OtherSendError++};return
    }
    try{
        $from=[Net.IPEndPoint]::new([Net.IPAddress]::Any,0);$deadline=[DateTime]::UtcNow.AddSeconds(1)
        do{
            $echo=$s.Receive([ref]$from)
            $late=$echo.Length -ge 8 -and [BitConverter]::ToInt32($echo,4) -ne [BitConverter]::ToInt32($b,4)
            if($late){$r.LateResponses++}
        }while($late -and [DateTime]::UtcNow -lt $deadline)
        if($late){$r.Timeout++;return}
        $same=$echo.Length -eq $b.Length
        for($i=0;$same -and $i -lt $b.Length;$i++){if($echo[$i] -ne $b[$i]){$same=$false}}
        if($same){$r.ReceivedExact++}else{$r.BadPayload++}
    }catch [Net.Sockets.SocketException]{$r.Timeout++}
}
try{
    while($watch.Elapsed.TotalSeconds -lt $Seconds){
        foreach($df in @($false,$true)){foreach($n in @(1371,1372,1373,1472,1972,4068,8972)){if($watch.Elapsed.TotalSeconds -ge $Seconds){break};Probe $n $df}}
        Probe 96 $false
        Start-Sleep -Milliseconds 100
    }
    $null=$s.Send([Text.Encoding]::ASCII.GetBytes('P7M-DONE'),8)
}finally{$s.Dispose()}
$version=(& (Join-Path $Bundle 'wbd-client.exe') --version) -join ' '
if($version -notmatch 'source_sha=([0-9a-f]{40})'){throw 'Missing exact deployed source SHA'}
$sourceSHA=$Matches[1]
$r=[pscustomobject]@{SourceSHA=$sourceSHA;ConnectionMTU=1400;RequestedSeconds=$Seconds;ElapsedSeconds=$watch.Elapsed.TotalSeconds;Interfaces=$interfaces;Cases=@($results.Values|Sort-Object DontFragment,UDPPayload);NoRawPayloadStored=$true}
[IO.File]::WriteAllText((Join-Path $data ($Name+'.json')),($r|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
$r|ConvertTo-Json -Depth 6
