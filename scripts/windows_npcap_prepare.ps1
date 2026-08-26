param(
    [ValidateSet('Status','Fetch','Install')]
    [string]$Action = 'Status',
    [string]$DownloadDirectory = "$env:LOCALAPPDATA\WBD\downloads"
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Release-authoritative dependency lock. The Free Edition is downloaded by the
# operator directly from npcap.com and is never committed to or redistributed
# by WBD. Npcap's public license permits limited internal/personal use but does
# not grant WBD redistribution rights for the Free Edition.
$NpcapVersion = '1.88'
$NpcapURL = "https://npcap.com/dist/npcap-$NpcapVersion.exe"
$NpcapInstaller = Join-Path $DownloadDirectory "npcap-$NpcapVersion.exe"
$ExpectedSigner = 'Nmap Software LLC'

function Get-NpcapRuntimeState {
    $system32 = Join-Path $env:SystemRoot 'System32\Npcap'
    $wpcap = Join-Path $system32 'wpcap.dll'
    $packet = Join-Path $system32 'Packet.dll'
    $service = Get-Service -Name 'npcap' -ErrorAction SilentlyContinue
    [pscustomobject]@{
        Wpcap = $wpcap
        Packet = $packet
        WpcapExists = Test-Path -LiteralPath $wpcap
        PacketExists = Test-Path -LiteralPath $packet
        ServiceExists = $null -ne $service
        ServiceStatus = if ($service) { [string]$service.Status } else { 'Missing' }
    }
}

function Assert-TrustedNmapSignature([string]$Path, [string]$Label) {
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "$Label not found: $Path"
    }
    $signature = Get-AuthenticodeSignature -FilePath $Path
    if ($signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "$Label Authenticode signature is not valid: status=$($signature.Status) path=$Path"
    }
    $subject = [string]$signature.SignerCertificate.Subject
    if ($subject -notmatch [regex]::Escape($ExpectedSigner)) {
        throw "$Label signer mismatch: subject=$subject expected=$ExpectedSigner"
    }
    return $signature
}

function Assert-NpcapInstalled {
    $state = Get-NpcapRuntimeState
    if (-not $state.WpcapExists -or -not $state.PacketExists -or -not $state.ServiceExists) {
        throw "Npcap $NpcapVersion runtime is not ready. Expected $($state.Wpcap), $($state.Packet), and the npcap driver service. Run this script with -Action Install."
    }
    [void](Assert-TrustedNmapSignature -Path $state.Wpcap -Label 'Npcap wpcap.dll')
    [void](Assert-TrustedNmapSignature -Path $state.Packet -Label 'Npcap Packet.dll')
    Write-Output "WBD_WINDOWS_NPCAP_READY version=$NpcapVersion service=$($state.ServiceStatus) wpcap=$($state.Wpcap)"
    return $state
}

function Fetch-NpcapInstaller {
    if (-not (Test-Path -LiteralPath $DownloadDirectory)) {
        New-Item -ItemType Directory -Force -Path $DownloadDirectory | Out-Null
    }
    Invoke-WebRequest -UseBasicParsing -Uri $NpcapURL -OutFile $NpcapInstaller
    $signature = Assert-TrustedNmapSignature -Path $NpcapInstaller -Label "Npcap $NpcapVersion installer"
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $NpcapInstaller).Hash.ToLowerInvariant()
    Write-Output "WBD_WINDOWS_NPCAP_FETCH_PASS version=$NpcapVersion signer=$($signature.SignerCertificate.Subject) sha256=$hash path=$NpcapInstaller"
    return $NpcapInstaller
}

switch ($Action) {
    'Status' {
        [void](Assert-NpcapInstalled)
        exit 0
    }
    'Fetch' {
        [void](Fetch-NpcapInstaller)
        exit 0
    }
    'Install' {
        $installer = Fetch-NpcapInstaller | Select-Object -Last 1
        # The public/free Npcap installer intentionally has no unattended /S
        # entitlement. Start it in the normal graphical mode and wait for the
        # operator to complete or cancel installation.
        $proc = Start-Process -FilePath $installer -Wait -PassThru
        if ($proc.ExitCode -notin @(0, 3010)) {
            throw "Npcap installer exited $($proc.ExitCode)"
        }
        [void](Assert-NpcapInstalled)
        if ($proc.ExitCode -eq 3010) {
            Write-Output 'WBD_WINDOWS_NPCAP_REBOOT_REQUIRED'
        }
        Write-Output "WBD_WINDOWS_NPCAP_INSTALL_PASS version=$NpcapVersion"
        exit 0
    }
}
