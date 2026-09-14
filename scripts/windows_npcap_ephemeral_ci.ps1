param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('Install','Status','Cleanup')]
    [string]$Action,

    [string]$InstallerPath = '',
    [string]$InstallerSHA256 = '',
    [string]$StatePath = "$env:RUNNER_TEMP\wbd-npcap-ephemeral-state.json"
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Get-NpcapState {
    $root = Join-Path $env:SystemRoot 'System32\Npcap'
    $service = Get-Service -Name npcap -ErrorAction SilentlyContinue
    [pscustomobject]@{
        Root = $root
        Wpcap = Join-Path $root 'wpcap.dll'
        Packet = Join-Path $root 'Packet.dll'
        ServiceExists = $null -ne $service
        ServiceStatus = if ($service) { [string]$service.Status } else { 'Missing' }
    }
}

function Assert-NmapSignature([string]$Path, [string]$Label) {
    if (-not (Test-Path -LiteralPath $Path)) { throw "$Label missing: $Path" }
    $sig = Get-AuthenticodeSignature -FilePath $Path
    if ($sig.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "$Label Authenticode signature invalid: $($sig.Status)"
    }
    $subject = [string]$sig.SignerCertificate.Subject
    if ($subject -notmatch 'Nmap Software LLC') {
        throw "$Label signer mismatch: $subject"
    }
}

function Assert-NpcapReady {
    $s = Get-NpcapState
    if (-not $s.ServiceExists -or -not (Test-Path -LiteralPath $s.Wpcap) -or -not (Test-Path -LiteralPath $s.Packet)) {
        throw 'Npcap runtime is not ready'
    }
    Assert-NmapSignature $s.Wpcap 'Npcap wpcap.dll'
    Assert-NmapSignature $s.Packet 'Npcap Packet.dll'
    Write-Output "WBD_WINDOWS_NPCAP_EPHEMERAL_READY service=$($s.ServiceStatus) wpcap=$($s.Wpcap)"
    return $s
}

function Save-State([bool]$InstalledByJob, [string]$Installer) {
    $dir = Split-Path -Parent $StatePath
    if ($dir) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    [ordered]@{
        installed_by_job = $InstalledByJob
        installer = $Installer
        created_utc = [DateTime]::UtcNow.ToString('o')
    } | ConvertTo-Json | Set-Content -LiteralPath $StatePath -Encoding utf8
}

function Resolve-Uninstaller {
    $candidates = @(
        (Join-Path $env:ProgramFiles 'Npcap\uninstall.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Npcap\uninstall.exe')
    )
    foreach ($p in $candidates) {
        if ($p -and (Test-Path -LiteralPath $p)) { return $p }
    }
    $roots = @(
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    foreach ($root in $roots) {
        $entry = Get-ItemProperty $root -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -match '^Npcap\b' } | Select-Object -First 1
        if ($entry -and $entry.UninstallString) {
            $raw = [string]$entry.UninstallString
            if ($raw -match '^\s*"([^"]+)"') { return $Matches[1] }
            if ($raw -match '^\s*([^ ]+\.exe)') { return $Matches[1] }
        }
    }
    throw 'Npcap uninstaller not found'
}

switch ($Action) {
    'Status' {
        [void](Assert-NpcapReady)
        exit 0
    }
    'Install' {
        $existing = Get-NpcapState
        if ($existing.ServiceExists -and (Test-Path -LiteralPath $existing.Wpcap) -and (Test-Path -LiteralPath $existing.Packet)) {
            [void](Assert-NpcapReady)
            Save-State $false ''
            Write-Output 'WBD_WINDOWS_NPCAP_EPHEMERAL_INSTALL_SKIP reason=preexisting'
            exit 0
        }
        if ([string]::IsNullOrWhiteSpace($InstallerPath) -or -not (Test-Path -LiteralPath $InstallerPath)) {
            throw 'authorized Npcap OEM/internal-use installer is required for unattended CI'
        }
        if ([string]::IsNullOrWhiteSpace($InstallerSHA256)) {
            throw 'authorized Npcap installer SHA256 is required'
        }
        $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $InstallerPath).Hash.ToLowerInvariant()
        $want = $InstallerSHA256.Trim().ToLowerInvariant()
        if ($got -ne $want) { throw "Npcap installer SHA256 mismatch: got=$got want=$want" }
        Assert-NmapSignature $InstallerPath 'Npcap installer'
        $p = Start-Process -FilePath $InstallerPath -ArgumentList '/S' -Wait -PassThru
        if ($p.ExitCode -notin @(0,3010)) { throw "Npcap silent installer exited $($p.ExitCode)" }
        [void](Assert-NpcapReady)
        Save-State $true $InstallerPath
        Write-Output "WBD_WINDOWS_NPCAP_EPHEMERAL_INSTALL_PASS sha256=$got reboot_required=$($p.ExitCode -eq 3010)"
        exit 0
    }
    'Cleanup' {
        if (-not (Test-Path -LiteralPath $StatePath)) {
            Write-Output 'WBD_WINDOWS_NPCAP_EPHEMERAL_CLEANUP_SKIP reason=no_state'
            exit 0
        }
        $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
        if (-not [bool]$state.installed_by_job) {
            Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
            Write-Output 'WBD_WINDOWS_NPCAP_EPHEMERAL_CLEANUP_SKIP reason=preexisting'
            exit 0
        }
        $uninstaller = Resolve-Uninstaller
        Assert-NmapSignature $uninstaller 'Npcap uninstaller'
        $p = Start-Process -FilePath $uninstaller -ArgumentList '/S' -Wait -PassThru
        if ($p.ExitCode -notin @(0,3010)) { throw "Npcap silent uninstaller exited $($p.ExitCode)" }
        for ($i = 0; $i -lt 30; $i++) {
            if (-not (Get-Service -Name npcap -ErrorAction SilentlyContinue)) { break }
            Start-Sleep -Seconds 1
        }
        if (Get-Service -Name npcap -ErrorAction SilentlyContinue) { throw 'Npcap service survived cleanup' }
        Remove-Item -LiteralPath $StatePath -Force -ErrorAction SilentlyContinue
        Write-Output "WBD_WINDOWS_NPCAP_EPHEMERAL_CLEANUP_PASS reboot_required=$($p.ExitCode -eq 3010)"
        exit 0
    }
}
