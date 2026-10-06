# Actions-only dry command seam: no real packet monitor or workload.
$ErrorActionPreference='Stop'
$fixture=Join-Path $env:RUNNER_TEMP ('wbd-fragment-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $fixture 'data')|Out-Null
$global:WBDPktmonCalls=[Collections.Generic.List[object]]::new()
$global:WBDPktmonForeign=$false;$global:WBDPktmonActive=$null
$global:WBDPktmonChinese=$false
function global:pktmon.exe {
    $values=@($args);$global:WBDPktmonCalls.Add($values);$global:LASTEXITCODE=0
    switch($values[0]){
        'status' {if($global:WBDPktmonForeign){'Packet monitor is running foreign.etl'}elseif($global:WBDPktmonActive){'Running '+$global:WBDPktmonActive}elseif($global:WBDPktmonChinese){-join @([char]0x6ca1,[char]0x6709,[char]0x8fd0,[char]0x884c)}else{'Packet monitor is not running'}}
        'filter' {if($values[1] -eq 'list'){if($global:WBDPktmonChinese){[string][char]0x65e0}else{'None'}}}
        'list' {'57    WBD Tunnel'}
        'start' {
            $global:WBDPktmonActive=$values[[array]::IndexOf($values,'--file-name')+1]
            [IO.File]::WriteAllBytes($global:WBDPktmonActive,[byte[]]@(1,2,3,4))
        }
        'stop' {$global:WBDPktmonActive=$null}
        'counters' {'Controlled counter fixture'}
        'etl2pcap' {[IO.File]::WriteAllBytes($values[[array]::IndexOf($values,'--out')+1],[byte[]]@(1,2,3,4))}
        'etl2txt' {[IO.File]::WriteAllText($values[[array]::IndexOf($values,'--out')+1],'Controlled event fixture')}
        default {throw 'Unexpected fake monitor operation'}
    }
}
try {
    & $PSScriptRoot/physical_windows_fragments.ps1 -Bundle $fixture -Lease '10.66.1.1' -ComponentId 57 -Seconds 1 -Name fixture
    $result=Get-Content (Join-Path $fixture 'data/fixture-capture.json') -Raw|ConvertFrom-Json
    if(-not $result.Converted -or -not $result.RawETLDeleted -or $result.OwnedCaptureRemaining -or $result.OwnedFilterRemaining){throw 'Own capture cleanup failed'}
    $start=@($global:WBDPktmonCalls|Where-Object {$_[0] -eq 'start'})
    if($start.Count -ne 1 -or -not ($start[0] -contains '--file-size') -or -not ($start[0] -contains '16') -or -not ($start[0] -contains '--pkt-size') -or -not ($start[0] -contains '64')){throw 'Capture arguments were lost or bound changed'}
    $global:WBDPktmonChinese=$true
    & $PSScriptRoot/physical_windows_fragments.ps1 -Bundle $fixture -Lease '10.66.1.1' -ComponentId 57 -Seconds 1 -Name localized
    $global:WBDPktmonChinese=$false
    $foreignBefore=$global:WBDPktmonCalls.Count;$global:WBDPktmonForeign=$true;$failed=$false
    try{& $PSScriptRoot/physical_windows_fragments.ps1 -Bundle $fixture -Lease '10.66.1.1' -ComponentId 57 -Seconds 1 -Name foreign}catch{$failed=$true}
    if(-not $failed){throw 'Foreign active capture was accepted'}
    $foreignCalls=@($global:WBDPktmonCalls|Select-Object -Skip $foreignBefore)
    if(@($foreignCalls|Where-Object {$_[0] -in @('start','stop','filter')}).Count){throw 'Foreign capture was modified'}
    $global:WBDPktmonForeign=$false;$prior=Join-Path $fixture 'data/prior-capture.json';[IO.File]::WriteAllText($prior,'preserve')
    $before=$global:WBDPktmonCalls.Count;$failed=$false
    try{& $PSScriptRoot/physical_windows_fragments.ps1 -Bundle $fixture -Lease '10.66.1.1' -ComponentId 57 -Seconds 1 -Name prior}catch{$failed=$true}
    if(-not $failed -or (Get-Content $prior -Raw) -ne 'preserve' -or $global:WBDPktmonCalls.Count -ne $before){throw 'Existing artifact ownership failed'}
    Write-Output 'FRAGMENT_CAPTURE_OWNERSHIP_PASS'
}finally {
    Remove-Item Function:\pktmon.exe
    $resolvedFixture=[IO.Path]::GetFullPath($fixture)
    $resolvedRoot=[IO.Path]::GetFullPath($env:RUNNER_TEMP).TrimEnd('\')+'\'
    if(-not $resolvedFixture.StartsWith($resolvedRoot,[StringComparison]::OrdinalIgnoreCase)){throw 'Fixture cleanup escaped runner temp'}
    Remove-Item -LiteralPath $fixture -Recurse -Force
}
