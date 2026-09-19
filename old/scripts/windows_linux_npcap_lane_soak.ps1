param(
    [Parameter(Mandatory=$true)][string]$BinDir,
    [Parameter(Mandatory=$true)][string]$LinuxAssets,
    [Parameter(Mandatory=$true)][ValidateSet(1,4)][int]$Lanes,
    [Parameter(Mandatory=$true)][ValidateSet('off','20:20')][string]$FEC,
    [int]$ConnectionMTU = 1420,
    [int]$DurationSec = 300,
    [int]$RateBps = 2000000,
    [int]$LossPct = 15,
    [int]$RotateEverySec = 90,
    [Parameter(Mandatory=$true)][string]$LogDir
)

$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if ($ConnectionMTU -ne 1420 -or $DurationSec -ne 300 -or $RateBps -ne 2000000 -or $LossPct -ne 15 -or $RotateEverySec -ne 90) {
    throw 'qualification constants must remain mtu=1420 duration=300 rate=2Mbps loss=15 rotation=90s'
}
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
$BinDir=(Resolve-Path $BinDir).Path; $LinuxAssets=(Resolve-Path $LinuxAssets).Path; $LogDir=(Resolve-Path $LogDir).Path
$repo=(Resolve-Path (Join-Path $PSScriptRoot '..')).Path

function Wait-Marker([string]$Path,[string]$Pattern,[int]$Seconds=60) {
    $end=[DateTime]::UtcNow.AddSeconds($Seconds)
    while ([DateTime]::UtcNow -lt $end) {
        if ((Test-Path $Path) -and (Select-String -LiteralPath $Path -Pattern $Pattern -Quiet -ErrorAction SilentlyContinue)) { return }
        Start-Sleep -Milliseconds 100
    }
    $tail=if(Test-Path $Path){(Get-Content $Path -Tail 80) -join "`n"}else{'<missing>'}
    throw "marker timeout path=$Path pattern=$Pattern`n$tail"
}

function Start-Logged([string]$Name,[string]$Exe,[string[]]$Args) {
    $out=Join-Path $LogDir "$Name.stdout.log"; $err=Join-Path $LogDir "$Name.stderr.log"
    Remove-Item $out,$err -Force -ErrorAction SilentlyContinue
    $p=Start-Process -FilePath $Exe -ArgumentList $Args -RedirectStandardOutput $out -RedirectStandardError $err -PassThru -NoNewWindow
    return [pscustomobject]@{Name=$Name;Process=$p;Out=$out;Err=$err}
}

function Stop-Logged($Proc) {
    if ($null -eq $Proc) { return }
    try { if (-not $Proc.Process.HasExited) { Stop-Process -Id $Proc.Process.Id -Force -ErrorAction SilentlyContinue } } catch {}
}

$budgetJson = & go run (Join-Path $repo '.github\scripts\mtu_budget.go') -connection-mtu $ConnectionMTU -fec $FEC
if ($LASTEXITCODE -ne 0) { throw 'MTU budget probe failed' }
$budget=$budgetJson | ConvertFrom-Json
$innerMTU=[int]$budget.InnerMTU; $linkMTU=[int]$budget.LinkPlaintextMTU
$budget | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $LogDir 'mtu-budget.json') -Encoding utf8
Write-Output "WBD_SOAK_MTU_DERIVED connection=$ConnectionMTU fec=$FEC link=$linkMTU inner=$innerMTU"

$distro=@(& wsl.exe -l -q | ForEach-Object { $_.Trim([char]0).Trim() } | Where-Object { $_ }) | Select-Object -First 1
if (-not $distro) { throw 'no WSL distribution available on windows-latest' }
function WslPath([string]$p) { (& wsl.exe -d $distro -- wslpath -a $p).Trim() }
$wslAssets=WslPath $LinuxAssets; $wslLog=WslPath $LogDir; $wslServerScript=WslPath (Join-Path $repo 'scripts\wsl_lane_soak_server.sh')
$serverOut=Join-Path $LogDir 'wsl-wrapper.stdout.log'; $serverErr=Join-Path $LogDir 'wsl-wrapper.stderr.log'
$server=Start-Process -FilePath 'wsl.exe' -ArgumentList @('-d',$distro,'--','sudo','bash',$wslServerScript,$wslAssets,$wslLog,"$innerMTU","$Lanes","$LossPct") -RedirectStandardOutput $serverOut -RedirectStandardError $serverErr -PassThru -NoNewWindow
$all=@(); $game=$null; $load=$null; $firewallName="WBD-Npcap-Soak-$PID"
try {
    $ready=Join-Path $LogDir 'server-ready.json'; $end=[DateTime]::UtcNow.AddSeconds(60)
    while (-not (Test-Path $ready)) {
        if ($server.HasExited) { throw "WSL server exited early: $($server.ExitCode)" }
        if ([DateTime]::UtcNow -ge $end) { throw 'WSL server readiness timeout' }
        Start-Sleep -Milliseconds 200
    }
    $serverInfo=Get-Content $ready -Raw | ConvertFrom-Json; $serverIP=[string]$serverInfo.server_ip

    $underlayLines=& powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $repo 'scripts\windows_faketcp_underlay.ps1') -RemoteIPAddress $serverIP
    if ($LASTEXITCODE -ne 0) { throw 'underlay discovery failed' }
    $jsonLine=$underlayLines | Where-Object { $_ -match '^\{' } | Select-Object -First 1
    if (-not $jsonLine) { throw "underlay JSON missing: $($underlayLines -join ' | ')" }
    $underlay=$jsonLine | ConvertFrom-Json
    $underlay | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $LogDir 'underlay.json') -Encoding utf8

    New-NetFirewallRule -DisplayName $firewallName -Direction Inbound -Action Block -Protocol TCP -RemoteAddress $serverIP -Profile Any | Out-Null
    Get-NetFirewallRule -DisplayName $firewallName | Format-List * | Out-String | Set-Content (Join-Path $LogDir 'windows-firewall.txt')
    Write-Output "WBD_WINDOWS_FIREWALL_READY remote=$serverIP action=block-inbound npcap_bypass_expected=1"

    $generation=0
    function Start-Lane([int]$LogicalID,[int]$Slot,[int]$Gen) {
        $fakePort=45100+$Slot-1; $dtlsPort=46100+$Slot-1; $linkPort=47100+$Slot-1
        $sourcePort=51000+($Gen*10)+$LogicalID
        if ($sourcePort -gt 65000) { throw 'source port exhausted' }
        $tag="lane-$LogicalID-g$Gen-s$Slot"
        $ticket=Join-Path $LogDir "$tag.ticket"; $tunnel=Join-Path $LogDir "$tag.tunnel.json"
        $fake=Start-Logged "$tag-faketcp" (Join-Path $BinDir 'wbd-faketcp.exe') @(
            'client','--local-udp',"127.0.0.1:$fakePort",'--source',"$($underlay.source_ip):$sourcePort",'--remote',"${serverIP}:40000",
            '--shadow-recovery','legacy','--packet-device',[string]$underlay.packet_device,'--source-mac',[string]$underlay.source_mac,'--next-hop-mac',[string]$underlay.next_hop_mac,
            '--reality-server-name','target.example','--reality-route-key','WBD_REALITY_ROUTE_KEY_0123456789abcdef','--reality-username','solo','--reality-password','shared-password',
            '--reality-ticket-out',$ticket,'--reality-installation-id','00112233445566778899aabbccddeeff','--reality-tunnel-config-out',$tunnel,'--reality-verify-server=false','--reality-timeout','30s')
        $script:all += $fake
        Wait-Marker $fake.Out 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' 60
        Wait-Marker $fake.Out 'READY role=client' 20
        Wait-Marker $fake.Out 'WBD_FAKETCP_WINDOWS_RAW_SYN_TX' 5
        Wait-Marker $fake.Out 'WBD_FAKETCP_WINDOWS_RAW_SYNACK_RX' 5
        if (-not (Test-Path $ticket) -or -not (Test-Path $tunnel)) { throw "$tag bootstrap outputs missing" }
        $ticketValue=(Get-Content $ticket -Raw).Trim(); $tun=Get-Content $tunnel -Raw | ConvertFrom-Json

        $dtls=Start-Logged "$tag-dtls" (Join-Path $BinDir 'wbd_dtls_shim.exe') @('client',"$dtlsPort",'127.0.0.1',"$fakePort",'none','none')
        $script:all += $dtls; Wait-Marker $dtls.Out 'READY role=client version=DTLSv1.3' 60
        $link=Start-Logged "$tag-link" (Join-Path $BinDir 'wbd-link-proxy.exe') @('-mode','client','-listen',"127.0.0.1:$linkPort",'-dtls',"127.0.0.1:$dtlsPort",'-fec',$FEC,'-mtu',"$linkMTU",'-lanes','1','-keepalive','15s','-demo-reality-ticket',$ticketValue)
        $script:all += $link; Wait-Marker $link.Out "WBD_LINK_READY role=client fec=$([regex]::Escape($FEC))" 60
        return [pscustomobject]@{ID=$LogicalID;Slot=$Slot;Gen=$Gen;LinkAddr="127.0.0.1:$linkPort";Fake=$fake;DTLS=$dtls;Link=$link;TunnelID=[string]$tun.tunnel_id;SourcePort=$sourcePort;Tag=$tag}
    }

    $active=@{}; for($i=1;$i -le $Lanes;$i++){ $lane=Start-Lane $i $i 0; $active[$i]=$lane }
    $sessionIDs=@($active.Values | ForEach-Object {$_.TunnelID} | Select-Object -Unique)
    if ($sessionIDs.Count -ne 1) { throw "initial lanes have different tunnel ids: $($sessionIDs -join ',')" }
    $sessionID=$sessionIDs[0]
    $laneTargets=1..$Lanes | ForEach-Object { $active[$_].LinkAddr }
    $game=Start-Logged 'game-client' (Join-Path $BinDir 'wbd-game-lane-client.exe') @('-listen','127.0.0.1:47500','-lanes',($laneTargets -join ','),'-control','127.0.0.1:47499','-session-id',$sessionID,'-inner-rate-mbps','2')
    $all += $game; Wait-Marker $game.Out "WBD_GAME_LANE_CLIENT_READY.*lanes=$Lanes.*inner_ceiling_mbps=2.000000" 30

    function Send-LaneSet([object[]]$Targets) {
        $rows=@(); foreach($x in $Targets){$rows += [ordered]@{id=[int]$x.ID;address=[string]$x.Address}}
        $body=[ordered]@{op='set';lanes=$rows} | ConvertTo-Json -Compress -Depth 5
        $udp=[System.Net.Sockets.UdpClient]::new(); try {
            $udp.Client.ReceiveTimeout=20000; $bytes=[Text.Encoding]::UTF8.GetBytes($body); [void]$udp.Send($bytes,$bytes.Length,'127.0.0.1',47499)
            $peer=[Net.IPEndPoint]::new([Net.IPAddress]::Any,0); $reply=[Text.Encoding]::UTF8.GetString($udp.Receive([ref]$peer)) | ConvertFrom-Json
            if (-not $reply.ok) { throw "Game lane set rejected: $($reply | ConvertTo-Json -Compress)" }
        } finally {$udp.Dispose()}
    }

    $loadResult=Join-Path $LogDir 'load-result.json'; $loadOut=Join-Path $LogDir 'load.stdout.log'; $loadErr=Join-Path $LogDir 'load.stderr.log'
    $load=Start-Process -FilePath 'python' -ArgumentList @((Join-Path $repo '.github\scripts\windows_game_load.py'),'--duration',"$DurationSec",'--rate-bps',"$RateBps",'--payload-bytes','1000','--out',$loadResult) -RedirectStandardOutput $loadOut -RedirectStandardError $loadErr -PassThru -NoNewWindow
    $loadStart=[Diagnostics.Stopwatch]::StartNew(); $rotations=@(); $spare=5
    for($r=1;$r -le 3;$r++){
        $target=$r*$RotateEverySec
        while($loadStart.Elapsed.TotalSeconds -lt $target){ if($load.HasExited){throw "load exited early: $($load.ExitCode)"}; Start-Sleep -Milliseconds 100 }
        $id=(($r-1)%$Lanes)+1; $old=$active[$id]; $generation++; $begin=$loadStart.Elapsed.TotalSeconds
        $candidate=Start-Lane $id $spare $generation
        if ($candidate.TunnelID -ne $sessionID) { throw "rotation changed logical tunnel id=$id" }
        $before=(Select-String -LiteralPath $game.Out -Pattern "WBD_GAME_LANE_CLIENT_QUALIFIED lane=$id " -ErrorAction SilentlyContinue).Count
        $overlap=@(); for($i=1;$i -le $Lanes;$i++){ $overlap += [pscustomobject]@{ID=$i;Address=$active[$i].LinkAddr}; if($i -eq $id){$overlap += [pscustomobject]@{ID=$i;Address=$candidate.LinkAddr}} }
        Send-LaneSet $overlap
        $qualEnd=[DateTime]::UtcNow.AddSeconds(30); do { Start-Sleep -Milliseconds 100; $nowCount=(Select-String -LiteralPath $game.Out -Pattern "WBD_GAME_LANE_CLIENT_QUALIFIED lane=$id " -ErrorAction SilentlyContinue).Count } while($nowCount -le $before -and [DateTime]::UtcNow -lt $qualEnd)
        if($nowCount -le $before){throw "candidate lane $id did not qualify"}
        $active[$id]=$candidate
        $commit=@(); for($i=1;$i -le $Lanes;$i++){ $commit += [pscustomobject]@{ID=$i;Address=$active[$i].LinkAddr} }
        Send-LaneSet $commit
        $commitAt=$loadStart.Elapsed.TotalSeconds
        Stop-Logged $old.Link; Stop-Logged $old.DTLS; Stop-Logged $old.Fake
        $spare=$old.Slot
        $rotations += [ordered]@{ordinal=$r;lane=$id;scheduled_sec=$target;start_sec=$begin;commit_sec=$commitAt;qualification_sec=($commitAt-$begin);old_slot=$old.Slot;new_slot=$candidate.Slot}
        Write-Output ("WBD_WINDOWS_LANE_ROTATION_PASS ordinal={0} lane={1} scheduled_sec={2} start_sec={3:F3} commit_sec={4:F3}" -f $r,$id,$target,$begin,$commitAt)
    }
    $load.WaitForExit(); if($load.ExitCode -ne 0){throw "load failed: $($load.ExitCode)"}
    if(-not(Test-Path $loadResult)){throw 'load result missing'}
    $loadStats=Get-Content $loadResult -Raw | ConvertFrom-Json
    if([int]$loadStats.bad_payload -ne 0){throw "bad inner payloads=$($loadStats.bad_payload)"}

    New-Item -ItemType File -Force -Path (Join-Path $LogDir 'stop.server') | Out-Null
    if(-not $server.WaitForExit(30000)){ throw 'WSL server did not stop after stop file' }
    if($server.ExitCode -ne 0){throw "WSL server failed: $($server.ExitCode)"}

    $fecRows=@(); $linkServer=Join-Path $LogDir 'link-server.log'
    if(Test-Path $linkServer){ foreach($line in Get-Content $linkServer){ if($line -match '^WBD_LINK_FEC_DIAG\s+(\{.*\})$'){ $fecRows += ($Matches[1] | ConvertFrom-Json) } } }
    $fecPeak=0; $fecCapacity=640
    if($FEC -eq '20:20'){
        if($fecRows.Count -eq 0){throw 'FEC enabled but no WBD_LINK_FEC_DIAG samples were observed'}
        foreach($row in $fecRows){ if([int]$row.decoder.max_blocks -ne 640){throw "unexpected FEC capacity=$($row.decoder.max_blocks)"}; $fecPeak=[Math]::Max($fecPeak,[int]$row.peak_in_flight) }
        if($fecPeak -ge 640){throw "FEC decoder reached capacity: peak=$fecPeak capacity=640"}
    } elseif($fecRows.Count -ne 0){ throw 'FEC off emitted FEC decoder diagnostics unexpectedly' }

    $pressureCapacity=4096; $pressurePeak=0; $pressureMaxCurrent=0; $pressureSamples=0; $pressureByProcess=@()
    $fakeProcesses=@($all | Where-Object {$_.Name -like '*faketcp'})
    $expectedFakeProcesses=$Lanes+3
    if($fakeProcesses.Count -ne $expectedFakeProcesses){throw "unexpected FakeTCP process count=$($fakeProcesses.Count) want=$expectedFakeProcesses"}
    foreach($fp in $fakeProcesses){
        $sampleCount=0; $processPeak=0; $processMaxCurrent=0
        if(-not(Test-Path $fp.Out)){throw "FakeTCP pressure log missing for $($fp.Name)"}
        foreach($line in Get-Content $fp.Out){
            if($line -match '^WBD_FAKETCP_PRESSURE role=client pending=(\d+) peak_pending=(\d+) capacity=(\d+)$'){
                $pending=[int]$Matches[1]; $peak=[int]$Matches[2]; $capacity=[int]$Matches[3]
                if($capacity -ne $pressureCapacity){throw "unexpected FakeTCP outstanding capacity=$capacity process=$($fp.Name)"}
                $sampleCount++; $pressureSamples++; $processMaxCurrent=[Math]::Max($processMaxCurrent,$pending); $processPeak=[Math]::Max($processPeak,$peak)
                $pressureMaxCurrent=[Math]::Max($pressureMaxCurrent,$pending); $pressurePeak=[Math]::Max($pressurePeak,$peak)
            }
        }
        if($sampleCount -eq 0){throw "no FakeTCP pressure samples for $($fp.Name)"}
        if(Select-String -LiteralPath $fp.Out -Pattern 'WBD_FAKETCP_OUTSTANDING_PRESSURE .*action=wait' -Quiet -ErrorAction SilentlyContinue){throw "FakeTCP outstanding window saturated for $($fp.Name)"}
        if($processPeak -ge $pressureCapacity -or $processMaxCurrent -ge $pressureCapacity){throw "FakeTCP outstanding pressure reached capacity process=$($fp.Name) current_peak=$processMaxCurrent sender_peak=$processPeak capacity=$pressureCapacity"}
        $pressureByProcess += [ordered]@{process=$fp.Name;samples=$sampleCount;max_pending=$processMaxCurrent;peak_pending=$processPeak;capacity=$pressureCapacity}
    }

    $net=Get-Content (Join-Path $LogDir 'network-end.log') -Raw
    $drop=0;$accept=0;$sent=0;$qdrop=0
    foreach($line in ($net -split "`n")){
        if($line -match '^\s*(\d+)\s+\d+\s+DROP\s+tcp.*dpt:40000'){ $drop=[int64]$Matches[1] }
        elseif($line -match '^\s*(\d+)\s+\d+\s+ACCEPT\s+tcp.*dpt:40000'){ $accept=[int64]$Matches[1] }
        elseif($line -match 'Sent\s+\d+\s+bytes\s+(\d+)\s+pkt\s+\(dropped\s+(\d+)'){ $sent=[int64]$Matches[1];$qdrop=[int64]$Matches[2] }
    }
    $ingressLoss=if(($drop+$accept)-gt 0){100.0*$drop/($drop+$accept)}else{-1}
    $egressLoss=if(($sent+$qdrop)-gt 0){100.0*$qdrop/($sent+$qdrop)}else{-1}
    if($ingressLoss -lt 10 -or $ingressLoss -gt 20){throw "measured outer ingress loss out of band: $ingressLoss"}
    if($egressLoss -lt 10 -or $egressLoss -gt 20){throw "measured outer egress loss out of band: $egressLoss"}

    $fatal=@(); Get-ChildItem $LogDir -Filter '*.log' | ForEach-Object { $fatal += Select-String -LiteralPath $_.FullName -Pattern 'panic:|fatal error:|WBD_[A-Z0-9_]+_FAIL' -ErrorAction SilentlyContinue }
    if($fatal.Count -gt 0){ $fatal | Out-String | Set-Content (Join-Path $LogDir 'unexpected-errors.txt'); throw "unexpected fatal markers found: $($fatal.Count)" }

    $summary=[ordered]@{lanes=$Lanes;fec=$FEC;connection_mtu=$ConnectionMTU;link_plaintext_mtu=$linkMTU;inner_mtu=$innerMTU;duration_sec=$DurationSec;rate_bps=$RateBps;configured_loss_pct=$LossPct;inner_loss_pct=[double]$loadStats.inner_loss_pct;max_rx_gap_ms=[double]$loadStats.max_rx_gap_ms;outer_ingress_loss_pct=$ingressLoss;outer_egress_loss_pct=$egressLoss;fec_decoder_capacity=$fecCapacity;fec_peak_in_flight=$fecPeak;fec_samples=$fecRows.Count;faketcp_outstanding_capacity=$pressureCapacity;faketcp_max_sampled_pending=$pressureMaxCurrent;faketcp_peak_pending=$pressurePeak;faketcp_pressure_samples=$pressureSamples;faketcp_pressure_by_process=$pressureByProcess;rotations=$rotations;npcap_raw_handshakes=$fakeProcesses.Count}
    $summary | ConvertTo-Json -Depth 8 | Set-Content (Join-Path $LogDir 'summary.json') -Encoding utf8
    Write-Output ("WBD_WINDOWS_LINUX_NPCAP_SOAK_PASS lanes={0} fec={1} inner_loss_pct={2:F4} outer_in_loss_pct={3:F3} outer_out_loss_pct={4:F3} fec_peak={5}/640 faketcp_pending_peak={6}/4096 rotations=3" -f $Lanes,$FEC,[double]$loadStats.inner_loss_pct,$ingressLoss,$egressLoss,$fecPeak,$pressurePeak)
}
finally {
    if($load -and -not $load.HasExited){try{Stop-Process -Id $load.Id -Force}catch{}}
    if($game){Stop-Logged $game}
    foreach($p in @($all)){Stop-Logged $p}
    try{New-Item -ItemType File -Force -Path (Join-Path $LogDir 'stop.server') | Out-Null}catch{}
    if($server -and -not $server.HasExited){try{if(-not $server.WaitForExit(10000)){Stop-Process -Id $server.Id -Force}}catch{}}
    Get-NetFirewallRule -DisplayName $firewallName -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
}
