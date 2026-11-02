$ErrorActionPreference='Stop'
foreach($p in (Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.ps1')){
 $tokens=$null;$errors=$null;[void][Management.Automation.Language.Parser]::ParseFile($p.FullName,[ref]$tokens,[ref]$errors)
 if($errors){throw ('PowerShell syntax: '+$p.Name)}
}
$plan=& "$PSScriptRoot\install-stage1.ps1" -Phase Plan|Out-String
$p=$plan|ConvertFrom-Json
if($p.authorises_mutation-isnot[bool]-or$p.authorises_mutation){throw 'Plan must be read-only'}
$failed=$false
try{& "$PSScriptRoot\install-stage1.ps1" -Phase ProtectHost -ReviewComment 0 -OwnerGoComment 0|Out-Null}catch{if($_.Exception.Message-eq'STOP:two-external-approvals-required'){$failed=$true}else{throw}}
if(-not$failed){throw 'Missing authority must fail before collection/mutation'}
$csc=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$temp=Join-Path ([IO.Path]::GetTempPath()) ('sfd-stage1-probe-'+[guid]::NewGuid().ToString('N'));New-Item -ItemType Directory -Path $temp|Out-Null
& $csc /nologo /r:System.Net.Http.dll "/out:$temp\TlsProbe.exe" "$PSScriptRoot\TlsProbe.cs"
if($LASTEXITCODE-ne0){throw 'Framework48 probe compilation'}
# Do not run TLS without separately approved guest root scope.
[ordered]@{schema='SFD_STAGE1_POWERSHELL_TEST_V1';all_scripts_parse=$true;Plan_no_mutation=$true;no_approval_stops_before_collection=$true;Framework48_probe_compiles=$true;TLS_probe_executed=$false;host_setting_changed=$false}|ConvertTo-Json
