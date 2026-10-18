param([Parameter(Mandatory=$true)][string]$Origin)
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$output = Join-Path $root "dist"
$null = New-Item -ItemType Directory -Path $output -Force
$compiler = Join-Path $env:WINDIR "Microsoft.NET\Framework64\v4.0.30319\csc.exe"
$target = Join-Path $output "Sfd.Live.Probe.exe"
& $compiler /nologo /target:exe /warn:4 /warnaserror+ "/out:$target" /r:System.dll /r:System.Core.dll /r:System.Net.Http.dll /r:System.Runtime.Serialization.dll (Join-Path $root "FrameworkTarget.cs") (Join-Path $root "Client.cs") (Join-Path $PSScriptRoot "LiveProbe.cs")
if ($LASTEXITCODE -ne 0) { throw "Live probe compilation failed." }
Copy-Item -LiteralPath (Join-Path $root "SecureFileDrop.exe.config") -Destination ($target + ".config")
& $target $Origin
if ($LASTEXITCODE -ne 0) { throw "Credential-free live HTTPS checks failed." }