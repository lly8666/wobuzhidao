param([Parameter(Mandatory=$true)][string]$Bundle,[ValidateRange(1,300)][int]$Seconds=300,[switch]$MaximumUDP,[string]$Name='m03-boundary-300s')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
if($Name -notmatch '^[a-zA-Z0-9-]+$'){throw 'Managed output name required'}
$data=Join-Path $Bundle 'data';$env:TEMP=Join-Path $data 'tmp';$env:TMP=$env:TEMP
New-Item -ItemType Directory -Path $env:TEMP -Force|Out-Null
$state=Get-Content -LiteralPath (Join-Path $data 'p7-status.json') -Raw|ConvertFrom-Json
if($state.State -ne 'RUNNING' -or -not $state.Ready){throw 'Client not ready'}
$interfaces=@(Get-NetIPInterface -AddressFamily IPv4|Select-Object InterfaceIndex,InterfaceAlias,NlMtu)
[IO.File]::WriteAllText((Join-Path $data ($Name+'-ip-before.txt')),((netstat.exe -s -p ip) -join [Environment]::NewLine),[Text.UTF8Encoding]::new($false))
Add-Type -Path (Join-Path (Split-Path -Parent $Bundle) 'native_mtu_probe.cs')
$r=[WBDNativeMTU]::Run('198.18.0.1',18446,$Seconds,$MaximumUDP.IsPresent,$state.PID)
[IO.File]::WriteAllText((Join-Path $data ($Name+'-ip-after.txt')),((netstat.exe -s -p ip) -join [Environment]::NewLine),[Text.UTF8Encoding]::new($false))
$version=(& (Join-Path $Bundle 'wbd-client.exe') --version)-join ' '
if($version -notmatch 'source_sha=([0-9a-f]{40})'){throw 'Exact binary source required'}
$result=[pscustomobject]@{SourceSHA=$Matches[1];Scope='native MTU/API boundary functional scenario,not a throughput test';Interfaces=$interfaces;Result=$r;MaximumUDP=$MaximumUDP.IsPresent;NoRawPayloadStored=$true}
[IO.File]::WriteAllText((Join-Path $data ($Name+'.json')),($result|ConvertTo-Json -Depth 7),[Text.UTF8Encoding]::new($false))
$result|ConvertTo-Json -Depth 7
if(-not $r.ClientAliveAfter -or $r.InvalidResponses -ne 0 -or $r.ReceiveSocketErrors -ne 0 -or @($r.Cases|Where-Object {$_.BadPayload -ne 0 -or $_.OtherSendError -ne 0}).Count){exit 1}
