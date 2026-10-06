# Actions fixed vectors only; physical_udp_client.cs has already been compiled.
$ErrorActionPreference='Stop'
$h=[WBDPhysicalSendLag]::new()
foreach($value in @(0,.0001,.05,.1,1.05,10,10.001)){$h.Record($value)}
if($h.Samples -ne 7 -or $h.OverflowSamples -ne 1 -or $h.UpperPercentile(.5) -ne .1 -or $h.UpperPercentile(.99) -ne 10.001){throw 'Native histogram boundary wrong'}
$h=[WBDPhysicalSendLag]::new()
for($i=0;$i -lt 99;$i++){$h.Record(1.05)}
$h.Record(200)
if($h.Samples -ne 100 -or $h.UpperPercentile(.99) -ne 1.1 -or $h.UpperPercentile(1) -ne 200){throw 'Native p99 was replaced by maximum'}
$h=[WBDPhysicalSendLag]::new()
for($i=0;$i -lt 98;$i++){$h.Record(1.05)}
$h.Record(20);$h.Record(200)
if($h.UpperPercentile(.99) -ne 200 -or $h.OverflowSamples -ne 2){throw 'Overflow must remain conservative'}
$h=[WBDPhysicalSendLag]::new()
if($h.UpperPercentile(.99) -ne 0){throw 'Empty histogram wrong'}
$h.Record(-1)
if($h.UpperPercentile(.99) -ne 0){throw 'Negative lag wrong'}
for($i=0;$i -lt 100000;$i++){$h.Record(10)}
if($h.Samples -ne 100001 -or $h.UpperPercentile(.99) -ne 10){throw 'Repeated boundary wrong'}
foreach($bad in @([double]::NaN,[double]::PositiveInfinity,[double]::NegativeInfinity)){
    $failed=$false;try{$h.Record($bad)}catch{$failed=$true}
    if(-not $failed){throw 'Non-finite lag accepted'}
}
foreach($bad in @(0,-1,1.001,[double]::NaN,[double]::PositiveInfinity)){
    $failed=$false;try{$h.UpperPercentile($bad)}catch{$failed=$true}
    if(-not $failed){throw 'Invalid percentile accepted'}
}
Write-Output 'NATIVE_SEND_LAG_FIXED_VECTORS_PASS'
