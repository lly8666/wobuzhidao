from pathlib import Path

p = Path('scripts/windows_linux_wintun_lane_soak.ps1')
s = p.read_text(encoding='utf-8-sig')

old = "[Parameter(Mandatory=$true)][ValidateSet('off','20:20')][string]$FEC"
new = "[Parameter(Mandatory=$true)][ValidateSet('off','20:4','20:8','20:10','20:12','20:16','20:20')][string]$FEC"
if old not in s:
    raise SystemExit('FEC ValidateSet anchor changed')
s = s.replace(old, new, 1)

start = s.find(" if($rotations.Count-lt3){")
end = s.find(" $fecRows=@();", start)
if start < 0 or end < 0:
    raise SystemExit('rotation assertion anchors changed')
rotation = r''' $expectedRotationEpochs=@();for($epoch=$RotateEverySec;$epoch-le($DurationSec-10);$epoch+=$RotateEverySec){$expectedRotationEpochs+=$epoch};if($expectedRotationEpochs.Count-gt0){if($rotations.Count-lt$expectedRotationEpochs.Count){throw "automatic lane rotations observed=$($rotations.Count), want at least $($expectedRotationEpochs.Count) for duration=$DurationSec rotate=$RotateEverySec"};$times=@($rotations|ForEach-Object{[double]$_.elapsed_sec});$tolerance=[Math]::Max(10.0,[Math]::Min(30.0,[double]$RotateEverySec*0.5));foreach($epoch in $expectedRotationEpochs){if(-not($times|Where-Object{[Math]::Abs($_-$epoch)-le$tolerance})){throw "automatic rotation missing near ${epoch}s tolerance=${tolerance}s observed=$($times-join ',')"}}}
'''
s = s[:start] + rotation + s[end:]

start = s.find(" if($FEC-eq'20:20'){")
end = s.find(" $fecSaturation=", start)
if start < 0 or end < 0:
    raise SystemExit('FEC diagnostics assertion anchors changed')
fixed_diag = r''' if($FEC-ne'off'){if($fecRows.Count-eq0){throw "FEC diagnostics missing for fixed profile $FEC"};foreach($r in $fecRows){if([int]$r.max_blocks-ne640){throw "FEC decoder max_blocks drifted: $($r.max_blocks)"}};$fecLast=$fecRows[-1];$fecCurrent=[int]$fecLast.current_blocks;$fecPeak=[int]$fecLast.peak_blocks;$fecMaxBlocks=[int]$fecLast.max_blocks;$fecPeakRetired=[uint64]$fecLast.peak_retired_blocks;$fecPeakRetiredIncomplete=[uint64]$fecLast.peak_retired_incomplete_blocks;$fecPeakRetiredMissing=[uint64]$fecLast.peak_retired_missing_sources;$fecPressureRetire=[uint64]$fecLast.pressure_retire_events;$fecRecoveryExpire=[uint64]$fecLast.recovery_expire_events;$fecRecoveryExpiredIncomplete=[uint64]$fecLast.recovery_expired_incomplete_blocks;if($fecPeak-ge$fecMaxBlocks){throw "FEC decoder saturated peak=$fecPeak max=$fecMaxBlocks"}}elseif($fecRows.Count-ne0){throw 'FEC diagnostics unexpectedly present while FEC is off'}
'''
s = s[:start] + fixed_diag + s[end:]

ready = ' Write-Output "WBD_WINDOWS_PRODUCTION_CONTROLLER_READY lease=$leaseCIDR lanes=$Lanes fec=$FEC rotation=${RotateEverySec}/${RotateEverySec}"'
if ready not in s:
    raise SystemExit('production controller ready anchor changed')
negotiated = ready + r'''
 $clientReadyPattern='WBD_LINK_READY role=client fec='+[Regex]::Escape($FEC)+'\b';if($qualStdout-notmatch$clientReadyPattern){throw "client did not report negotiated FEC profile $FEC"};$serverLinkPath=Join-Path $LogDir 'link-server.log';if(-not(Test-Path -LiteralPath $serverLinkPath)){throw 'server LINK log missing'};$serverLinkText=Get-Content -LiteralPath $serverLinkPath -Raw;if($FEC-eq'off'){$serverFECPattern='WBD_LINK_MUX_SESSION_READY .*fec_mode=0 fec=0:0 '}else{$parity=$FEC.Split(':')[1];$serverFECPattern='WBD_LINK_MUX_SESSION_READY .*fec_mode=1 fec=20:'+[Regex]::Escape($parity)+' '};if($serverLinkText-notmatch$serverFECPattern){throw "server did not report negotiated FEC profile $FEC pattern=$serverFECPattern"};Write-Output "WBD_FEC_NEGOTIATION_BIDIRECTIONAL_PASS fec=$FEC client=1 server=1"
'''
s = s.replace(ready, negotiated, 1)

p.write_text(s, encoding='utf-8-sig')
print('generalized Windows/WSL Npcap soak to all supported FEC profiles and dynamic rotation assertions')
