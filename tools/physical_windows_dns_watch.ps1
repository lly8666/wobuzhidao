param([Parameter(Mandatory=$true)][string]$Bundle,[ValidateRange(1,360)][int]$Seconds=330,[ValidatePattern('^[a-zA-Z0-9-]+$')][string]$Name='dns-leak')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';$data=Join-Path $Bundle 'data';$env:TEMP=Join-Path $data 'tmp';$env:TMP=$env:TEMP
$config=Get-Content (Join-Path $data 'test-config.json') -Raw|ConvertFrom-Json
$route=Find-NetRoute -RemoteIPAddress $config.'server-ip'|Where-Object IPAddress|Select-Object -First 1
$adapter=Get-NetAdapter|Where-Object ifIndex -eq $route.InterfaceIndex|Select-Object -First 1
if(-not $adapter){throw 'No physical adapter'}
Add-Type -Path (Join-Path (Split-Path -Parent $Bundle) 'physical_npcap_watch.cs')
[WBDPhysicalNpcapWatch]::RunDNS(('\Device\NPF_'+([guid]$adapter.InterfaceGuid).ToString('B')),$Seconds,(Join-Path $data ($Name+'-physical-dns.jsonl')))
Write-Output 'DNS_METADATA_OBSERVER_COMPLETE'
