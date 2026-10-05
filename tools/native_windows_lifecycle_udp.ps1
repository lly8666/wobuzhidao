param([Parameter(Mandatory=$true)][string]$Bundle,[double]$Mbps=10,[ValidateRange(300,300)][int]$Seconds=300,[int]$Seed=1001,[string]$Name='idle30-native',[string]$StartSignal='',[switch]$PureDownlink)
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
if($Name -notmatch '^[a-zA-Z0-9-]+$'){throw 'Managed output name required'}
$data=Join-Path $Bundle 'data';$env:TEMP=Join-Path $data 'tmp';$env:TMP=$env:TEMP
$state=Get-Content -LiteralPath (Join-Path $data 'p7-status.json') -Raw | ConvertFrom-Json
if(-not $state.Ready -or $state.State -ne 'RUNNING'){throw 'Client not ready'}
Add-Type -Path (Join-Path (Split-Path -Parent $Bundle) 'native_lifecycle_udp_client.cs')
if($StartSignal){$deadline=[DateTime]::UtcNow.AddSeconds(30);while(-not(Test-Path -LiteralPath $StartSignal)){if([DateTime]::UtcNow -gt $deadline){throw 'Start barrier timeout'};Start-Sleep -Milliseconds 100}}
$index=(Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).InterfaceIndex
$physical=Get-NetAdapter | Where-Object ifIndex -eq $index | Select-Object -First 1
$before=Get-NetAdapterStatistics -Name $physical.Name | Select-Object ReceivedBytes,SentBytes,ReceivedDiscardedPackets,OutboundDiscardedPackets
$r=[WBDNativeLifecycleUDP]::Run('198.18.0.1',18445,$Mbps,$Seconds,$Seed,$state.PID,(Join-Path $data ($Name+'-live.json')),$PureDownlink.IsPresent)
$version=(& (Join-Path $Bundle 'wbd-client.exe') --version) -join ' '
if($version -notmatch 'source_sha=([0-9a-f]{40})'){throw 'Missing exact deployed source SHA'}
$sourceSHA=$Matches[1]
$server=if($r.SummaryReceived){$r.ServerJSON | ConvertFrom-Json}else{$null}
$after=Get-NetAdapterStatistics -Name $physical.Name | Select-Object ReceivedBytes,SentBytes,ReceivedDiscardedPackets,OutboundDiscardedPackets
$result=[pscustomobject]@{Name=$Name;SourceSHA=$sourceSHA;MeasurementStatus=$(if($r.CompletedSendWindow -and $r.SendSocketError -eq 0){'COMPLETE'}else{'INCOMPLETE'});Scope='native scheduled payload idle or pure downlink;no probes/background DNS from fixture';Seconds=$Seconds;RequestedMbps=$Mbps;Client=$r;Server=$server;C2SGoodputMbps=$(if($null -ne $server){$server.RxBytes*8/120/1e6}else{$null});S2CGoodputMbps=$r.RxBytes*8/$(if($PureDownlink){300}else{120})/1e6;C2SByteLossPercent=$(if($null -ne $server){$(if($r.TxBytes -gt 0){100*(1-$server.RxBytes/[double]$r.TxBytes)}else{$null})}else{$null});S2CByteLossPercent=$(if($null -ne $server){100*(1-$r.RxBytes/[double]$server.TxBytes)}else{$null});ClientOfferedMbps=$r.TxBytes*8/120/1e6;ServerOfferedMbps=$(if($null -ne $server){$server.TxBytes*8/$(if($PureDownlink){300}else{120})/1e6}else{$null});NICBefore=$before;NICAfter=$after;NoPcap=$true;ProbePolicy='DISABLED_FOR_PAYLOAD_IDLE';ClientActiveSendSeconds=$(if($PureDownlink){0}else{120});ServerActiveSendSeconds=$(if($PureDownlink){300}else{120});SummaryRetrieval=$(if($r.SummaryReceived){'PASS'}else{'FAILED_IN_BAND; retrieve independent server JSON by SSH'})}
[IO.File]::WriteAllText((Join-Path $data ($Name+'.json')),($result | ConvertTo-Json -Depth 7),[Text.UTF8Encoding]::new($false))
$result | ConvertTo-Json -Depth 7
if($result.MeasurementStatus -ne 'COMPLETE'){exit 1}
