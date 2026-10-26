param(
 [Parameter(Mandatory=$true)][string]$ReleasePath,
 [Parameter(Mandatory=$true)][string]$OutDirectory
)
$ErrorActionPreference='Stop'
$worker=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
if((Get-FileHash -LiteralPath $ReleasePath -Algorithm SHA256).Hash.ToLowerInvariant() -cne 'f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca'){throw 'F196 release mismatch'}
if((Get-FileHash -LiteralPath ($ReleasePath+'.config') -Algorithm SHA256).Hash.ToLowerInvariant() -cne '4d83c0772f4b593d6e16657fb5be7e4478321cb9782ba744c93b90915fb7175b'){throw 'F196 configuration mismatch'}
if(Test-Path -LiteralPath $OutDirectory){throw 'Refusing an existing output directory'}
New-Item -ItemType Directory -Path $OutDirectory | Out-Null
$csc=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$framework=Join-Path $worker 'desktop\windows\FrameworkTarget.cs'
& $csc /nologo /target:winexe /optimize+ /warnaserror+ ("/out:"+$OutDirectory+'\OwnerStagedInput.exe') /r:System.Windows.Forms.dll /r:System.Drawing.dll /r:System.Web.Extensions.dll $framework (Join-Path $PSScriptRoot 'OwnerStagedInput.cs')
if($LASTEXITCODE -ne 0){throw 'Native interface compilation failed'}
& $csc /nologo /target:exe /optimize+ /warnaserror+ ("/out:"+$OutDirectory+'\StrictTransport.exe') /r:System.Net.Http.dll /r:System.Web.Extensions.dll ("/r:"+$ReleasePath) $framework (Join-Path $PSScriptRoot 'StrictTransport.cs')
if($LASTEXITCODE -ne 0){throw 'Transport compilation failed'}
Copy-Item -LiteralPath $ReleasePath -Destination (Join-Path $OutDirectory 'SecureFileDrop.exe')
Copy-Item -LiteralPath ($ReleasePath+'.config') -Destination (Join-Path $OutDirectory 'StrictTransport.exe.config')
$p=Start-Process -FilePath (Join-Path $OutDirectory 'OwnerStagedInput.exe') -ArgumentList '--self-test' -WindowStyle Hidden -PassThru -Wait
if($p.ExitCode -ne 0){throw 'Masked UI synthetic self-test failed'}
$start=[Diagnostics.ProcessStartInfo]::new()
$start.FileName=Join-Path $OutDirectory 'StrictTransport.exe'
$start.UseShellExecute=$false;$start.CreateNoWindow=$true
$start.RedirectStandardInput=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
$p=[Diagnostics.Process]::Start($start)
$p.StandardInput.WriteLine('{"op":"policy"}');$p.StandardInput.Close()
$answer=$p.StandardOutput.ReadToEnd();$null=$p.StandardError.ReadToEnd()
$p.WaitForExit()
$policy=$answer|ConvertFrom-Json
if($p.ExitCode -ne 0 -or -not $policy.ok -or -not $policy.revocation){throw 'Frozen F196 transport policy self-test failed'}
$hashes=@{}
foreach($file in Get-ChildItem -LiteralPath $OutDirectory -File){$hashes[$file.Name]=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
$evidence=@{framework_release=(Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full').Release;masked_UI_self_test=$true;frozen_F196_transport_policy=$true;actual_Owner_visual_acceptance=$false;genuine_credentials_used=$false;hashes=$hashes}
$json=$evidence|ConvertTo-Json -Depth 4
[IO.File]::WriteAllText((Join-Path $OutDirectory 'build-evidence.json'),$json,[Text.UTF8Encoding]::new($false))
Write-Output $json
