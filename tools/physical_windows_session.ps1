param(
    [Parameter(Mandatory=$true)][string]$Bundle,
    [int]$MaxSeconds = 600
)
# Physical qualification controller only: no payload capture, no new data plane.
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$data=Join-Path $Bundle 'data'
$logs=Join-Path $Bundle 'logs'
$stop=Join-Path $data 'p7-stop.request'
$status=Join-Path $data 'p7-status.json'
$log=Join-Path $logs 'p7-session.log'
$utf8=[Text.UTF8Encoding]::new($false)
$env:TEMP=Join-Path $data 'tmp'; $env:TMP=$env:TEMP
New-Item -ItemType Directory -Path $env:TEMP,$logs -Force | Out-Null
Remove-Item -LiteralPath $stop -Force -ErrorAction SilentlyContinue
[IO.File]::WriteAllText($log,'',$utf8)
function Snapshot {
    [pscustomobject]@{
        Routes=@(Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric)
        NRPT=@(Get-DnsClientNrptRule | Select-Object Name,Namespace,NameServers,DisplayName)
        Firewall=@(Get-NetFirewallRule -Group 'WBD Runtime IPv6 Kill Switch' -ErrorAction SilentlyContinue | Select-Object Name,Enabled)
    }
}
$before=Snapshot
[IO.File]::WriteAllText((Join-Path $data 'p7-network-before.json'),($before | ConvertTo-Json -Depth 6),$utf8)
$info=[Diagnostics.ProcessStartInfo]::new()
$info.FileName=Join-Path $Bundle 'wbd-client.exe'
$info.Arguments='--config "'+(Join-Path $data 'test-config.json')+'" --control-stdin'
$info.WorkingDirectory=$Bundle
$info.UseShellExecute=$false; $info.CreateNoWindow=$true
$info.RedirectStandardInput=$true; $info.RedirectStandardOutput=$true; $info.RedirectStandardError=$true
$process=[Diagnostics.Process]::new();$process.StartInfo=$info
$ready=$false; $reason='watchdog';$started=[DateTime]::UtcNow; $samples=0
try {
    if(-not $process.Start()){throw 'Client process did not start'}
    $out=$process.StandardOutput.ReadLineAsync();$err=$process.StandardError.ReadLineAsync()
    while(([DateTime]::UtcNow-$started).TotalSeconds -lt $MaxSeconds){
        foreach($channel in @('out','err')){
            $pending=Get-Variable -Name $channel -ValueOnly
            if($null -ne $pending -and $pending.IsCompleted){
                $line=$pending.GetAwaiter().GetResult()
                if($null -eq $line){Set-Variable -Name $channel -Value $null;continue}
                [IO.File]::AppendAllText($log,$line+[Environment]::NewLine,$utf8)
                if($line -eq 'WBD_WINDOWS_CLIENT_READY'){$ready=$true}
                $reader=if($channel -eq 'out'){$process.StandardOutput}else{$process.StandardError}
                Set-Variable -Name $channel -Value $reader.ReadLineAsync()
            }
        }
        if($process.HasExited){$reason='client_exit';break}
        if(Test-Path -LiteralPath $stop){$reason='requested_stop';break}
        if((Get-Item -LiteralPath $log).Length -gt 2MB){$reason='bounded_log_limit';break}
        if($samples++ % 10 -eq 0){
            $process.Refresh()
            [IO.File]::WriteAllText($status,([pscustomobject]@{State='RUNNING';Ready=$ready;PID=$process.Id;ElapsedSeconds=[math]::Round(([DateTime]::UtcNow-$started).TotalSeconds,1);CPUSeconds=$process.TotalProcessorTime.TotalSeconds;WorkingSetBytes=$process.WorkingSet64} | ConvertTo-Json),$utf8)
        }
        Start-Sleep -Milliseconds 100
    }
} finally {
    if(-not $process.HasExited){
        try{$process.StandardInput.WriteLine('stop');$process.StandardInput.Close()}catch{}
        if(-not $process.WaitForExit(30000)){
            [IO.File]::WriteAllText($status,'{"State":"CLEANUP_TIMEOUT","Ready":false}',$utf8)
            throw 'Graceful client cleanup timed out; leaving owned process for inspection'
        }
    }
    foreach($channel in @('out','err')){
        $pending=Get-Variable -Name $channel -ValueOnly
        if($null -ne $pending){
            $line=$pending.GetAwaiter().GetResult()
            if($null -ne $line){[IO.File]::AppendAllText($log,$line+[Environment]::NewLine,$utf8)}
            $reader=if($channel -eq 'out'){$process.StandardOutput}else{$process.StandardError}
            [IO.File]::AppendAllText($log,$reader.ReadToEnd(),$utf8)
        }
    }
    $after=Snapshot
    [IO.File]::WriteAllText((Join-Path $data 'p7-network-after.json'),($after | ConvertTo-Json -Depth 6),$utf8)
    [IO.File]::WriteAllText($status,([pscustomobject]@{State='STOPPED';Ready=$ready;Reason=$reason;ExitCode=$process.ExitCode;NetworkStateRemaining=(Test-Path -LiteralPath (Join-Path $data 'network-state.json'));OwnedNRPTRemaining=@($after.NRPT | Where-Object DisplayName -eq 'WBD Runtime DNS').Count;OwnedFirewallRemaining=@($after.Firewall).Count;ElapsedSeconds=[math]::Round(([DateTime]::UtcNow-$started).TotalSeconds,1)} | ConvertTo-Json),$utf8)
    $process.Dispose()
}
