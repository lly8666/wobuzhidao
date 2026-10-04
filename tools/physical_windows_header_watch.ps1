param([Parameter(Mandatory=$true)][string]$Bundle,[int]$Seconds=360,[ValidatePattern('^[a-zA-Z0-9-]+$')][string]$Name='d01-diagnostic')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';$data=Join-Path $Bundle 'data';$env:TEMP=Join-Path $data 'tmp';$env:TMP=$env:TEMP
$config=Get-Content (Join-Path $data 'test-config.json') -Raw|ConvertFrom-Json
$route=Find-NetRoute -RemoteIPAddress $config.'server-ip'|Where-Object IPAddress|Select-Object -First 1
$adapter=Get-NetAdapter|Where-Object ifIndex -eq $route.InterfaceIndex|Select-Object -First 1
if(-not $adapter){throw 'No physical adapter'}
Add-Type -Path (Join-Path (Split-Path -Parent $Bundle) 'physical_npcap_watch.cs')
[WBDPhysicalNpcapWatch]::Run(('\Device\NPF_'+([guid]$adapter.InterfaceGuid).ToString('B')),$route.IPAddress,$config.'server-ip',$Seconds,(Join-Path $data ($Name+'-windows-headers.jsonl')))
Write-Output 'HEADER_OBSERVER_COMPLETE'
