param([switch]$Execute)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
function Invoke-StagePythonJSON([string]$Arguments,[string]$Text){
 # Explicit UTF8 payload bytes, independent of PowerShell's native text-pipe
 # OutputEncoding. Framework48 can prepend its single UTF8 stdin preamble.
 $bytes=[Text.UTF8Encoding]::new($false).GetBytes($Text)
 if($bytes.Length-gt131072){throw 'STOP:json-input-bound'}
 $p=[Diagnostics.Process]::new();$p.StartInfo.FileName=(Get-Command python.exe -ErrorAction Stop).Source;$p.StartInfo.Arguments=$Arguments;$p.StartInfo.UseShellExecute=$false;$p.StartInfo.CreateNoWindow=$true;$p.StartInfo.RedirectStandardInput=$true;$p.StartInfo.RedirectStandardOutput=$true;$p.StartInfo.RedirectStandardError=$true
 $started=$false
 try{
  if(-not$p.Start()){throw 'STOP:json-process'};$started=$true
  $writer=$p.StandardInput;$preamble=$writer.Encoding.GetPreamble()
  if($preamble.Length-ne0-and[BitConverter]::ToString($preamble)-ne'EF-BB-BF'){throw 'STOP:json-unsupported-process-encoding'}
  $stdout=$p.StandardOutput.ReadToEndAsync();$stderr=$p.StandardError.ReadToEndAsync()
  # Flush an optional Framework preamble BEFORE raw UTF8 bytes, never after.
  $writer.Flush();$writer.BaseStream.Write($bytes,0,$bytes.Length);$writer.Close()
  if(-not[Threading.Tasks.Task]::WaitAll([Threading.Tasks.Task[]]@($stdout,$stderr),15000)-or-not$p.WaitForExit(15000)){throw 'STOP:json-process-timeout'}
  if($p.ExitCode-ne0){throw 'STOP:strict-json-process'}
  return $stdout.Result
 }finally{
  # Only the exact child created here can be terminated on a stalled parse.
  if($started-and-not$p.HasExited){$p.Kill();[void]$p.WaitForExit(3000)}
  $p.Dispose()
 }
}
function Read-ContainmentJSON([string]$Text){
 # Constant parser, not a working-tree import. Records contain identities only.
 # Windows PowerShell may emit one UTF-8 transport BOM. Byte hashes were
 # already checked before this parse; JSON values and duplicate checks stay exact.
 $code=@("import json,sys","def unique(p):"," if len(dict(p))!=len(p): raise ValueError()"," return dict(p)","try:"," v=json.loads(sys.stdin.buffer.read().decode('utf-8-sig'),object_pairs_hook=unique,parse_constant=lambda _: (_ for _ in ()).throw(ValueError()))"," print(json.dumps(v))","except Exception:"," sys.stderr.write('STOP:containment-json\n');sys.exit(2)") -join "`n"
 $out=Invoke-StagePythonJSON ('-X utf8 -I -B -c "'+$code+'"') $Text
 return ($out|ConvertFrom-Json)
}
function Get-ContainmentByteHash([byte[]]$Bytes){$h=[Security.Cryptography.SHA256]::Create();try{return ([BitConverter]::ToString($h.ComputeHash($Bytes))).Replace('-','').ToLowerInvariant()}finally{$h.Dispose()}}
function Read-ContainmentGitBlob([string]$Repository,[string]$SHA,[string]$Path){
 if($SHA-notmatch'^[a-f0-9]{40}$'-or$Path-notin@('tools/stage1/manifest.json','tools/stage1/policy.json','tools/stage1/containment.ps1')){throw 'STOP:containment-blob-scope'}
 $p=[Diagnostics.Process]::new();$p.StartInfo.FileName=(Get-Command git.exe -ErrorAction Stop).Source;$p.StartInfo.WorkingDirectory=$Repository;$p.StartInfo.Arguments="--no-pager show ${SHA}:$Path";$p.StartInfo.UseShellExecute=$false;$p.StartInfo.CreateNoWindow=$true;$p.StartInfo.RedirectStandardOutput=$true;$p.StartInfo.RedirectStandardError=$true
 $m=[IO.MemoryStream]::new()
 try{if(-not$p.Start()){throw 'STOP:containment-git'};$copy=$p.StandardOutput.BaseStream.CopyToAsync($m);$errors=$p.StandardError.ReadToEndAsync();[Threading.Tasks.Task]::WaitAll([Threading.Tasks.Task[]]@($copy,$errors));$p.WaitForExit();if($p.ExitCode-ne0){throw 'STOP:containment-commit-unavailable'};return ,$m.ToArray()}finally{$m.Dispose();$p.Dispose()}
}
function Read-ReviewedContainmentSource([string]$Repository,[string]$SourceSHA,[string]$ManifestSHA256){
 if($ManifestSHA256-notmatch'^[a-f0-9]{64}$'){throw 'STOP:containment-manifest-identity'}
 [byte[]]$raw=Read-ContainmentGitBlob $Repository $SourceSHA 'tools/stage1/manifest.json'
 if((Get-ContainmentByteHash $raw)-ne$ManifestSHA256){throw 'STOP:containment-manifest-hash'}
 $m=Read-ContainmentJSON ([Text.Encoding]::UTF8.GetString($raw));if($m.schema-ne'SFD_STAGE1_SOURCE_MANIFEST_V1'){throw 'STOP:containment-manifest-schema'}
 $data=@{}
 foreach($path in @('policy.json','containment.ps1')){
  $row=@($m.files|Where-Object{$_.path-ceq$path});if($row.Count-ne1){throw 'STOP:containment-manifest-row'}
  [byte[]]$bytes=Read-ContainmentGitBlob $Repository $SourceSHA ('tools/stage1/'+$path)
  if($bytes.Length-ne$row[0].bytes-or(Get-ContainmentByteHash $bytes)-ne$row[0].SHA256){throw 'STOP:containment-source-hash'}
  $data[$path]=[Text.Encoding]::UTF8.GetString($bytes)
 }
 return [pscustomobject]@{policy=Read-ContainmentJSON $data['policy.json'];code=$data['containment.ps1'];source_sha=$SourceSHA}
}
function Invoke-RecordedContainment {
$root='H:\SFD-Isolated-Lab-20261102-8df9c4a1'
for($p=$root;$p;$p=[IO.Path]::GetDirectoryName($p)){if((Get-Item -Force -LiteralPath $p -ErrorAction Stop).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:containment-root-reparse'}}
$acl=Get-Acl -LiteralPath $root;$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
if(-not$acl.AreAccessRulesProtected-or$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value-ne$sid){throw 'STOP:containment-custody'}
foreach($rule in $acl.Access){if($rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value-notin@($sid,'S-1-5-18')){throw 'STOP:containment-custody'}}
$receipt=Read-ContainmentJSON ([IO.File]::ReadAllText((Join-Path $root 'approval-receipts.json')))
$own=Read-ContainmentJSON ([IO.File]::ReadAllText((Join-Path $root 'guests.json')))
if($own.source_sha-ne$receipt.source.sha){throw 'STOP:containment-receipt-source'}
$repo=git -C $PSScriptRoot rev-parse --show-toplevel;if($LASTEXITCODE-ne0){throw 'STOP:containment-repository'}
$capsule=Read-ReviewedContainmentSource $repo $receipt.source.sha $receipt.source.manifest_sha256
if($capsule.policy.root-cne$root-or$capsule.policy.work_id-ne'SFD-STAGE1-INSTALLATION-20261102'){throw 'STOP:containment-policy'}
. ([scriptblock]::Create($capsule.code))
Save-OwnedLab -Policy $capsule.policy -Ownership $own -ExpectedSourceSHA $capsule.source_sha

}
if($MyInvocation.InvocationName-eq'.'){return}
if(-not$Execute){Write-Output 'PLAN: manifest-bound save-only recovery; no startup approval or clean checkout required.';return}
Invoke-RecordedContainment
