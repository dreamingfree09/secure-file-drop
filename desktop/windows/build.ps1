param([string]$OutputDirectory = (Join-Path $PSScriptRoot "dist"))
$ErrorActionPreference = "Stop"
$compiler = Join-Path $env:WINDIR "Microsoft.NET\Framework64\v4.0.30319\csc.exe"
if (-not (Test-Path -LiteralPath $compiler)) { throw "Install the Windows .NET Framework 4.8 developer/runtime prerequisites; this script never installs them." }
$null = New-Item -ItemType Directory -Path $OutputDirectory -Force
$target = Join-Path $OutputDirectory "SecureFileDrop.exe"
$inputs = @("FrameworkTarget.cs", "Client.cs", "WindowsIntegration.cs", "App.cs") | ForEach-Object { Join-Path $PSScriptRoot $_ }
& $compiler /nologo /target:winexe /optimize+ /warn:4 /warnaserror+ /platform:anycpu /utf8output "/out:$target" "/win32manifest:$(Join-Path $PSScriptRoot 'app.manifest')" /r:System.dll /r:System.Core.dll /r:System.Net.Http.dll /r:System.Runtime.Serialization.dll /r:System.Security.dll /r:System.Windows.Forms.dll /r:System.Drawing.dll @inputs
if ($LASTEXITCODE -ne 0) { throw "Windows companion build failed." }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "SecureFileDrop.exe.config") -Destination ($target + ".config")
Get-FileHash -LiteralPath $target -Algorithm SHA256 | Select-Object Path, Hash
