param(
    [Parameter(Mandatory=$true)]
    [string]$AssetDirectory,
    [Parameter(Mandatory=$true)]
    [string]$ProfilePath,
    [Parameter(Mandatory=$true)]
    [string]$DnsServer,
    [Parameter(Mandatory=$true)]
    [string]$DnsName,
    [Parameter(Mandatory=$true)]
    [string]$TcpHost,
    [Parameter(Mandatory=$true)]
    [ValidateRange(1,65535)]
    [int]$TcpPort,
    [string]$TcpPayload = '',
    [string]$TcpExpect = '',
    [Parameter(Mandatory=$true)]
    [string]$UdpHost,
    [Parameter(Mandatory=$true)]
    [ValidateRange(1,65535)]
    [int]$UdpPort,
    [Parameter(Mandatory=$true)]
    [string]$UdpPayload,
    [string]$UdpExpect = '',
    [ValidateRange(5,120)]
    [int]$ReadyTimeoutSeconds = 45,
    [ValidateRange(1,30)]
    [int]$ProbeTimeoutSeconds = 8,
    [string]$LogDirectory = "$env:TEMP\wbd-windows-frozen-one-shot"
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Require-Admin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Windows frozen-WBD one-shot requires an elevated PowerShell session'
    }
}

function Resolve-IPv4([string]$Value, [string]$Label) {
    $ip = $null
    if ([System.Net.IPAddress]::TryParse($Value, [ref]$ip)) {
        if ($ip.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
            throw "$Label must resolve to IPv4 for the current Windows release gate: $Value"
        }
        return $ip
    }
    $addresses = [System.Net.Dns]::GetHostAddresses($Value) | Where-Object {
        $_.AddressFamily -eq [System.Net.Sockets.AddressFamily]::InterNetwork
    }
    if (-not $addresses) {
        throw "$Label did not resolve to IPv4: $Value"
    }
    return $addresses[0]
}

function Wait-Log([string]$Path, [string]$Pattern, [int]$TimeoutSeconds) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        if ((Test-Path -LiteralPath $Path) -and ((Get-Content -LiteralPath $Path -Raw) -match $Pattern)) {
            return
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    $text = if (Test-Path -LiteralPath $Path) { Get-Content -LiteralPath $Path -Raw } else { '<missing>' }
    throw "timeout waiting for $Pattern in $Path; log=$text"
}

function Invoke-DnsProbe([string]$Server, [string]$Name) {
    $answers = @(Resolve-DnsName -Name $Name -Server $Server -DnsOnly -Type A -ErrorAction Stop | Where-Object { $_.IPAddress })
    if ($answers.Count -eq 0) {
        throw "DNS probe returned no A record: name=$Name server=$Server"
    }
    Write-Output "WBD_WINDOWS_FROZEN_DNS_PASS server=$Server name=$Name answer=$($answers[0].IPAddress)"
}

function Invoke-TcpProbe([string]$HostName, [int]$Port, [string]$Payload, [string]$Expect, [int]$TimeoutSeconds) {
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $connect = $client.ConnectAsync($HostName, $Port)
        if (-not $connect.Wait([TimeSpan]::FromSeconds($TimeoutSeconds))) {
            throw "TCP connect timeout: $HostName`:$Port"
        }
        if ($connect.IsFaulted) { throw $connect.Exception }
        $stream = $client.GetStream()
        $stream.ReadTimeout = $TimeoutSeconds * 1000
        $stream.WriteTimeout = $TimeoutSeconds * 1000
        $response = ''
        if ($Payload -ne '') {
            $bytes = [Text.Encoding]::UTF8.GetBytes($Payload)
            $stream.Write($bytes, 0, $bytes.Length)
            $stream.Flush()
            if ($Expect -ne '') {
                $buffer = New-Object byte[] 65535
                $n = $stream.Read($buffer, 0, $buffer.Length)
                $response = [Text.Encoding]::UTF8.GetString($buffer, 0, $n)
                if ($response -notlike "*$Expect*") {
                    throw "TCP response did not contain expected text '$Expect': $response"
                }
            }
        }
        Write-Output "WBD_WINDOWS_FROZEN_TCP_PASS host=$HostName port=$Port bytes_sent=$([Text.Encoding]::UTF8.GetByteCount($Payload)) bytes_received=$([Text.Encoding]::UTF8.GetByteCount($response))"
    } finally {
        $client.Dispose()
    }
}

function Invoke-UdpProbe([string]$HostName, [int]$Port, [string]$Payload, [string]$Expect, [int]$TimeoutSeconds) {
    $remoteIP = Resolve-IPv4 $HostName 'UdpHost'
    $client = [System.Net.Sockets.UdpClient]::new([System.Net.Sockets.AddressFamily]::InterNetwork)
    try {
        $client.Client.ReceiveTimeout = $TimeoutSeconds * 1000
        $bytes = [Text.Encoding]::UTF8.GetBytes($Payload)
        [void]$client.Send($bytes, $bytes.Length, [System.Net.IPEndPoint]::new($remoteIP, $Port))
        $peer = [System.Net.IPEndPoint]::new([System.Net.IPAddress]::Any, 0)
        $reply = $client.Receive([ref]$peer)
        $text = [Text.Encoding]::UTF8.GetString($reply)
        if ($Expect -ne '' -and $text -notlike "*$Expect*") {
            throw "UDP response did not contain expected text '$Expect': $text"
        }
        Write-Output "WBD_WINDOWS_FROZEN_UDP_PASS host=$HostName port=$Port bytes_sent=$($bytes.Length) bytes_received=$($reply.Length)"
    } finally {
        $client.Dispose()
    }
}

Require-Admin
$AssetDirectory = [IO.Path]::GetFullPath($AssetDirectory)
$ProfilePath = [IO.Path]::GetFullPath($ProfilePath)
$qualifier = Join-Path $AssetDirectory 'wbd-windows-qualify.exe'
$npcapPrepare = Join-Path $AssetDirectory 'windows_npcap_prepare.ps1'
$routeScript = Join-Path $AssetDirectory 'windows_tun_route.ps1'
foreach ($path in @($ProfilePath, $qualifier, $npcapPrepare, $routeScript)) {
    if (-not (Test-Path -LiteralPath $path)) { throw "required Windows one-shot asset missing: $path" }
}

# Validate probe address families before mutating any WBD state.
[void](Resolve-IPv4 $DnsServer 'DnsServer')
[void](Resolve-IPv4 $TcpHost 'TcpHost')
[void](Resolve-IPv4 $UdpHost 'UdpHost')

& $npcapPrepare -Action Status
if ($LASTEXITCODE -ne 0) { throw "Npcap preflight exited $LASTEXITCODE" }

$stateDir = Join-Path $env:ProgramData 'WBD\qualification'
$routeState = Join-Path $stateDir 'route-state.json'
if (Test-Path -LiteralPath $routeState) {
    Write-Output 'WBD_WINDOWS_FROZEN_PRE_CLEANUP stale_state=1'
    & $routeScript -Action Cleanup -StatePath $routeState
    if ($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $routeState)) {
        throw 'failed to remove stale WBD qualification route state before one-shot'
    }
}

if (-not (Test-Path -LiteralPath $LogDirectory)) {
    New-Item -ItemType Directory -Force -Path $LogDirectory | Out-Null
}
$stdout = Join-Path $LogDirectory 'qualifier.stdout.log'
$stderr = Join-Path $LogDirectory 'qualifier.stderr.log'
$stopFile = Join-Path $LogDirectory 'stop.request'
Remove-Item -LiteralPath $stdout,$stderr,$stopFile -Force -ErrorAction SilentlyContinue

$proc = $null
$connected = $false
$ifIndex = $null
try {
    # Start-Process joins ArgumentList into one Windows command line. Quote path
    # arguments explicitly so a normal Program Files/profile path cannot be
    # split into extra flags.
    $profileArg = '"' + $ProfilePath + '"'
    $stopFileArg = '"' + $stopFile + '"'
    $proc = Start-Process -FilePath $qualifier -ArgumentList @(
        '-profile', $profileArg,
        '-run-for', '2m',
        '-stop-file', $stopFileArg
    ) -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr

    $deadline = [DateTime]::UtcNow.AddSeconds($ReadyTimeoutSeconds)
    do {
        $proc.Refresh()
        if ($proc.HasExited) {
            if (Test-Path -LiteralPath $stdout) { Get-Content -LiteralPath $stdout }
            if (Test-Path -LiteralPath $stderr) { Get-Content -LiteralPath $stderr }
            throw "WBD Windows qualifier exited before CONNECTED: exit=$($proc.ExitCode)"
        }
        if ((Test-Path -LiteralPath $stdout) -and ((Get-Content -LiteralPath $stdout -Raw) -match 'WBD_WINDOWS_QUALIFY_CONNECTED')) {
            $connected = $true
            break
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    if (-not $connected) {
        throw 'timed out waiting for WBD_WINDOWS_QUALIFY_CONNECTED'
    }

    if (-not (Test-Path -LiteralPath $routeState)) {
        throw 'qualifier reported connected without persisted WBD route state'
    }
    $route = Get-Content -LiteralPath $routeState -Raw | ConvertFrom-Json
    $ifIndex = [uint32]$route.AdapterInterfaceIndex
    $prefixes = @($route.CaptureRoutes | ForEach-Object { [string]$_.DestinationPrefix })
    if ($prefixes -notcontains '0.0.0.0/1' -or $prefixes -notcontains '128.0.0.0/1') {
        throw "Full capture routes are incomplete: $($prefixes -join ',')"
    }
    Write-Output "WBD_WINDOWS_FROZEN_CAPTURE_PASS ifindex=$ifIndex prefixes=$($prefixes -join ',')"

    Invoke-DnsProbe -Server $DnsServer -Name $DnsName
    Invoke-TcpProbe -HostName $TcpHost -Port $TcpPort -Payload $TcpPayload -Expect $TcpExpect -TimeoutSeconds $ProbeTimeoutSeconds
    Invoke-UdpProbe -HostName $UdpHost -Port $UdpPort -Payload $UdpPayload -Expect $UdpExpect -TimeoutSeconds $ProbeTimeoutSeconds

    New-Item -ItemType File -Force -Path $stopFile | Out-Null
    if (-not $proc.WaitForExit(30000)) {
        throw 'qualifier did not complete automatic cleanup within 30 seconds after stop request'
    }
    $proc.Refresh()
    if ($proc.ExitCode -ne 0) {
        if (Test-Path -LiteralPath $stdout) { Get-Content -LiteralPath $stdout }
        if (Test-Path -LiteralPath $stderr) { Get-Content -LiteralPath $stderr }
        throw "qualifier exited $($proc.ExitCode)"
    }
    Wait-Log -Path $stdout -Pattern 'WBD_WINDOWS_QUALIFY_CLEANUP_PASS' -TimeoutSeconds 2
    if (Test-Path -LiteralPath $routeState) { throw 'WBD route-state survived qualifier cleanup' }
    if ($null -ne $ifIndex) {
        foreach ($prefix in @('0.0.0.0/1','128.0.0.0/1')) {
            if (Get-NetRoute -DestinationPrefix $prefix -InterfaceIndex $ifIndex -PolicyStore ActiveStore -ErrorAction SilentlyContinue) {
                throw "WBD capture route survived automatic Exit cleanup: $prefix ifindex=$ifIndex"
            }
        }
    }
    Write-Output 'WBD_WINDOWS_FROZEN_ONE_SHOT_PASS dns=1 tcp=1 udp=1 exit_cleanup=automatic transport=frozen_wbd'
} finally {
    # Never use Stop-Process for normal qualification shutdown. If an assertion
    # fails after CONNECTED, request the same Controller.Disconnect path and wait
    # for it. The qualifier also has a two-minute self-stop and defer cleanup.
    if ($proc) {
        $proc.Refresh()
        if (-not $proc.HasExited) {
            New-Item -ItemType File -Force -Path $stopFile | Out-Null
            [void]$proc.WaitForExit(30000)
        }
    }
    if (Test-Path -LiteralPath $routeState) {
        # Last-resort WBD-owned route recovery for a qualifier/process failure.
        # This is still the same qualified cleanup script and never removes
        # unrelated routes or addresses.
        try { & $routeScript -Action Cleanup -StatePath $routeState | Out-Null } catch { }
    }
}
