param([Parameter(Mandatory=$true)][string]$Bundle,[string]$Target='198.18.0.1')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$data=Join-Path $Bundle 'data';$utf8=[Text.UTF8Encoding]::new($false)
function FromHex([string]$value){
    $bytes=[byte[]]::new($value.Length/2)
    for($i=0;$i -lt $bytes.Length;$i++){$bytes[$i]=[Convert]::ToByte($value.Substring(2*$i,2),16)}
    return ,$bytes
}
$query=FromHex '123401000001000000000000047465737404776264300000010001'
$expected=FromHex '123481800001000100000000047465737404776264300000010001c00c000100010000003c00040a320002'
$udp=[Net.Sockets.UdpClient]::new()
try {
    $udp.Client.ReceiveTimeout=15000
    $watch=[Diagnostics.Stopwatch]::StartNew()
    [void]$udp.Send($query,$query.Length,$Target,15353)
    $peer=[Net.IPEndPoint]::new([Net.IPAddress]::Any,0)
    $answer=$udp.Receive([ref]$peer)
    $dnsRtt=$watch.Elapsed.TotalMilliseconds
    if([Convert]::ToBase64String($answer) -ne [Convert]::ToBase64String($expected)){throw 'Controlled DNS/UDP response differs'}
} finally {$udp.Dispose()}
$tcp=[Net.Sockets.TcpClient]::new()
try {
    $connect=$tcp.ConnectAsync($Target,18444)
    if(-not $connect.Wait(15000)){throw 'Controlled TCP connection timeout'}
    $tcp.ReceiveTimeout=15000;$tcp.SendTimeout=15000
    $stream=$tcp.GetStream();$marker=[Text.Encoding]::ASCII.GetBytes('wbd-plain-tcp-probe')
    $stream.Write($marker,0,$marker.Length)
    $reply=[byte[]]::new($marker.Length);$received=0
    while($received -lt $reply.Length){$n=$stream.Read($reply,$received,$reply.Length-$received);if($n -eq 0){throw 'Controlled TCP truncated'};$received+=$n}
    if([Convert]::ToBase64String($reply) -ne [Convert]::ToBase64String($marker)){throw 'Controlled TCP payload differs'}
} finally {$tcp.Dispose()}
$bodyPath=Join-Path $data 'p7-https-body.tmp'
try {
    & curl.exe --noproxy '*' --cacert (Join-Path $data 'qual.crt') --resolve "qual.test:18443:$Target" --connect-timeout 15 --max-time 30 --silent --show-error --output $bodyPath 'https://qual.test:18443/qualification'
    if($LASTEXITCODE -ne 0){throw 'Controlled HTTPS certificate/content request failed'}
    $bytes=[IO.File]::ReadAllBytes($bodyPath)
    if($bytes.Length -ne 102400){throw 'Controlled HTTPS length differs'}
    for($i=0;$i -lt $bytes.Length;$i++){if($bytes[$i] -ne ($i%256)){throw 'Controlled HTTPS payload differs'}}
    $hash=(Get-FileHash -LiteralPath $bodyPath -Algorithm SHA256).Hash.ToLower()
} finally {Remove-Item -LiteralPath $bodyPath -Force -ErrorAction SilentlyContinue}
$systemDNS=@(Resolve-DnsName 'www.cloudflare.com' -Type A -DnsOnly -ErrorAction Stop | Where-Object Type -eq A | Select-Object -ExpandProperty IPAddress)
if($systemDNS.Count -eq 0){throw 'System DNS produced no IPv4 address'}
$result=[pscustomobject]@{Result='PASS';ControlledUDPAndDNSExact=$true;ControlledTCPExact=$true;HTTPSCertificateVerified=$true;HTTPSBodyBytes=102400;HTTPSBodySHA256=$hash;UDPRoundTripMs=[math]::Round($dnsRtt,2);SystemDNSIPv4=$systemDNS;PayloadTempDeleted=(-not (Test-Path -LiteralPath $bodyPath));NoPcap=$true}
[IO.File]::WriteAllText((Join-Path $data 'p7-business-result.json'),($result | ConvertTo-Json),$utf8)
$result | ConvertTo-Json
