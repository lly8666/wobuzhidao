# Hosted ownership fixtures only; no network or driver mutation.
$ErrorActionPreference='Stop'
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Resolve-Path "$PSScriptRoot/../scripts/windows_client_network.ps1"),[ref]$tokens,[ref]$errors)
if($errors.Count){throw ($errors|Out-String)}
$defs=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Restore-OwnedTunnelMTU'},$true))
if($defs.Count -ne 1){throw 'Missing unique MTU restoration function'}
Invoke-Expression $defs[0].Extent.Text
$script:current=@();$script:writes=@()
function Get-NetIPInterface {
    param($InterfaceIndex,$AddressFamily,$PolicyStore,$ErrorAction)
    if($InterfaceIndex -ne 21 -or $AddressFamily -ne 'IPv4' -or $PolicyStore -ne 'ActiveStore'){throw 'Unsafe lookup'}
    return $script:current
}
function Set-NetIPInterface {
    param($InterfaceIndex,$AddressFamily,$NlMtuBytes,$PolicyStore,$ErrorAction)
    $script:writes+=@([pscustomobject]@{Index=$InterfaceIndex;Family=$AddressFamily;MTU=$NlMtuBytes;Store=$PolicyStore})
}
$saved=[pscustomobject]@{InterfaceIndex=21;AdapterAlias='WBD';Applied=9000;Previous=65535}
$script:current=@([pscustomobject]@{InterfaceAlias='WBD';NlMtu=9000})
Restore-OwnedTunnelMTU $saved
if($script:writes.Count -ne 1 -or $script:writes[0].MTU -ne 65535 -or $script:writes[0].Index -ne 21 -or $script:writes[0].Family -ne 'IPv4' -or $script:writes[0].Store -ne 'ActiveStore'){throw 'Owned original MTU not restored safely'}
foreach($current in @(
    [pscustomobject]@{InterfaceAlias='WBD';NlMtu=1500},
    [pscustomobject]@{InterfaceAlias='other';NlMtu=9000}
)){
    $script:current=@($current);$script:writes=@()
    Restore-OwnedTunnelMTU $saved
    if($script:writes.Count){throw 'Cleanup overwrote later MTU or foreign adapter'}
}
$script:current=@();$script:writes=@()
Restore-OwnedTunnelMTU $saved
if($script:writes.Count){throw 'Cleanup changed a missing interface'}
Write-Output 'WBD_TUN_MTU_OWNERSHIP_FIXTURES_PASS actual_driver=UNSUPPORTED'
