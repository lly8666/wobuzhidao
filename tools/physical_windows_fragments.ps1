param(
    [Parameter(Mandatory=$true)][string]$Bundle,
    [Parameter(Mandatory=$true)][ValidatePattern('^10\.66\.[0-9]{1,3}\.[0-9]{1,3}$')][string]$Lease,
    [Parameter(Mandatory=$true)][ValidateRange(1,65535)][int]$ComponentId,
    [ValidateRange(1,390)][int]$Seconds=330,
    [ValidatePattern('^[a-zA-Z0-9-]{1,64}$')][string]$Name='mtu-fragments'
)
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
$data=Join-Path (Resolve-Path -LiteralPath $Bundle).Path 'data'
$prefix=Join-Path $data $Name
$etl=$prefix+'.etl';$pcap=$prefix+'.pcapng';$text=$prefix+'-events.txt'
$receipt=$prefix+'-capture.json';$ready=$prefix+'-ready.json';$stopRequest=$prefix+'-stop.request'
$filter='WBD-'+$Name
$ownedCapture=$false;$ownedFilter=$false;$errors=@();$started=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
function Invoke-Pktmon([string[]]$Arguments) {
    $result=(& pktmon.exe @Arguments 2>&1 | Out-String)
    if($LASTEXITCODE -ne 0){throw ('Pktmon operation failed: '+$result)}
    return $result
}
function Write-Receipt($Value,[string]$Path) {
    [IO.File]::WriteAllText($Path,($Value|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
}
foreach($path in @($etl,$pcap,$text,$receipt,$ready,$stopRequest)){
    if(Test-Path -LiteralPath $path){throw 'Existing diagnostic artifact; preserve it'}
}
[Net.IPAddress]$parsedLease=$null
if(-not [Net.IPAddress]::TryParse($Lease,[ref]$parsedLease)){throw 'Invalid leased IPv4 address'}
try {
    $status=Invoke-Pktmon @('status')
    # Fail closed on unrecognized/localized status instead of touching another
    # capture. Supported target reports one of these explicit inactive states.
    if($status -notmatch '(?i)(not running|没有运行|未运行)'){throw 'Packet monitor already active or status unknown'}
    $filters=Invoke-Pktmon @('filter','list')
    if($filters -notmatch '(?im)^\s*(无|None|No filters\.?)\s*$'){throw 'Existing/unknown filters; preserve foreign filters'}
    $components=Invoke-Pktmon @('list')
    if($components -notmatch ('(?m)^\s*'+$ComponentId+'\s+.*WBD Tunnel\s*$')){throw 'Selected component is not the current WBD Tunnel'}
    Invoke-Pktmon @('filter','add',$filter,'-d','IPv4','-i','198.18.0.1',$Lease)|Out-Null
    $ownedFilter=$true
    # No port predicate: nonfirst IP fragments have no UDP ports. Select only
    # the controlled target/lease pair and the actual Wintun component.
    Invoke-Pktmon @('start','--capture','--comp',[string]$ComponentId,'--type','all','--pkt-size','64','--file-name',$etl,'--file-size','16','--log-mode','circular')|Out-Null
    $ownedCapture=$true
    Write-Receipt ([pscustomobject]@{StartedUnixMS=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds();ComponentId=$ComponentId;Lease=$Lease;Target='198.18.0.1';SnapBytes=64;MaxETLMiB=16}) $ready
    $clock=[Diagnostics.Stopwatch]::StartNew()
    while($clock.Elapsed.TotalSeconds -lt $Seconds -and -not(Test-Path -LiteralPath $stopRequest)){Start-Sleep -Milliseconds 200}
}catch {
    $errors+= $_.Exception.Message
}finally {
    $captureStatus=$null;$counters=$null;$converted=$false
    if($ownedCapture){
        try {
            $captureStatus=Invoke-Pktmon @('status')
            if($captureStatus -notlike ('*'+$Name+'.etl*')){throw 'Capture ownership changed; preserve current monitor'}
            $counters=Invoke-Pktmon @('counters')
            Invoke-Pktmon @('stop')|Out-Null
            $ownedCapture=$false
            if(-not(Test-Path -LiteralPath $etl) -or (Get-Item -LiteralPath $etl).Length -gt 17MB){throw 'ETL size outside bound'}
            Invoke-Pktmon @('etl2pcap',$etl,'--out',$pcap,'--component-id',[string]$ComponentId)|Out-Null
            Invoke-Pktmon @('etl2txt',$etl,'--out',$text,'--stats','--timestamp','--metadata','--brief')|Out-Null
            if((Get-Item -LiteralPath $pcap).Length -gt 16MB -or (Get-Item -LiteralPath $text).Length -gt 32MB){throw 'Converted diagnostic size outside bound'}
            $converted=$true
            Remove-Item -LiteralPath $etl -Force
        }catch {$errors+= $_.Exception.Message}
    }
    if($ownedFilter -and -not $ownedCapture){
        try{Invoke-Pktmon @('filter','remove',$filter)|Out-Null;$ownedFilter=$false}catch{$errors+= $_.Exception.Message}
    }
    Write-Receipt ([pscustomobject]@{StartedUnixMS=$started;EndedUnixMS=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds();ComponentId=$ComponentId;Lease=$Lease;Target='198.18.0.1';SnapBytes=64;Converted=$converted;CaptureStatus=$captureStatus;Counters=$counters;OwnedCaptureRemaining=$ownedCapture;OwnedFilterRemaining=$ownedFilter;Errors=$errors;Scope='Selected Wintun component; controlled target and lease; not application delivery proof';RawETLDeleted=(-not(Test-Path -LiteralPath $etl));RawPCAPPendingMetadataAudit=(Test-Path -LiteralPath $pcap)}) $receipt
}
if($errors.Count){throw ($errors -join '; ')}
Write-Output 'BOUNDED_FRAGMENT_CAPTURE_COMPLETE'
