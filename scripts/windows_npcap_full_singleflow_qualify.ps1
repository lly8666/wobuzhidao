param(
    [Parameter(Mandatory = $true)]
    [string]$FakeTCPExe
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Fail([string]$Message) {
    throw "WBD Windows full single-flow qualification failed: $Message"
}

function ReadText([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        return ''
    }
    $value = Get-Content -LiteralPath $Path -Raw
    if ($null -eq $value) {
        return ''
    }
    return [string]$value
}

$exe = (Resolve-Path -LiteralPath $FakeTCPExe).Path
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$bridgeSource = Join-Path $repo 'tests\windows_npcap_abi\wpcap_bridge_stub.c'
if (-not (Test-Path -LiteralPath $bridgeSource)) {
    Fail "bridge stub source missing: $bridgeSource"
}

$systemNpcap = Join-Path $env:SystemRoot 'System32\Npcap\wpcap.dll'
if (Test-Path -LiteralPath $systemNpcap) {
    Fail "hosted full single-flow test requires no real Npcap; found $systemNpcap"
}

$work = Join-Path $env:RUNNER_TEMP 'wbd-npcap-full-singleflow'
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
    $compile = "call `"$vcvars`" >nul && cl.exe /nologo /W4 /WX /O2 /LD /Fe:`"$stubDLL`" `"$bridgeSource`" /link ws2_32.lib"
    & cmd.exe /d /s /c $compile
    if ($LASTEXITCODE -ne 0) {
        Fail "bridge stub DLL compile exited $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
if (-not (Test-Path -LiteralPath $stubDLL)) {
    Fail 'bridge wpcap.dll was not produced'
}

$serverExe = Join-Path $work 'wbd-npcap-full-server.exe'
Push-Location $repo
try {
    & go build -trimpath -o $serverExe ./tests/windows_npcap_abi/full_server
    if ($LASTEXITCODE -ne 0) {
        Fail "full server helper build exited $LASTEXITCODE"
    }
} finally {
    Pop-Location
}
if (-not (Test-Path -LiteralPath $serverExe)) {
    Fail 'full server helper was not produced'
}

$testExe = Join-Path $work 'wbd-faketcp.exe'
Copy-Item -LiteralPath $exe -Destination $testExe -Force
$serverOut = Join-Path $work 'server.stdout.log'
$serverErr = Join-Path $work 'server.stderr.log'
$clientOut = Join-Path $work 'wbd-faketcp.stdout.log'
$clientErr = Join-Path $work 'wbd-faketcp.stderr.log'
$ticket = Join-Path $work 'ticket.tmp'
$bridgePort = 48188

$server = $null
$client = $null
$udp = $null
$oldBridgePort = $env:WBD_NPCAP_BRIDGE_PORT
try {
    $serverArgs = @(
        '--listen', "127.0.0.1:$bridgePort",
        '--server-name', 'www.speedtest.net',
        '--route-key', '0123456789abcdef0123456789abcdef',
        '--username', 'abi-user',
        '--password', 'abi-password'
    )
    $server = Start-Process -FilePath $serverExe -ArgumentList $serverArgs -WorkingDirectory $work -NoNewWindow `
        -RedirectStandardOutput $serverOut -RedirectStandardError $serverErr -PassThru

    $deadline = [DateTime]::UtcNow.AddSeconds(5)
    $serverReady = $false
    while ([DateTime]::UtcNow -lt $deadline) {
        $text = ReadText $serverOut
        if ($text.Contains('WBD_NPCAP_FULL_SERVER_READY')) {
            $serverReady = $true
            break
        }
        if ($server.HasExited) { break }
        Start-Sleep -Milliseconds 50
    }
    if (-not $serverReady) {
        $so = ReadText $serverOut
        $se = ReadText $serverErr
        Fail "bridge server did not become ready.`nSTDOUT:`n$so`nSTDERR:`n$se"
    }

    $env:WBD_NPCAP_BRIDGE_PORT = [string]$bridgePort
    $args = @(
        'client',
        '--local-udp', '127.0.0.1:45101',
        '--source', '192.0.2.10:41001',
        '--remote', '198.51.100.20:443',
        '--shadow-recovery', 'legacy',
        '--packet-device', '\Device\NPF_{WBD-NPCAP-FULL-BRIDGE}',
        '--source-mac', '02:11:22:33:44:55',
        '--next-hop-mac', '02:aa:bb:cc:dd:ee',
        '--reality-server-name', 'www.speedtest.net',
        '--reality-route-key', '0123456789abcdef0123456789abcdef',
        '--username', 'abi-user',
        '--password', 'abi-password',
        '--verify-server=false',
        '--ticket-out', $ticket,
        '--bootstrap-timeout', '10s'
    )
    $client = Start-Process -FilePath $testExe -ArgumentList $args -WorkingDirectory $work -NoNewWindow `
        -RedirectStandardOutput $clientOut -RedirectStandardError $clientErr -PassThru

    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    $datagramReady = $false
    while ([DateTime]::UtcNow -lt $deadline) {
        $text = ReadText $clientErr
        if ($text.Contains('WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only')) {
            $datagramReady = $true
            break
        }
        if ($client.HasExited -or $server.HasExited) { break }
        Start-Sleep -Milliseconds 50
    }
    if (-not $datagramReady) {
        $co = ReadText $clientOut
        $ce = ReadText $clientErr
        $so = ReadText $serverOut
        $se = ReadText $serverErr
        Fail "real Windows FakeTCP process did not reach same-flow datagram phase.`nCLIENT STDOUT:`n$co`nCLIENT STDERR:`n$ce`nSERVER STDOUT:`n$so`nSERVER STDERR:`n$se"
    }

    if (-not (Test-Path -LiteralPath $ticket)) {
        Fail 'single-flow ticket file was not created'
    }
    $ticketText = (ReadText $ticket).Trim()
    if ($ticketText -notmatch '^[0-9a-fA-F]{64}$') {
        Fail "invalid ticket output: $ticketText"
    }

    $udp = [System.Net.Sockets.UdpClient]::new(0)
    $udp.Client.ReceiveTimeout = 4000
    $payload = [System.Text.Encoding]::ASCII.GetBytes('wbd-hosted-singleflow-post-switch-echo')
    $sent = $udp.Send($payload, $payload.Length, '127.0.0.1', 45101)
    if ($sent -ne $payload.Length) {
        Fail "post-switch UDP send length mismatch: $sent"
    }
    $remote = [System.Net.IPEndPoint]::new([System.Net.IPAddress]::Any, 0)
    $reply = $udp.Receive([ref]$remote)
    $replyText = [System.Text.Encoding]::ASCII.GetString($reply)
    if ($replyText -ne 'wbd-hosted-singleflow-post-switch-echo') {
        Fail "post-switch echo mismatch: $replyText"
    }

    $deadline = [DateTime]::UtcNow.AddSeconds(3)
    $serverEcho = $false
    while ([DateTime]::UtcNow -lt $deadline) {
        $text = ReadText $serverOut
        if ($text.Contains('WBD_NPCAP_FULL_SERVER_DATAGRAM_ECHO bytes=')) {
            $serverEcho = $true
            break
        }
        Start-Sleep -Milliseconds 50
    }
    if (-not $serverEcho) {
        Fail 'server did not record the post-switch datagram echo'
    }
} finally {
    if ($udp) { $udp.Dispose() }
    foreach ($p in @($client, $server)) {
        if ($p -and -not $p.HasExited) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
            try { $p.WaitForExit(3000) | Out-Null } catch {}
        }
    }
    if ($null -eq $oldBridgePort) {
        Remove-Item Env:WBD_NPCAP_BRIDGE_PORT -ErrorAction SilentlyContinue
    } else {
        $env:WBD_NPCAP_BRIDGE_PORT = $oldBridgePort
    }
}

$clientStdout = ReadText $clientOut
$clientStderr = ReadText $clientErr
$serverStdout = ReadText $serverOut
foreach ($required in @(
    'READY role=client'
)) {
    if (-not $clientStdout.Contains($required)) {
        Fail "missing client stdout evidence '$required'`n$clientStdout"
    }
}
foreach ($required in @(
    'WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared',
    'WBD_SINGLEFLOW_TLS_SWITCH_REQUEST_SENT',
    'WBD_SINGLEFLOW_TLS_SWITCH_ACK_RECEIVED',
    'WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only'
)) {
    if (-not $clientStderr.Contains($required)) {
        Fail "missing client stderr evidence '$required'`n$clientStderr"
    }
}
if ($clientStderr.Contains('faketcp: not ipv4/tcp')) {
    Fail "strict parser received non-flow adapter traffic`n$clientStderr"
}
foreach ($required in @(
    'WBD_NPCAP_FULL_SERVER_AUTH_OK tls=1.3',
    'WBD_NPCAP_FULL_SERVER_SWITCH_REQUEST_OK',
    'WBD_NPCAP_FULL_SERVER_SWITCH_ACK_SENT',
    'WBD_NPCAP_FULL_SERVER_DATAGRAM_READY hol=bootstrap-only',
    'WBD_NPCAP_FULL_SERVER_DATAGRAM_ECHO bytes='
)) {
    if (-not $serverStdout.Contains($required)) {
        Fail "missing server evidence '$required'`n$serverStdout"
    }
}

Write-Output 'WBD_WINDOWS_NPCAP_FULL_SINGLEFLOW_PASS real_exe=1 dynamic_wpcap=1 tls_auth=1 encrypted_switch=1 datagram_echo=1 same_flow=1 physical_driver=0'
