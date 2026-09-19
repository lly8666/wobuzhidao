param(
    [Parameter(Mandatory = $true)]
    [string]$WbdExe,

    [Parameter(Mandatory = $true)]
    [string]$Profile,

    [string]$LogPath = "",

    [switch]$UninstallNpcapAfter,

    [string]$NpcapPrepareScript = (Join-Path $PSScriptRoot 'windows_npcap_prepare.ps1')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Physical Npcap qualification must run from an elevated Administrator session/runner.'
    }
}

function Assert-Path([string]$Path, [string]$Label) {
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "$Label not found: $Path"
    }
}

function Assert-Event([object[]]$Rows, [string]$Name) {
    if (-not ($Rows | Where-Object { $_.event -eq $Name } | Select-Object -First 1)) {
        throw "qualification log missing event: $Name"
    }
}

function Assert-ChildMarker([object[]]$Rows, [string]$Command, [string]$Pattern) {
    $match = $Rows | Where-Object {
        $_.event -eq 'child_log' -and
        $_.command -eq $Command -and
        ([string]$_.text) -match $Pattern
    } | Select-Object -First 1
    if (-not $match) {
        throw "qualification log missing child marker: command=$Command pattern=$Pattern"
    }
}

Assert-Administrator
$WbdExe = (Resolve-Path -LiteralPath $WbdExe).Path
$Profile = (Resolve-Path -LiteralPath $Profile).Path
$NpcapPrepareScript = (Resolve-Path -LiteralPath $NpcapPrepareScript).Path
Assert-Path -Path $WbdExe -Label 'WBD portable EXE'
Assert-Path -Path $Profile -Label 'WBD profile'

if ([string]::IsNullOrWhiteSpace($LogPath)) {
    $dir = Join-Path $env:TEMP 'WBD-physical-qualification'
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $LogPath = Join-Path $dir ("npcap-physical-{0}.jsonl" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
} else {
    $parent = Split-Path -Parent $LogPath
    if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
}

$oldHeadless = $env:WBD_HEADLESS
$qualificationError = $null
$uninstallError = $null

try {
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $NpcapPrepareScript -Action Status
    if ($LASTEXITCODE -ne 0) {
        throw "Npcap status preflight failed with exit code $LASTEXITCODE"
    }

    $env:WBD_HEADLESS = '1'
    $proc = Start-Process -FilePath $WbdExe -ArgumentList @(
        '-self-test',
        '-profile', $Profile,
        '-self-test-log', $LogPath
    ) -Wait -PassThru
    if ($proc.ExitCode -ne 0) {
        throw "WBD physical self-test exited $($proc.ExitCode); log=$LogPath"
    }
    if (-not (Test-Path -LiteralPath $LogPath)) {
        throw "WBD physical self-test produced no JSONL log: $LogPath"
    }

    $rows = @()
    foreach ($line in (Get-Content -LiteralPath $LogPath)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $rows += ($line | ConvertFrom-Json)
    }
    if ($rows.Count -eq 0) { throw "WBD physical self-test JSONL is empty: $LogPath" }

    foreach ($required in @(
        'dependency_preflight_pass',
        'underlay_pass',
        'connect_pass',
        'probe_system_dns_pass',
        'probe_udp_pass',
        'probe_tcp_pass',
        'cleanup_pass',
        'self_test_pass'
    )) {
        Assert-Event -Rows $rows -Name $required
    }

    # The retired architecture launched a separate kernel-TCP Reality bootstrap
    # before starting FakeTCP. ADR-0014 requires TLS/Reality-like admission to
    # happen inside the one public FakeTCP association, so seeing that old child
    # is a hard qualification failure even if later probes happen to pass.
    $retiredBootstrap = $rows | Where-Object {
        ($_.PSObject.Properties.Name -contains 'name' -and $_.name -eq 'reality-bootstrap') -or
        ($_.PSObject.Properties.Name -contains 'command' -and $_.command -eq 'reality-bootstrap')
    } | Select-Object -First 1
    if ($retiredBootstrap) {
        throw 'single-flow qualification observed retired standalone reality-bootstrap command'
    }

    # These markers prove the physical NPF path opened, FakeTCP itself completed
    # Reality-like TLS/auth on the same public flow, payload actually crossed the
    # Npcap TX and RX boundaries, and only then DTLS/LINK/TUN became usable.
    Assert-ChildMarker -Rows $rows -Command 'faketcp' -Pattern '^WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY\s+sendtorx=cleared\b'
    Assert-ChildMarker -Rows $rows -Command 'faketcp' -Pattern '^WBD_SINGLE_FLOW_BOOTSTRAP_READY\b.*\bsame_flow=1\b.*\blogical_tunnel=1\b'
    Assert-ChildMarker -Rows $rows -Command 'faketcp' -Pattern '^READY role=client\b.*\bsingle_flow_bootstrap=true\b'
    Assert-ChildMarker -Rows $rows -Command 'faketcp' -Pattern '^WBD_FAKETCP_WINDOWS_RAW_PAYLOAD_TX\b'
    Assert-ChildMarker -Rows $rows -Command 'faketcp' -Pattern '^WBD_FAKETCP_WINDOWS_RAW_PAYLOAD_RX\b'
    Assert-ChildMarker -Rows $rows -Command 'dtls' -Pattern '^READY role=client\b'
    Assert-ChildMarker -Rows $rows -Command 'link' -Pattern '^WBD_LINK_READY role=client\b'
    Assert-ChildMarker -Rows $rows -Command 'tun' -Pattern '^WBD_TUN_READY mode=client\b'

    $bad = $rows | Where-Object { $_.event -match '(^fail$|_fail$)' } | Select-Object -First 1
    if ($bad) { throw "qualification log contains failure event: $($bad.event)" }

    Write-Output "WBD_WINDOWS_NPCAP_PHYSICAL_PASS log=$LogPath npcap_driver=1 capture_injection=1 raw_tx=1 raw_rx=1 single_public_flow=1 reality_like_same_flow=1 dtls=1 link=1 tun=1 probes=1 cleanup=1"
}
catch {
    $qualificationError = $_
    Write-Error $_
}
finally {
    if ($null -eq $oldHeadless) {
        Remove-Item Env:WBD_HEADLESS -ErrorAction SilentlyContinue
    } else {
        $env:WBD_HEADLESS = $oldHeadless
    }

    if ($UninstallNpcapAfter) {
        try {
            & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $NpcapPrepareScript -Action Uninstall
            if ($LASTEXITCODE -ne 0) { throw "Npcap uninstall failed with exit code $LASTEXITCODE" }
        }
        catch {
            $uninstallError = $_
            Write-Error $_
        }
    }
}

if ($qualificationError -and $uninstallError) {
    throw "physical qualification failed: $($qualificationError.Exception.Message); Npcap cleanup also failed: $($uninstallError.Exception.Message)"
}
if ($qualificationError) { throw $qualificationError }
if ($uninstallError) { throw $uninstallError }
