param(
    [Parameter(Mandatory = $true)]
    [string]$FakeTCPExe
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Fail([string]$Message) {
    throw "WBD Windows Npcap ABI qualification failed: $Message"
}

$exe = (Resolve-Path -LiteralPath $FakeTCPExe).Path
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$stubSource = Join-Path $repo 'tests\windows_npcap_abi\wpcap_stub.c'
if (-not (Test-Path -LiteralPath $stubSource)) {
    Fail "stub source missing: $stubSource"
}

# The test must exercise the repository stub, never a machine-installed Npcap.
# Physical Npcap capture/injection remains a separate self-hosted release gate.
$systemNpcap = Join-Path $env:SystemRoot 'System32\Npcap\wpcap.dll'
if (Test-Path -LiteralPath $systemNpcap) {
    Fail "hosted ABI test requires a runner without real Npcap; found $systemNpcap"
}

$work = Join-Path $env:RUNNER_TEMP 'wbd-npcap-abi'
if (Test-Path -LiteralPath $work) {
    Remove-Item -LiteralPath $work -Recurse -Force
}
New-Item -ItemType Directory -Path $work | Out-Null

$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path -LiteralPath $vswhere)) {
    Fail 'vswhere.exe not found on hosted Windows runner'
}
$vsInstall = (& $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | Select-Object -First 1)
if ([string]::IsNullOrWhiteSpace($vsInstall)) {
    Fail 'Visual C++ x64 build tools not found'
}
$vcvars = Join-Path $vsInstall 'VC\Auxiliary\Build\vcvars64.bat'
if (-not (Test-Path -LiteralPath $vcvars)) {
    Fail "vcvars64.bat not found: $vcvars"
}

$stubDLL = Join-Path $work 'wpcap.dll'
Push-Location $work
try {
    $compile = "call `"$vcvars`" >nul && cl.exe /nologo /W4 /WX /O2 /LD /Fe:`"$stubDLL`" `"$stubSource`""
    & cmd.exe /d /s /c $compile
    if ($LASTEXITCODE -ne 0) {
        Fail "stub DLL compile exited $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
if (-not (Test-Path -LiteralPath $stubDLL)) {
    Fail 'wpcap.dll stub was not produced'
}

$testExe = Join-Path $work 'wbd-faketcp.exe'
Copy-Item -LiteralPath $exe -Destination $testExe -Force
$marker = Join-Path $work 'stub-marker.log'
$stdout = Join-Path $work 'wbd-faketcp.stdout.log'
$stderr = Join-Path $work 'wbd-faketcp.stderr.log'
$ticket = Join-Path $work 'ticket.tmp'

$oldMarker = $env:WBD_NPCAP_STUB_MARKER
$env:WBD_NPCAP_STUB_MARKER = $marker
$proc = $null
try {
    $args = @(
        'client',
        '--local-udp', '127.0.0.1:45101',
        '--source', '192.0.2.10:41001',
        '--remote', '198.51.100.20:443',
        '--shadow-recovery', 'legacy',
        '--packet-device', '\Device\NPF_{WBD-NPCAP-ABI-STUB}',
        '--source-mac', '02:11:22:33:44:55',
        '--next-hop-mac', '02:aa:bb:cc:dd:ee',
        '--reality-server-name', 'www.speedtest.net',
        '--reality-route-key', '0123456789abcdef0123456789abcdef',
        '--username', 'abi-user',
        '--password', 'abi-password',
        '--verify-server=false',
        '--ticket-out', $ticket,
        '--bootstrap-timeout', '6s'
    )
    $proc = Start-Process -FilePath $testExe -ArgumentList $args -WorkingDirectory $work -NoNewWindow `
        -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru

    $deadline = [DateTime]::UtcNow.AddSeconds(8)
    $passedBoundary = $false
    while ([DateTime]::UtcNow -lt $deadline) {
        if (Test-Path -LiteralPath $marker) {
            $m = Get-Content -LiteralPath $marker -Raw
            if ($m -match '(?m)^TLS_PAYLOAD bytes=[1-9][0-9]* ') {
                $passedBoundary = $true
                break
            }
        }
        if ($proc.HasExited) {
            break
        }
        Start-Sleep -Milliseconds 100
    }

    if (-not $passedBoundary) {
        $outText = if (Test-Path $stdout) { Get-Content -LiteralPath $stdout -Raw } else { '' }
        $errText = if (Test-Path $stderr) { Get-Content -LiteralPath $stderr -Raw } else { '' }
        $markText = if (Test-Path $marker) { Get-Content -LiteralPath $marker -Raw } else { '' }
        Fail "real wbd-faketcp.exe did not emit a TLS bootstrap payload through pcap_sendpacket.`nSTDOUT:`n$outText`nSTDERR:`n$errText`nSTUB:`n$markText"
    }
} finally {
    if ($proc -and -not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        try { $proc.WaitForExit(3000) | Out-Null } catch {}
    }
    if ($null -eq $oldMarker) {
        Remove-Item Env:WBD_NPCAP_STUB_MARKER -ErrorAction SilentlyContinue
    } else {
        $env:WBD_NPCAP_STUB_MARKER = $oldMarker
    }
}

$stubLog = Get-Content -LiteralPath $marker -Raw
$stderrLog = Get-Content -LiteralPath $stderr -Raw
foreach ($required in @(
    'OPEN device=',
    'MODE value=0x0200',
    'SYN_SEEN client_port=41001 server_port=443',
    'NOISE_QUEUED udp=1 wrong_tuple=1 self_frame=1',
    'SYNACK_QUEUED ack=',
    'TLS_PAYLOAD bytes='
)) {
    if (-not $stubLog.Contains($required)) {
        Fail "missing stub evidence '$required'`n$stubLog"
    }
}
if (-not $stderrLog.Contains('WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared')) {
    Fail "real executable did not accept MODE_SENDTORX_CLEAR`n$stderrLog"
}
if ($stderrLog.Contains('faketcp: not ipv4/tcp')) {
    Fail "adapter noise escaped Npcap demultiplexing`n$stderrLog"
}

Write-Output 'WBD_WINDOWS_NPCAP_ABI_PASS real_exe=1 dynamic_wpcap=1 open_live=1 setmode=1 next_ex=1 sendpacket=1 adapter_noise=ignored synack=accepted tls_payload_tx=1 physical_driver=0'
