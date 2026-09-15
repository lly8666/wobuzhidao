param(
    [Parameter(Mandatory=$true)][ValidateSet('Install','Status','Cleanup')][string]$Action,
    [string]$DriverPackageDir = '',
    [string]$UserlandExtractDir = '',
    [string]$StatePath = "$env:RUNNER_TEMP\wbd-npcap-driver-ci-state.json"
)

$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest

$NpcapVersion='1.88'
$ExpectedSigner='Nmap Software LLC'
$ExpectedDriverHashes=@{
    'NPFInstall.exe'='97bdbecd9cd9d246369d14b24a5f0c8ddaae9c8592f55566158a13e02dff2301'
    'npcap.sys'='13d598e277e9c7bf43688d7087ef9b944e8036561a1e7169d31d9ec1d38f9a01'
    'npcap.cat'='e71aee891d5374c9edcfd429445d687e0fd56832ff8681e6ab975e2bd767607b'
}
$ExpectedPacketHash='2793ce72f0e04d5885aaee1273a7373441d01934b2cff3886b031c13ca826345'
$ExpectedWpcapHash='d1ca7fcf9128d02a75eaf29ce9a9d85c5697377460f92420d976da187521cf39'
$RuntimeDir=Join-Path $env:SystemRoot 'System32\Npcap'
$RuntimePacket=Join-Path $RuntimeDir 'Packet.dll'
$RuntimeWpcap=Join-Path $RuntimeDir 'wpcap.dll'

function Get-Hash([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}
function Assert-Hash([string]$Path,[string]$Want,[string]$Label) {
    if(-not(Test-Path -LiteralPath $Path)){throw "$Label missing: $Path"}
    $got=Get-Hash $Path
    if($got-ne$Want){throw "$Label SHA256 mismatch: got=$got want=$Want path=$Path"}
    return $got
}
function Assert-NmapSignature([string]$Path,[string]$Label) {
    $sig=Get-AuthenticodeSignature -FilePath $Path
    $subject=if($sig.SignerCertificate){[string]$sig.SignerCertificate.Subject}else{''}
    if($sig.Status-ne[System.Management.Automation.SignatureStatus]::Valid-or$subject-notmatch[regex]::Escape($ExpectedSigner)){
        throw "$Label signature invalid: status=$($sig.Status) signer=$subject path=$Path"
    }
    return $subject
}
function Assert-CatalogSignature([string]$Path) {
    $sig=Get-AuthenticodeSignature -FilePath $Path
    $subject=if($sig.SignerCertificate){[string]$sig.SignerCertificate.Subject}else{''}
    if($sig.Status-ne[System.Management.Automation.SignatureStatus]::Valid-or$subject-notmatch'Microsoft Windows Hardware Compatibility Publisher'){
        throw "Npcap catalog signature invalid: status=$($sig.Status) signer=$subject"
    }
    return $subject
}
function Get-NpcapService {
    return Get-Service -Name 'npcap' -ErrorAction SilentlyContinue
}
function Find-UserlandByHash([string]$Root,[string]$Name,[string]$Hash) {
    if(-not(Test-Path -LiteralPath $Root)){throw "Npcap userland extract directory missing: $Root"}
    $matches=@(Get-ChildItem -LiteralPath $Root -Recurse -File -Filter $Name | Where-Object {(Get-Hash $_.FullName)-eq$Hash})
    if($matches.Count-ne1){throw "expected exactly one signed x64 $Name with locked hash, found $($matches.Count)"}
    return $matches[0].FullName
}
function Publish-WindowsPowerShellModulePath {
    $required=Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\Modules'
    $machine=[string][Environment]::GetEnvironmentVariable('PSModulePath','Machine')
    $parts=New-Object System.Collections.Generic.List[string]
    foreach($candidate in @($required)+@($machine -split ';')+@([string]$env:PSModulePath -split ';')){
        if([string]::IsNullOrWhiteSpace($candidate)){continue}
        $normalized=$candidate.Trim()
        if(-not($parts|Where-Object{$_-ieq$normalized})){[void]$parts.Add($normalized)}
    }
    $value=[string]::Join(';',$parts)
    if([string]::IsNullOrWhiteSpace($value)){throw 'unable to construct Windows PowerShell module path'}
    $env:PSModulePath=$value
    $powershell=Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    & $powershell -NoProfile -NonInteractive -Command "Import-Module Microsoft.PowerShell.Security -ErrorAction Stop; if(-not(Get-Command Get-AuthenticodeSignature -ErrorAction Stop)){exit 2}"
    if($LASTEXITCODE){throw "Windows PowerShell security-module probe failed: $LASTEXITCODE"}
    if(-not[string]::IsNullOrWhiteSpace([string]$env:GITHUB_ENV)){
        "PSModulePath=$value"|Out-File -LiteralPath $env:GITHUB_ENV -Append -Encoding utf8
    }
    Write-Output "WBD_WINDOWS_POWERSHELL_MODULE_PATH_READY security_module=Microsoft.PowerShell.Security"
}
function Assert-RuntimeReady {
    $svc=Get-NpcapService
    if($null-eq$svc){throw 'npcap service is missing'}
    if([string]$svc.Status-ne'Running'){
        Start-Service -Name 'npcap'
        $svc=Get-NpcapService
        if([string]$svc.Status-ne'Running'){throw "npcap service is not running: $($svc.Status)"}
    }
    [void](Assert-Hash $RuntimePacket $ExpectedPacketHash 'Npcap Packet.dll')
    [void](Assert-Hash $RuntimeWpcap $ExpectedWpcapHash 'Npcap wpcap.dll')
    [void](Assert-NmapSignature $RuntimePacket 'Npcap Packet.dll')
    [void](Assert-NmapSignature $RuntimeWpcap 'Npcap wpcap.dll')
    Publish-WindowsPowerShellModulePath
    Write-Output "WBD_NPCAP_DRIVER_CI_READY version=$NpcapVersion service=$($svc.Status) packet_sha256=$ExpectedPacketHash wpcap_sha256=$ExpectedWpcapHash"
}

switch($Action){
    'Install' {
        if([string]::IsNullOrWhiteSpace($DriverPackageDir)-or[string]::IsNullOrWhiteSpace($UserlandExtractDir)){throw 'Install requires -DriverPackageDir and -UserlandExtractDir'}
        if((Get-NpcapService)){throw 'refusing to modify a runner with a pre-existing npcap service'}
        $npf=Join-Path $DriverPackageDir 'NPFInstall.exe'
        $sys=Join-Path $DriverPackageDir 'npcap.sys'
        $cat=Join-Path $DriverPackageDir 'npcap.cat'
        foreach($name in @('NPFInstall.exe','npcap.sys','npcap.cat')){[void](Assert-Hash (Join-Path $DriverPackageDir $name) $ExpectedDriverHashes[$name] "Npcap $name")}
        [void](Assert-NmapSignature $npf 'Npcap NPFInstall.exe')
        [void](Assert-NmapSignature $sys 'Npcap npcap.sys')
        [void](Assert-CatalogSignature $cat)
        $npfVersion=(Get-Item -LiteralPath $npf).VersionInfo.FileVersion
        $sysVersion=(Get-Item -LiteralPath $sys).VersionInfo.FileVersion
        if($npfVersion-ne$NpcapVersion-or$sysVersion-ne$NpcapVersion){throw "Npcap driver package version mismatch: npfinstall=$npfVersion sys=$sysVersion want=$NpcapVersion"}
        $packetSrc=Find-UserlandByHash $UserlandExtractDir 'Packet.dll' $ExpectedPacketHash
        $wpcapSrc=Find-UserlandByHash $UserlandExtractDir 'wpcap.dll' $ExpectedWpcapHash
        [void](Assert-NmapSignature $packetSrc 'Npcap extracted Packet.dll')
        [void](Assert-NmapSignature $wpcapSrc 'Npcap extracted wpcap.dll')
        $installed=$false
        try {
            $proc=Start-Process -FilePath $npf -WorkingDirectory $DriverPackageDir -ArgumentList @('-n','-i') -Wait -PassThru -NoNewWindow
            if($proc.ExitCode-ne0){throw "NPFInstall -n -i exited $($proc.ExitCode)"}
            $installed=$true
            New-Item -ItemType Directory -Force -Path $RuntimeDir|Out-Null
            Copy-Item -LiteralPath $packetSrc -Destination $RuntimePacket -Force
            Copy-Item -LiteralPath $wpcapSrc -Destination $RuntimeWpcap -Force
            $state=[ordered]@{version=$NpcapVersion;installed_by_job=$true;driver_package_dir=(Resolve-Path $DriverPackageDir).Path;runtime_dir=$RuntimeDir;packet_sha256=$ExpectedPacketHash;wpcap_sha256=$ExpectedWpcapHash}
            $state|ConvertTo-Json -Depth 4|Set-Content -LiteralPath $StatePath -Encoding utf8
            Assert-RuntimeReady
            Write-Output "WBD_NPCAP_DRIVER_CI_INSTALL_PASS version=$NpcapVersion state=$StatePath"
        } catch {
            if($installed){& $npf -n -u 2>$null|Out-Null}
            Remove-Item -LiteralPath $RuntimePacket,$RuntimeWpcap -Force -ErrorAction SilentlyContinue
            throw
        }
    }
    'Status' {
        Assert-RuntimeReady
    }
    'Cleanup' {
        if(-not(Test-Path -LiteralPath $StatePath)){
            Write-Output 'WBD_NPCAP_DRIVER_CI_CLEANUP_SKIP reason=no_state'
            exit 0
        }
        $state=Get-Content -LiteralPath $StatePath -Raw|ConvertFrom-Json
        if(-not [bool]$state.installed_by_job){throw 'Npcap CI state exists but does not authorize uninstall'}
        $npf=Join-Path ([string]$state.driver_package_dir) 'NPFInstall.exe'
        if(Test-Path -LiteralPath $npf){
            [void](Assert-Hash $npf $ExpectedDriverHashes['NPFInstall.exe'] 'Npcap NPFInstall.exe cleanup')
            $proc=Start-Process -FilePath $npf -WorkingDirectory ([string]$state.driver_package_dir) -ArgumentList @('-n','-u') -Wait -PassThru -NoNewWindow
            if($proc.ExitCode-ne0){throw "NPFInstall -n -u exited $($proc.ExitCode)"}
        }
        Remove-Item -LiteralPath $RuntimePacket,$RuntimeWpcap -Force -ErrorAction SilentlyContinue
        if(Test-Path -LiteralPath $RuntimeDir){
            $remaining=@(Get-ChildItem -LiteralPath $RuntimeDir -Force -ErrorAction SilentlyContinue)
            if($remaining.Count-eq0){Remove-Item -LiteralPath $RuntimeDir -Force -ErrorAction SilentlyContinue}
        }
        $deadline=[DateTime]::UtcNow.AddSeconds(15)
        do {$svc=Get-NpcapService;if($null-eq$svc){break};Start-Sleep -Milliseconds 250}while([DateTime]::UtcNow-lt$deadline)
        if((Get-NpcapService)){throw 'npcap service remains after CI cleanup'}
        Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
        Write-Output 'WBD_NPCAP_DRIVER_CI_CLEANUP_PASS'
    }
}