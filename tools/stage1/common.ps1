Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
$script:StageRoot=$PSScriptRoot
$script:Policy=Get-Content -LiteralPath "$PSScriptRoot\policy.json" -Raw|ConvertFrom-Json
. "$PSScriptRoot\save-lab.ps1"
function Get-Hash([string]$Path){(Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()}
function Get-TextHash([string]$Text){$h=[Security.Cryptography.SHA256]::Create();try{([BitConverter]::ToString($h.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text)))).Replace('-','').ToLowerInvariant()}finally{$h.Dispose()}}
function New-File([string]$Path,[string]$Text){$f=[IO.File]::Open($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);try{$b=[Text.Encoding]::UTF8.GetBytes($Text);$f.Write($b,0,$b.Length);$f.Flush($true)}finally{$f.Dispose()}}
function Assert-NewPath([string]$Path){
 if($Path-match'(^|\\)(\.{1,2}|[^\\]*[ .])(\\|$)'-or$Path.Contains('/')){throw 'STOP:path-alias'}
 $full=[IO.Path]::GetFullPath($Path);$prefix=$script:Policy.root+'\'
 if($full-ne$Path -or -not $full.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or $Path.Substring(2).Contains(':')){throw 'STOP:path-scope'}
 $p=$full
 while($p){if(Test-Path -LiteralPath $p){if((Get-Item -Force -LiteralPath $p).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:reparse'}};$next=[IO.Path]::GetDirectoryName($p);if($next-eq$p){break};$p=$next}
}
function Get-SourceIdentity {
 $repo=(git -C $script:StageRoot rev-parse --show-toplevel);if($LASTEXITCODE-ne0){throw 'STOP:source-root'}
 $head=(git -C $repo rev-parse HEAD);if($head-notmatch'^[a-f0-9]{40}$'){throw 'STOP:source-head'}
 $status=@(git --no-optional-locks -C $repo status --porcelain=v1);if($status.Count-ne0){throw 'STOP:source-dirty'}
 $check=& (Get-Command python.exe -ErrorAction Stop).Source -I -B -c "import sys;sys.path.insert(0,sys.argv[1]);import guard;print(guard.verify_manifest(guard.HERE))" $script:StageRoot
 if($LASTEXITCODE-ne0){throw 'STOP:source-manifest'}
 $m=Get-Content -LiteralPath "$script:StageRoot\manifest.json" -Raw|ConvertFrom-Json
 foreach($row in $m.files){if($row.path-match'(^/|\\|\.\.|:)'){throw 'STOP:manifest-path'};$p=Join-Path $script:StageRoot $row.path;if((Get-Item -Force -LiteralPath $p).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:source-reparse'};if((Get-Hash $p)-ne$row.SHA256 -or (Get-Item -Force -LiteralPath $p).Length-ne$row.bytes){throw 'STOP:source-hash'}}
 [pscustomobject]@{sha=$head;manifest_sha256=Get-Hash "$script:StageRoot\manifest.json";repo=$repo}
}
function Read-Strict([string]$Body){
 $q=@{mode='json';body=$Body}|ConvertTo-Json -Depth 12 -Compress
 $result=Invoke-StagePythonJSON ('-X utf8 -I -B "'+$script:StageRoot+'\packet.py"') $q
 return ($result|ConvertFrom-Json)
}
function Get-VerifiedApprovals([long]$ReviewId,[long]$OwnerId,[string]$BundlePath,[string]$HostIdentity) {
 if($ReviewId-le0-or$OwnerId-le0-or$ReviewId-eq$OwnerId){throw 'STOP:two-external-approvals-required'}
 $a=$script:Policy.approval_authority
 if(-not$a.enrollment_accepted-or$null-eq$a.reviewer.id-or$a.reviewer.id-eq$a.owner.id-or$a.reviewer.id-eq$a.producer_id){throw 'STOP:independent-reviewer-unenrolled'}
 $source=Get-SourceIdentity;$bundleHash=Get-Hash $BundlePath
 $uri='https://api.github.com/repos/dreamingfree09/secure-file-drop'
 $headers=@{'User-Agent'='SFD-independent-approval-readonly';'Accept'='application/vnd.github+json'}
 $pr=Invoke-RestMethod -Uri "$uri/pulls/$($a.pull_request_number)" -Headers $headers -TimeoutSec 15
 if($pr.id-ne$a.pull_request_id-or$pr.base.repo.id-ne$a.repository_id-or$pr.head.repo.id-ne$a.repository_id-or$pr.head.sha-ne$source.sha-or$pr.merged-or$pr.state-ne'open'){throw 'STOP:approval-pr-drift'}
 $records=@();$raw=@()
 foreach($id in @($ReviewId,$OwnerId)){
  $c=Invoke-RestMethod -Uri "$uri/issues/comments/$id" -Headers $headers -TimeoutSec 15
  if($c.id-ne$id-or$c.issue_url-ne"$uri/issues/$($a.pull_request_number)"){throw 'STOP:approval-comment-scope'}
  $records+=@(@{id=$c.id;author_id=$c.user.id;login=$c.user.login;user_type=$c.user.type;association=$c.author_association;via_app=($null-ne$c.performed_via_github_app);repository_id=$pr.base.repo.id;pull_request_id=$pr.id;pull_request_number=$pr.number;created_utc=$c.created_at;updated_utc=$c.updated_at;body=$c.body});$raw+=@($c)
 }
 $context=@{source_sha=$source.sha;manifest_sha256=$source.manifest_sha256;bundle_sha256=$bundleHash;host_identity_sha256=$HostIdentity}
 $q=@{mode='approval_pair';review=$records[0];owner=$records[1];context=$context}|ConvertTo-Json -Depth 14 -Compress
 $pair=(Invoke-StagePythonJSON ('-X utf8 -I -B "'+$script:StageRoot+'\packet.py"') $q)|ConvertFrom-Json
 # Re-read immutable comments and ref after validation; edits/deletion/drift deny.
 for($i=0;$i-lt2;$i++){$c=Invoke-RestMethod -Uri "$uri/issues/comments/$($raw[$i].id)" -Headers $headers -TimeoutSec 15;if($c.body-cne$raw[$i].body-or$c.updated_at-ne$raw[$i].updated_at-or$c.user.id-ne$raw[$i].user.id){throw 'STOP:approval-comment-race'}}
 $again=Invoke-RestMethod -Uri "$uri/pulls/$($a.pull_request_number)" -Headers $headers -TimeoutSec 15
 if($again.head.sha-ne$source.sha-or$again.merged-or$again.state-ne'open'){throw 'STOP:approval-ref-race'}
 [pscustomobject]@{source=$source;verified_pair=$pair;packets=$records;bundle=$BundlePath;bundle_hash=$bundleHash;host_identity=$HostIdentity;expires_utc=$pair.expires_utc}
}

function Assert-Bundle([string]$Path){
 $b=Read-Strict ([IO.File]::ReadAllText($Path))
 if(@($b.files).Count-gt64){throw 'STOP:input-count'}
 if($b.schema-ne'SFD_STAGE1_INPUT_BUNDLE_V1' -or $b.work_id-ne$script:Policy.work_id -or $b.complete-isnot[bool] -or -not$b.complete){throw 'STOP:bundle-incomplete'}
 if(($b.files|Measure-Object -Property bytes -Sum).Sum-gt24GB){throw 'STOP:input-size-budget'}
 $names=@($b.files|ForEach-Object{[IO.Path]::GetFileName($_.path)});if(@($names|Select-Object -Unique).Count-ne$names.Count){throw 'STOP:input-filename-collision'}
 foreach($x in $b.files){
  $full=[IO.Path]::GetFullPath($x.path);if($full-ne$x.path-or-not$full.StartsWith($script:Policy.preparation_input_root+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'STOP:preparation-input-root'}
  if($x.path-match'(?i)\.(vhdx?|env|key|pfx|p12)$' -or $x.path-match'(?i)(\\\.codex\\|\\Secure File Drop\\|\\Docker\\wsl\\)' -or $x.sha256-notmatch'^[a-f0-9]{64}$'){throw 'STOP:input-scope'}
  if($x.path.Substring(2).Contains(':')-or$x.path-match'(^|\\)(\.{1,2}|[^\\]*[ .])(\\|$)'){throw 'STOP:input-alias'}
  for($parent=$x.path;$parent;$parent=[IO.Path]::GetDirectoryName($parent)){if((Get-Item -Force -LiteralPath $parent).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:input-reparse'}}
  if((Get-Hash $x.path)-ne$x.sha256 -or (Get-Item -Force -LiteralPath $x.path).Length-ne$x.bytes){throw 'STOP:input-identity'}
 }
 foreach($role in @('linux_iso','windows_iso','runtime_export','public_payload_iso')){if(@($b.files|Where-Object{$_.role-eq$role}).Count-ne1){throw 'STOP:input-role'}}
 if($b.signed_media_verified-isnot[bool]-or-not$b.signed_media_verified-or$b.windows_licence_verified-isnot[bool]-or-not$b.windows_licence_verified-or$b.runtime_acceptance_verified-isnot[bool]-or-not$b.runtime_acceptance_verified){throw 'STOP:media-runtime-gates'}
 $run=Invoke-RestMethod -Uri ("https://api.github.com/repos/dreamingfree09/secure-file-drop/actions/runs/"+[long]$b.runtime_CI_run) -Headers @{'User-Agent'='SFD-Stage1-readonly-runtime-proof'} -TimeoutSec 15
 if($run.head_sha-ne$script:Approval.source.sha-or$run.conclusion-ne'success'-or$run.path-ne'.github/workflows/stage1.yml'){throw 'STOP:runtime-CI-not-accepted'}
 return $b
}
