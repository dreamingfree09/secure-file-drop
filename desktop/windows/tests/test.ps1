$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$output = Join-Path $root "dist"
$null = New-Item -ItemType Directory -Path $output -Force
$compiler = Join-Path $env:WINDIR "Microsoft.NET\Framework64\v4.0.30319\csc.exe"
$sources = @("FrameworkTarget.cs", "Client.cs", "WindowsIntegration.cs", "App.cs", "tests\WindowsTests.cs", "tests\ProtocolTests.cs") | ForEach-Object { Join-Path $root $_ }
$target = Join-Path $output "Sfd.Windows.Tests.exe"
& $compiler /nologo /target:exe /define:SFD_SYNTHETIC_TESTS /main:SecureFileDrop.Tests.WindowsTests /warn:4 /warnaserror+ /utf8output "/out:$target" /r:System.dll /r:System.Core.dll /r:System.Net.Http.dll /r:System.Runtime.Serialization.dll /r:System.Security.dll /r:System.Windows.Forms.dll /r:System.Drawing.dll @sources
if ($LASTEXITCODE -ne 0) { throw "Windows test compilation failed." }
Copy-Item -LiteralPath (Join-Path $root "SecureFileDrop.exe.config") -Destination ($target + ".config")
& $target
if ($LASTEXITCODE -ne 0) { throw "Windows checks failed." }

# Compile the same client without the synthetic-test symbol, exactly as the shipping build does.
$policy = Join-Path $output "Sfd.Product.Policy.exe"
& $compiler /nologo /target:exe /warn:4 /warnaserror+ "/out:$policy" /r:System.dll /r:System.Core.dll /r:System.Net.Http.dll /r:System.Runtime.Serialization.dll (Join-Path $root "FrameworkTarget.cs") (Join-Path $root "Client.cs") (Join-Path $PSScriptRoot "ProductPolicyTests.cs")
if ($LASTEXITCODE -ne 0) { throw "Release-policy compilation failed." }
Copy-Item -LiteralPath (Join-Path $root "SecureFileDrop.exe.config") -Destination ($policy + ".config")
& $policy
if ($LASTEXITCODE -ne 0) { throw "Release TLS policy checks failed." }
