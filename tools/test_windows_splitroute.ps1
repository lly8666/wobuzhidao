# Hosted Apply/Cleanup ownership simulation; never changes runner networking.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$taskDir = Join-Path $env:TEMP ([Guid]::NewGuid().ToString())
[void](New-Item -ItemType Directory -Path $taskDir)
try {
    $capture = @(0..1499 | ForEach-Object { "198.18.$([int][Math]::Floor($_/256)).$($_%256)/32" })
    $captureFile = Join-Path $taskDir 'capture.txt'
    [IO.File]::WriteAllLines($captureFile, $capture)
    $body = [IO.File]::ReadAllText((Join-Path $PSScriptRoot '../scripts/windows_client_network.ps1'))
    $body = $body.Replace("`r`n", "`n")
    # Substitute only privileges/process exit and a journal counter. All real
    # route planning, validation, ownership, rollback, and file I/O stay intact.
    $body = $body.Replace("`nRequire-Admin`n", "`n# MOCK privileges`n").Replace('exit 0','return')
    $body = $body.Replace('function Save-State($State) {','function Save-State($State) { $script:saveCount++')
    $block = [scriptblock]::Create($body)
    function Get-NetFirewallProfile { [pscustomobject]@{Enabled=$true} }
    function Get-NetAdapter { param($Name,$ErrorAction) [pscustomobject]@{ifIndex=77} }
    function Get-NetRoute {
        param($DestinationPrefix,$InterfaceIndex,$NextHop,$PolicyStore,$ErrorAction)
        $script:enumerations++
        @($script:routes) | Where-Object {
            (-not $DestinationPrefix -or $_.DestinationPrefix -eq $DestinationPrefix) -and
            (-not $InterfaceIndex -or $_.InterfaceIndex -eq $InterfaceIndex) -and
            (-not $NextHop -or $_.NextHop -eq $NextHop)
        }
    }
    function New-NetRoute {
        param($DestinationPrefix,$InterfaceIndex,$NextHop,$RouteMetric,$PolicyStore)
        if ($script:failApply -and $DestinationPrefix -eq '198.18.0.7/32') { throw 'MOCK injected capture install failure' }
        $script:routes += [pscustomobject]@{DestinationPrefix=$DestinationPrefix;InterfaceIndex=$InterfaceIndex;NextHop=$NextHop}
    }
    function Remove-NetRoute {
        param([Parameter(ValueFromPipeline=$true)]$InputObject,$Confirm)
        process { $script:routes = @($script:routes | Where-Object {
            -not ($_.DestinationPrefix -eq $InputObject.DestinationPrefix -and $_.InterfaceIndex -eq $InputObject.InterfaceIndex -and $_.NextHop -eq $InputObject.NextHop)
        }) }
    }
    function Get-NetIPAddress { param($InterfaceIndex,$IPAddress,$AddressFamily,$ErrorAction) [pscustomobject]@{IPAddress='10.66.0.2';InterfaceIndex=77} }
    function Set-NetIPInterface { param($InterfaceIndex,$AddressFamily,$Dhcp,$ErrorAction) }
    function New-NetIPAddress { param($InterfaceIndex,$IPAddress,$PrefixLength,$AddressFamily,$SkipAsSource) }
    function Remove-NetIPAddress { param($InterfaceIndex,$IPAddress,$Confirm,$ErrorAction) }
    function Get-NetFirewallRule { param($Group,$ErrorAction) @($script:firewall | Where-Object { $_.Group -eq $Group }) }
    function New-NetFirewallRule { param($DisplayName,$Group,$Description,$Direction,$Action,$Enabled,$Profile,$Protocol,$RemoteAddress,$LocalAddress)
        $script:firewall += [pscustomobject]@{DisplayName=$DisplayName;Group=$Group;Description=$Description}
    }
    function Remove-NetFirewallRule { param([Parameter(ValueFromPipeline=$true)]$InputObject,$ErrorAction) process {
        $script:firewall=@($script:firewall | Where-Object { $_.DisplayName -ne $InputObject.DisplayName -or $_.Description -ne $InputObject.Description })
    } }
    function Get-DnsClientNrptRule { param($ErrorAction) @($script:nrpt) }
    function Add-DnsClientNrptRule { param($Namespace,$NameServers,$DisplayName,$Comment,$PassThru,$ErrorAction)
        if (($NameServers -join ',') -ne '1.1.1.1,8.8.8.8') { throw 'primary/backup resolver list lost' }
        $r=[pscustomobject]@{Name='owned-rule';DisplayName=$DisplayName;Comment=$Comment}
        $script:nrpt+= $r; return $r
    }
    function Remove-DnsClientNrptRule { param($Name,$Force,$Confirm,$ErrorAction) $script:nrpt=@($script:nrpt | Where-Object { $_.Name -ne $Name }) }

    foreach ($fail in @($false,$true)) {
        $script:failApply=$fail; $script:saveCount=0; $script:enumerations=0
        $script:firewall=@([pscustomobject]@{DisplayName='foreign';Group='WBD Runtime IPv6 Kill Switch';Description='foreign-owned'})
        $script:nrpt=@([pscustomobject]@{Name='foreign-rule';DisplayName='Foreign DNS';Comment='foreign-owned'})
        # Pre-existing same capture identity, plus an unrelated route sharing
        # a prefix and Wintun interface but different next hop. Both must stay.
        $script:routes=@(
            [pscustomobject]@{DestinationPrefix='198.18.0.1/32';InterfaceIndex=77;NextHop='0.0.0.0'},
            [pscustomobject]@{DestinationPrefix='198.18.0.2/32';InterfaceIndex=77;NextHop='192.0.2.254'}
        )
        $state=Join-Path $taskDir "state-$fail.json"
        $taskArgs=@{AdapterAlias='WBD';TunnelAddress4='10.66.0.2/32';Underlay4='203.0.113.10';PhysicalInterfaceIndex=12;PhysicalNextHop4='192.0.2.1';DNSServer='1.1.1.1,8.8.8.8';CapturePrefixFile4=$captureFile;StatePath=$state}
        $failed=$false
        try { & $block @taskArgs -Action Apply | Out-Null } catch { $failed=$true; if (-not $fail) { throw } }
        if ($failed -ne $fail) { throw 'injected failure did not execute' }
        if (-not $fail) {
            $saved=Get-Content -LiteralPath $state -Raw | ConvertFrom-Json
            if (@($saved.CaptureRoutes).Count -ne 1499) { throw 'owned capture journal count wrong' }
            if (@($saved.CaptureRoutes6).Count -ne 2) { throw 'IPv6 sink journal wrong' }
            if ($saved.NRPTRuleName -ne 'owned-rule' -or @($script:firewall).Count -ne 3) { throw 'DNS/IPv6 policies not applied' }
            if (@($script:routes).Count -ne 1504) { throw 'large Apply count wrong' }
            & $block @taskArgs -Action Cleanup | Out-Null
        }
        if (Test-Path -LiteralPath $state) { throw 'state not cleaned after cleanup/rollback' }
        if (@($script:routes).Count -ne 2 -or -not ($script:routes | Where-Object { $_.NextHop -eq '192.0.2.254' })) { throw 'foreign/pre-existing routes lost or owned leaked' }
        if (@($script:firewall).Count -ne 1 -or $script:firewall[0].Description -ne 'foreign-owned' -or @($script:nrpt).Count -ne 1 -or $script:nrpt[0].Name -ne 'foreign-rule') { throw 'DNS/firewall cleanup leaked or removed foreign state' }
        if ($script:saveCount -gt 5 -or $script:enumerations -gt 12) { throw 'per-prefix state/query work returned' }
        Write-Output "WINDOWS_SPLIT_OWNERSHIP_MOCK_PASS fail=$fail prefixes=1500 saves=$script:saveCount enumerations=$script:enumerations physical=NOT_RUN"
    }
} finally {
    $taskResolved = [IO.Path]::GetFullPath($taskDir)
    $taskTempRoot = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\'
    if (-not $taskResolved.StartsWith($taskTempRoot,[StringComparison]::OrdinalIgnoreCase)) { throw 'unsafe mock cleanup path' }
    Remove-Item -LiteralPath $taskResolved -Recurse -Force
}
