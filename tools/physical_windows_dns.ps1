param([Parameter(Mandatory=$true)][string]$Bundle,[string]$StartSignal='')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
$data=Join-Path $Bundle 'data';$samples=@()
if($StartSignal){$deadline=[DateTime]::UtcNow.AddSeconds(30);while(-not(Test-Path -LiteralPath $StartSignal)){if([DateTime]::UtcNow -gt $deadline){throw 'DNS start barrier timeout'};Start-Sleep -Milliseconds 100}}
$watch=[Diagnostics.Stopwatch]::StartNew()
$rules=@(Get-DnsClientNrptRule | Where-Object DisplayName -eq 'WBD Runtime DNS' | Select-Object Namespace,NameServers,DisplayName)
$slot=0
while($watch.Elapsed.TotalSeconds -lt 300){
    if($watch.Elapsed.TotalSeconds -lt $slot*10){Start-Sleep -Milliseconds 100;continue}
    foreach($tcp in @($false,$true)){
        Clear-DnsClientCache
        $probe=[Diagnostics.Stopwatch]::StartNew();$ok=$false;$addresses=@();$errorName=$null
        try{
            $p=@{Name='www.cloudflare.com';Type='A';DnsOnly=$true;NoHostsFile=$true;QuickTimeout=$true;ErrorAction='Stop'}
            if($tcp){$p['TcpOnly']=$true}
            $addresses=@(Resolve-DnsName @p | Where-Object IPAddress | Select-Object -ExpandProperty IPAddress)
            $ok=$addresses.Count -gt 0
        }catch{$errorName=$_.FullyQualifiedErrorId}
        $samples+=[pscustomobject]@{ElapsedSeconds=$watch.Elapsed.TotalSeconds;TCPOnly=$tcp;Success=$ok;Milliseconds=$probe.Elapsed.TotalMilliseconds;IPv4=$addresses;Error=$errorName}
    }
    $slot++
}
$r=[pscustomobject]@{SourceSHA='6181db66b67594b07cd989b8b8b5848cedf6ccc3';Seconds=300;OwnedNRPT=$rules;Samples=$samples;CacheFlushedBeforeEach=$true;ResolverSpecifiedOnQuery=$false;NoPacketPayloadStored=$true}
[IO.File]::WriteAllText((Join-Path $data 'd01-dns-default-300s-dns.json'),($r|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
$r|ConvertTo-Json -Depth 6
