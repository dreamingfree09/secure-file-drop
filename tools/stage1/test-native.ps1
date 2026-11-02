$ErrorActionPreference='Stop'
$root=Join-Path ([IO.Path]::GetTempPath()) ('sfd-stage1-fixture-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root -ErrorAction Stop|Out-Null
$compiler=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$source=Join-Path $root 'Canary.cs';[IO.File]::WriteAllText($source,'public static class Canary { public static int Main() { return 73; } }')
& $compiler /nologo /target:exe "/out:$root\Canary.exe" $source
if($LASTEXITCODE -ne 0){throw 'canary compile'}
& $compiler /nologo /target:library /r:System.Web.Extensions.dll "/out:$root\Guard.dll" "$PSScriptRoot\ActivationGuard.cs"
if($LASTEXITCODE -ne 0){throw 'guard compile'}
Add-Type -Path "$root\Guard.dll"
function Run-Canary {
 $p=[Diagnostics.Process]::new();$p.StartInfo.FileName="$root\Canary.exe";$p.StartInfo.UseShellExecute=$false;$p.StartInfo.CreateNoWindow=$true
 try{[void]$p.Start();$p.WaitForExit();return $p.ExitCode}finally{$p.Dispose()}
}
if((Run-Canary)-ne73){throw 'baseline execution'}
$t=[Sfd.Stage1.Target]::new();$t.path="$root\Canary.exe";$t.sha256=(Get-FileHash -LiteralPath $t.path -Algorithm SHA256).Hash.ToLowerInvariant()
$lease=[Sfd.Stage1.Lease]::new(@($t),[DateTime]::UtcNow.AddMinutes(2));$denied=$false
try{
 if(-not $lease.Healthy -or $lease.Count -ne1){throw 'lease identity'}
 try{[void](Run-Canary)}catch [System.ComponentModel.Win32Exception]{if($_.Exception.NativeErrorCode -in @(5,32)){$denied=$true}else{throw}}
 if(-not $denied){throw 'exclusive lock did not prevent canary execution'}
 try{[IO.File]::ReadAllBytes($t.path)|Out-Null;throw 'read incorrectly allowed'}catch [IO.IOException]{}
 if([Sfd.Stage1.Guardian]::Allowed('X:\original.vhdx')){throw 'protected image allowed'}
 if([Sfd.Stage1.Guardian]::Allowed('C:\unrelated\Docker Desktop.exe')){throw 'unrelated path allowed'}
}finally{$lease.Dispose()}
if((Run-Canary)-ne73){throw 'release recovery'}
$t.sha256='0'*64;$failed=$false
try{$bad=[Sfd.Stage1.Lease]::new(@($t),[DateTime]::UtcNow.AddMinutes(1));$bad.Dispose()}catch{$failed=$true}
if(-not $failed -or (Run-Canary)-ne73){throw 'identity failure must release acquired handles'}
[ordered]@{schema='SFD_STAGE1_NATIVE_TEST_V1';canary_baseline=$true;kernel_lock_blocks_execution=$true;read_sharing_denied=$true;release_restores_canary=$true;identity_failure_releases=$true;protected_target_rejected=$true;original_binary_opened=$false;host_setting_changed=$false;fixture_path=$root}|ConvertTo-Json
# Keep this tiny owned fixture for review; no recursive delete and no original process operation.
