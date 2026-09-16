from pathlib import Path

p = Path('.github/workflows/windows-portable-bundle.yml')
s = p.read_text(encoding='utf-8')

branch_anchor = '''      - agent/integrate-pressure-mtu-dtls-20b7\n'''
if '      - agent/fix-per-association-mtu-20260914\n' not in s:
    if s.count(branch_anchor) != 1:
        raise SystemExit('push branch anchor changed')
    s = s.replace(branch_anchor, branch_anchor + '      - agent/fix-per-association-mtu-20260914\n', 1)

start = s.find('      - name: Manifest hash and embed portable payload\n')
end = s.find('      - name: Verify portable EXE PE dependencies\n', start)
if start < 0 or end < 0:
    raise SystemExit('legacy embedded bundle block not found')
replacement = r'''      - name: Assemble install-directory client
        shell: pwsh
        run: |
          $ErrorActionPreference = 'Stop'
          $payload = 'build\windows-runtime'
          Copy-Item scripts\windows_tun_route.ps1 "$payload\windows_tun_route.ps1"
          Copy-Item scripts\windows_tun_rebind.ps1 "$payload\windows_tun_rebind.ps1"
          Copy-Item scripts\windows_ipv6_killswitch.ps1 "$payload\windows_ipv6_killswitch.ps1"
          Copy-Item scripts\windows_faketcp_underlay.ps1 "$payload\windows_faketcp_underlay.ps1"
          Copy-Item scripts\windows_npcap_prepare.ps1 "$payload\windows_npcap_prepare.ps1"
          Set-Content -LiteralPath "$payload\SOURCE_SHA.txt" -Encoding ascii -Value $env:WBD_SOURCE_SHA

          # The user-facing entry point is a normal launcher beside the runtime.
          # No build tag embeds payload.zip and no runtime extraction is involved.
          go build -trimpath -ldflags "-s -w -H=windowsgui" -o "$payload\wbd.exe" .\cmd\wbd-windows-portable
          if ($LASTEXITCODE -ne 0) { throw "installed launcher build failed: $LASTEXITCODE" }

          $required = @(
            'wbd.exe','wbd-reality-front.exe','wbd-faketcp.exe','wbd_dtls_shim.exe','wbd-link-proxy.exe','wbd-game-lane-client.exe','wbd-tun.exe','wbd-windows-gui.exe',
            'wintun.dll','WINTUN-LICENSE.txt','SOURCE_SHA.txt','windows_tun_route.ps1','windows_tun_rebind.ps1','windows_ipv6_killswitch.ps1','windows_faketcp_underlay.ps1','windows_npcap_prepare.ps1'
          )
          foreach ($name in $required) {
            if (-not (Test-Path (Join-Path $payload $name))) { throw "install directory missing $name" }
          }
          if (Test-Path 'internal\windowsbundle\payload.zip') {
            $length = (Get-Item 'internal\windowsbundle\payload.zip').Length
            if ($length -gt 0) { throw 'release packaging must not consume an embedded payload.zip' }
          }

          $files = [ordered]@{}
          Get-ChildItem $payload -File | Sort-Object Name | ForEach-Object {
            if ($_.Name -ne 'manifest.json') {
              $files[$_.Name] = (Get-FileHash -Algorithm SHA256 $_.FullName).Hash.ToLowerInvariant()
            }
          }
          $manifest = [ordered]@{
            schema = 'wbd-windows-install-directory/v1'
            launch = 'wbd.exe'
            extraction = $false
            files = $files
            wintun_version = '0.14.1'
            wintun_zip_sha256 = '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51'
          }
          $manifest | ConvertTo-Json -Depth 6 | Set-Content "$payload\manifest.json" -Encoding utf8
          Write-Output "WBD_WINDOWS_INSTALL_DIR_PASS source_sha=$env:WBD_SOURCE_SHA files=$($files.Count) extraction=0"
      - name: Qualify installed runtime directory
        shell: pwsh
        run: |
          $ErrorActionPreference = 'Stop'
          go test ./internal/windowsgui ./internal/windowsruntime ./cmd/wbd-windows-gui ./cmd/wbd-windows-portable -count=1
          if ($LASTEXITCODE -ne 0) { throw "installed runtime tests failed: $LASTEXITCODE" }
          $manifest = Get-Content build\windows-runtime\manifest.json -Raw | ConvertFrom-Json
          if ($manifest.schema -ne 'wbd-windows-install-directory/v1') { throw 'wrong install-directory manifest schema' }
          if ($manifest.extraction -ne $false) { throw 'install-directory manifest unexpectedly enables extraction' }
          if ($manifest.launch -ne 'wbd.exe') { throw 'install-directory launch entry is not wbd.exe' }
          Write-Output 'WBD_WINDOWS_NO_EXTRACTION_PASS temp_payload=0 installed_runtime=1'
'''
s = s[:start] + replacement + s[end:]
s = s.replace('''          $deps = & $dumpbin /dependents build\\wbd.exe | Out-String\n''', '''          $deps = & $dumpbin /dependents build\\windows-runtime\\wbd.exe | Out-String\n''', 1)
s = s.replace("Write-Output 'WBD_WINDOWS_PORTABLE_OUTER_DEPS_PASS dynamic_crt=0'", "Write-Output 'WBD_WINDOWS_INSTALL_ENTRY_DEPS_PASS dynamic_crt=0'", 1)
s = s.replace('''      - name: Upload portable bundle\n''', '''      - name: Upload install-directory bundle\n''', 1)
s = s.replace('''          path: build/wbd.exe\n''', '''          path: build/windows-runtime/**\n''', 1)

for forbidden in ['EnsureRuntime()', 'embedded portable runtime extraction', 'Build WBD portable launcher']:
    if forbidden in s:
        raise SystemExit(f'legacy extraction contract remains: {forbidden}')

p.write_text(s, encoding='utf-8')
print('Windows workflow switched to install-directory artifact without runtime extraction')
