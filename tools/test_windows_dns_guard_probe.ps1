# Pure fixture vectors only: no sockets or network mutation on hosted runner.
$ErrorActionPreference='Stop'
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Resolve-Path "$PSScriptRoot/native_windows_dns_guard_probe.ps1"),[ref]$tokens,[ref]$errors)
if($errors.Count){throw ($errors|Out-String)}
foreach($name in @('New-GuardDNSQuery','Test-GuardDNSReply')){
    $defs=@($ast.FindAll({param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name},$true))
    if($defs.Count -ne 1){throw 'Missing unique DNS vector function'}
    Invoke-Expression $defs[0].Extent.Text
}
$q=New-GuardDNSQuery
if($q.Length -ne 36 -or ([BitConverter]::ToString($q)) -ne 'A7-3B-01-00-00-01-00-00-00-00-00-00-03-77-77-77-0A-63-6C-6F-75-64-66-6C-61-72-65-03-63-6F-6D-00-00-01-00-01'){throw 'Known DNS request wire vector changed'}
if(Test-GuardDNSReply $q $q.Length){throw 'DNS query accepted as reply'}
$response=[byte[]]@(0xa7,0x3b,0x81,0x80,0,1,0,0,0,0,0,0)
if(-not(Test-GuardDNSReply $response 12)){throw 'Known DNS response rejected'}
if((Test-GuardDNSReply $response 11) -or (Test-GuardDNSReply $response 13)){throw 'Truncated/out-of-range reply accepted'}
$response[1]=0x3c
if(Test-GuardDNSReply $response 12){throw 'Wrong DNS transaction accepted'}
if([BitConverter]::ToString([BitConverter]::GetBytes([Net.IPAddress]::HostToNetworkOrder([int]12))) -ne '00-00-00-0C'){throw 'IP_UNICAST_IF network byte order incorrect'}
Write-Output 'WBD_DNS_GUARD_PROBE_VECTORS_PASS physical=NOT_RUN'
