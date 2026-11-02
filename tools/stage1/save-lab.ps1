param([switch]$Execute)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
function Read-ContainmentJSON([string]$Text){
 # Constant parser, not a working-tree import. Records contain identities only.
 # Windows PowerShell may emit one UTF-8 transport BOM. Byte hashes were
 # already checked before this parse; JSON values and duplicate checks stay exact.
 $code=@("import json,sys","def unique(p):"," if len(dict(p))!=len(p): raise ValueError()"," return dict(p)","try:"," v=json.loads(sys.stdin.buffer.read().decode('utf-8-sig'),object_pairs_hook=unique,parse_constant=lambda _: (_ for _ in ()).throw(ValueError()))"," print(json.dumps(v))","except Exception:"," sys.stderr.write('STOP:containment-json\n');sys.exit(2)") -join "`n"
 $out=$Text|& (Get-Command python.exe -ErrorAction Stop).Source -X utf8 -I -B -c $code
 if($LASTEXITCODE-ne0){throw 'STOP:containment-json'};return ($out|ConvertFrom-Json)
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
if($MyInvocation.InvocationName-eq'.'){return}
if(-not$Execute){Write-Output 'PLAN: save only exact approved new lab VMs; dirty checkout and expired startup lease do not block containment.';return}
# Fixed locator, never a mutable policy import. Validate private custody before
# reading the source pointer, then verify committed policy/module against the
# approval receipt's exact manifest. No git status/HEAD/working-file dependency.
$root='H:\SFD-Isolated-Lab-20261102-8df9c4a1'
for($p=$root;$p;$p=[IO.Path]::GetDirectoryName($p)){if((Get-Item -LiteralPath $p -ErrorAction Stop).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:containment-root-reparse'}}
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
