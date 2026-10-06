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
    $body = $body.Replace("`nRequire-Admin`n", "`n# MOCK privileges`n").Replace('    Require-Admin', '    # MOCK privileges').Replace('exit 0','return')
    $body = $body.Replace('function Save-State($State) {','function Save-State($State) { $script:saveCount++')
    $block = [scriptblock]::Create($body)
    function Get-NetFirewallProfile { [pscustomobject]@{Enabled=$true} }
    function Get-NetAdapter {
        param($Name,$ErrorAction)
        if($Name){return [pscustomobject]@{ifIndex=77;Name=$Name}}
        if($script:missingPhysical){return [pscustomobject]@{ifIndex=77;Name='WBD'}}
        @([pscustomobject]@{ifIndex=12;Name='Ethernet [test]*'},[pscustomobject]@{ifIndex=77;Name='WBD'})
    }
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
        if($script:guardRequired -and @($script:firewall | Where-Object Description -eq 'wbd-owned-runtime-dns-underlay-guard/v1').Count -ne 2){throw 'Route installed before DNS underlay guard'}
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
    function Get-NetIPInterface {
        param($InterfaceIndex,$AddressFamily,$PolicyStore,$ErrorAction)
        if($InterfaceIndex -ne 77 -or $AddressFamily -ne 'IPv4' -or $PolicyStore -ne 'ActiveStore'){throw 'MTU lookup touched foreign interface/store'}
        [pscustomobject]@{InterfaceAlias='WBD';InterfaceIndex=77;NlMtu=$script:tunnelMtu}
    }
    function Set-NetIPInterface {
        param($InterfaceIndex,$AddressFamily,$Dhcp,$NlMtuBytes,$PolicyStore,$ErrorAction)
        if($InterfaceIndex -ne 77 -or $AddressFamily -ne 'IPv4'){throw 'MTU mutation touched foreign interface'}
        if($NlMtuBytes){
            if($PolicyStore -ne 'ActiveStore'){throw 'MTU mutation persisted outside ActiveStore'}
            if($NlMtuBytes -eq 9000){
                $intent=Get-Content -LiteralPath $script:mtuJournal -Raw|ConvertFrom-Json
                if($intent.TunnelMTUState.Previous -ne $script:tunnelMtu -or $intent.TunnelMTUState.Applied -ne 9000){throw 'MTU changed before correct journal was saved'}
            }
            $script:tunnelMtu=$NlMtuBytes
        }
    }
    function New-NetIPAddress { param($InterfaceIndex,$IPAddress,$PrefixLength,$AddressFamily,$SkipAsSource) }
    function Remove-NetIPAddress { param($InterfaceIndex,$IPAddress,$Confirm,$ErrorAction) }
    function Get-NetFirewallRule { param($Group,$ErrorAction) @($script:firewall | Where-Object { $_.Group -eq $Group }) }
    function New-NetFirewallRule { param($DisplayName,$Group,$Description,$Direction,$Action,$Enabled,$Profile,$Protocol,$RemoteAddress,$LocalAddress,$RemotePort,$InterfaceAlias)
        if($Group -eq 'WBD Runtime DNS Underlay Guard'){
            if(-not $script:guardRequired){throw 'dns-hijack=false installed DNS guard'}
            if($Direction -ne 'Outbound' -or $Action -ne 'Block' -or $RemotePort -ne 53 -or $Protocol -notin @('UDP','TCP') -or $Profile -ne 'Any' -or $Enabled -ne 'True'){throw 'DNS guard port/protocol/direction/profile scope incorrect'}
            if($InterfaceAlias -isnot [Management.Automation.WildcardPattern] -or -not $InterfaceAlias.IsMatch('Ethernet [test]*') -or $InterfaceAlias.IsMatch('WBD') -or $InterfaceAlias.IsMatch('Ethernet testrandom')){throw 'DNS guard broadened literal physical interface alias'}
            if($script:failDNS -and $Protocol -eq 'TCP'){throw 'MOCK injected second DNS guard failure'}
        }
        $script:firewall += [pscustomobject]@{DisplayName=$DisplayName;Group=$Group;Description=$Description;Protocol=$Protocol;RemotePort=$RemotePort;InterfaceAlias=$InterfaceAlias}
    }
    function Remove-NetFirewallRule { param([Parameter(ValueFromPipeline=$true)]$InputObject) process {
        $script:firewall=@($script:firewall | Where-Object { $_.DisplayName -ne $InputObject.DisplayName -or $_.Description -ne $InputObject.Description })
    } }
    function Get-DnsClientNrptRule { param($ErrorAction) @($script:nrpt) }
    function Add-DnsClientNrptRule { param($Namespace,$NameServers,$DisplayName,$Comment,[switch]$PassThru,$ErrorAction)
        if (($NameServers -join ',') -ne '1.1.1.1,8.8.8.8') { throw 'primary/backup resolver list lost' }
        $r=[pscustomobject]@{Name='owned-rule';DisplayName=$DisplayName;Comment=$Comment}
        $script:nrpt+= $r; return $r
    }
    function Remove-DnsClientNrptRule { param($Name,[switch]$Force,$Confirm,$ErrorAction) $script:nrpt=@($script:nrpt | Where-Object { $_.Name -ne $Name }) }

    foreach ($scenario in @('success','route-failure','dns-failure','dns-off','alias-failure')) {
        $fail=$scenario -in @('route-failure','dns-failure','alias-failure')
        $script:missingPhysical=$scenario -eq 'alias-failure'
        $script:failApply=$scenario -eq 'route-failure';$script:failDNS=$scenario -eq 'dns-failure';$script:guardRequired=$scenario -ne 'dns-off'; $script:saveCount=0; $script:enumerations=0
        $script:firewall=@(
            [pscustomobject]@{DisplayName='foreign';Group='WBD Runtime IPv6 Kill Switch';Description='foreign-owned'},
            [pscustomobject]@{DisplayName='WBD Block Underlay DNS UDP';Group='WBD Runtime DNS Underlay Guard';Description='foreign-owned'}
        )
        $script:nrpt=@([pscustomobject]@{Name='foreign-rule';DisplayName='Foreign DNS';Comment='foreign-owned'})
        # Pre-existing same capture identity, plus an unrelated route sharing
        # a prefix and Wintun interface but different next hop. Both must stay.
        $script:routes=@(
            [pscustomobject]@{DestinationPrefix='198.18.0.1/32';InterfaceIndex=77;NextHop='0.0.0.0'},
            [pscustomobject]@{DestinationPrefix='198.18.0.2/32';InterfaceIndex=77;NextHop='192.0.2.254'}
        )
        $state=Join-Path $taskDir "state-$scenario.json"
        $script:tunnelMtu=65535;$script:mtuJournal=$state
        $taskArgs=@{AdapterAlias='WBD';TunnelAddress4='10.66.0.2/32';Underlay4='203.0.113.10';PhysicalInterfaceIndex=12;PhysicalNextHop4='192.0.2.1';DNSServer='1.1.1.1,8.8.8.8';CapturePrefixFile4=$captureFile;StatePath=$state}
        if(-not $script:guardRequired){$taskArgs.DNSServer=''}
        $failed=$false
        try { & $block @taskArgs -Action Apply | Out-Null } catch { $failed=$true; if (-not $fail) { throw } }
        if ($failed -ne $fail) { throw 'injected failure did not execute' }
        if (-not $fail) {
            $saved=Get-Content -LiteralPath $state -Raw | ConvertFrom-Json
            if (@($saved.CaptureRoutes).Count -ne 1499) { throw 'owned capture journal count wrong' }
            if (@($saved.CaptureRoutes6).Count -ne 2) { throw 'IPv6 sink journal wrong' }
            $expectedRule=if($script:guardRequired){'owned-rule'}else{''}
            $expectedFirewall=if($script:guardRequired){6}else{4}
            if ($saved.NRPTRuleName -ne $expectedRule -or @($script:firewall).Count -ne $expectedFirewall) { throw 'DNS/IPv6 policies not applied' }
            if($script:tunnelMtu -ne 9000 -or $saved.TunnelMTUState.Previous -ne 65535){throw 'Supported inner MTU not applied or original value lost'}
            if (@($script:routes).Count -ne 1504) { throw 'large Apply count wrong' }
            # GUI crash recovery has only the owned state path, not old profile args.
            & $block -Action Cleanup -StatePath $state | Out-Null
        }
        if (Test-Path -LiteralPath $state) { throw 'state not cleaned after cleanup/rollback' }
        if($script:tunnelMtu -ne 65535){throw 'Cleanup/rollback did not restore prior inner MTU'}
        if (@($script:routes).Count -ne 2 -or -not ($script:routes | Where-Object { $_.NextHop -eq '192.0.2.254' })) { throw 'foreign/pre-existing routes lost or owned leaked' }
        if (@($script:firewall).Count -ne 2 -or @($script:firewall | Where-Object Description -ne 'foreign-owned').Count -ne 0 -or @($script:nrpt).Count -ne 1 -or $script:nrpt[0].Name -ne 'foreign-rule') { throw 'DNS/firewall cleanup leaked or removed foreign state' }
        # One additional constant journal write protects the MTU intent, not a
        # return to per-prefix state rewriting for the1500-route fixture.
        if ($script:saveCount -gt 6 -or $script:enumerations -gt 12) { throw 'per-prefix state/query work returned' }
        Write-Output "WINDOWS_SPLIT_OWNERSHIP_MOCK_PASS scenario=$scenario prefixes=1500 saves=$script:saveCount enumerations=$script:enumerations physical=NOT_RUN"
    }
    # A crash before an NRPT/capture install can leave the first guard rule.
    # Missing-journal recovery must remove only the exact owned marker, including
    # when the next launch has DNS hijack disabled; foreign same-name rules stay.
    $script:firewall += [pscustomobject]@{DisplayName='WBD Block Underlay DNS TCP';Group='WBD Runtime DNS Underlay Guard';Description='wbd-owned-runtime-dns-underlay-guard/v1'}
    & $block -Action Cleanup -StatePath (Join-Path $taskDir 'missing-state.json') | Out-Null
    if(@($script:firewall).Count -ne 2 -or @($script:firewall | Where-Object Description -ne 'foreign-owned').Count){throw 'Missing-journal DNS guard cleanup lost ownership'}
} finally {
    $taskResolved = [IO.Path]::GetFullPath($taskDir)
    $taskTempRoot = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\'
    if (-not $taskResolved.StartsWith($taskTempRoot,[StringComparison]::OrdinalIgnoreCase)) { throw 'unsafe mock cleanup path' }
    Remove-Item -LiteralPath $taskResolved -Recurse -Force
}
