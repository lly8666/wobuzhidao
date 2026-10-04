param([Parameter(Mandatory=$true)][string]$OutputDirectory,[Parameter(Mandatory=$true)][string]$SourceSHA,[Parameter(Mandatory=$true)][string]$Version)
$ErrorActionPreference = 'Stop'
if ($SourceSHA -notmatch '^[0-9a-f]{40}$' -or $Version -notmatch '^[A-Za-z0-9.-]+$') { throw 'invalid build identity' }
$productRoot = Split-Path -Parent $PSScriptRoot
$out = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $out | Out-Null
$csc = 'C:\Program Files\Microsoft Visual Studio\2022\Enterprise\MSBuild\Current\Bin\Roslyn\csc.exe'
if (-not (Test-Path -LiteralPath $csc)) { throw 'hosted Windows Roslyn compiler missing' }
$framework = Join-Path ${env:ProgramFiles(x86)} 'Reference Assemblies\Microsoft\Framework\.NETFramework\v4.8'
$generated = Join-Path $out 'SourceInfo.cs'
[IO.File]::WriteAllText($generated, "namespace Wbd.Gui { static class SourceInfo { public const string SHA = `"$SourceSHA`"; public const string Version = `"$Version`"; } }", [Text.UTF8Encoding]::new($false))
$references = @('mscorlib','System','System.Core','System.Drawing','System.Windows.Forms','System.Web.Extensions','System.ServiceProcess') | ForEach-Object { '/reference:' + (Join-Path $framework ($_.ToString()+'.dll')) }
$sources = @('Portable.cs','MainForm.cs','GuiTests.cs') | ForEach-Object { Join-Path $productRoot ('windows\gui\'+$_) }
& $csc /nologo /noconfig /nostdlib+ /langversion:latest /target:winexe /platform:x64 /optimize+ /deterministic+ /utf8output /codepage:65001 ('/out:'+(Join-Path $out 'WBD.exe')) ('/win32manifest:'+(Join-Path $productRoot 'windows\gui\app.manifest')) $references $sources $generated
if ($LASTEXITCODE -ne 0) { throw "GUI compiler failed: $LASTEXITCODE" }
Remove-Item -LiteralPath $generated
Copy-Item -LiteralPath (Join-Path $productRoot 'windows/gui/WBD.exe.config') -Destination $out
Copy-Item -LiteralPath (Join-Path $productRoot 'windows/gui/fields.json') -Destination (Join-Path $out 'gui-fields.json')
Copy-Item -LiteralPath (Join-Path $productRoot 'docs/PARAMETERS.json') -Destination $out
