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
 while($p){if(Test-Path -LiteralPath $p){if((Get-Item -LiteralPath $p).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:reparse'}};$next=[IO.Path]::GetDirectoryName($p);if($next-eq$p){break};$p=$next}
}
function Get-SourceIdentity {
 $repo=(git -C $script:StageRoot rev-parse --show-toplevel);if($LASTEXITCODE-ne0){throw 'STOP:source-root'}
 $head=(git -C $repo rev-parse HEAD);if($head-notmatch'^[a-f0-9]{40}$'){throw 'STOP:source-head'}
 $status=@(git --no-optional-locks -C $repo status --porcelain=v1);if($status.Count-ne0){throw 'STOP:source-dirty'}
 $check=& (Get-Command python.exe -ErrorAction Stop).Source -I -B -c "import sys;sys.path.insert(0,sys.argv[1]);import guard;print(guard.verify_manifest(guard.HERE))" $script:StageRoot
 if($LASTEXITCODE-ne0){throw 'STOP:source-manifest'}
 $m=Get-Content -LiteralPath "$script:StageRoot\manifest.json" -Raw|ConvertFrom-Json
 foreach($row in $m.files){if($row.path-match'(^/|\\|\.\.|:)'){throw 'STOP:manifest-path'};$p=Join-Path $script:StageRoot $row.path;if((Get-Item -LiteralPath $p).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:source-reparse'};if((Get-Hash $p)-ne$row.SHA256 -or (Get-Item -LiteralPath $p).Length-ne$row.bytes){throw 'STOP:source-hash'}}
 [pscustomobject]@{sha=$head;manifest_sha256=Get-Hash "$script:StageRoot\manifest.json";repo=$repo}
}
function Read-Strict([string]$Body){
 $q=@{mode='json';body=$Body}|ConvertTo-Json -Depth 12 -Compress
 $result=Invoke-StagePythonJSON ('-X utf8 -I -B "'+$script:StageRoot+'\packet.py"') $q
 return ($result|ConvertFrom-Json)
}
function Get-VerifiedApprovals([long]$ReviewId,[long]$OwnerId,[string]$BundlePath,[string]$HostIdentity) {
 if($ReviewId-le0 -or $OwnerId-le0 -or $ReviewId-eq$OwnerId){throw 'STOP:two-external-approvals-required'}
 $source=Get-SourceIdentity;$bundleHash=Get-Hash $BundlePath;$packets=@();$issue=$null
 foreach($spec in @(@{id=$ReviewId;role='Aether'},@{id=$OwnerId;role='Owner'})){
  $c=Invoke-RestMethod -Uri "https://api.github.com/repos/dreamingfree09/secure-file-drop/issues/comments/$($spec.id)" -Headers @{'User-Agent'='SFD-Stage1-readonly-authority';'Accept'='application/vnd.github+json'} -TimeoutSec 15
  if($c.id-ne$spec.id -or $c.user.id-ne$script:Policy.author_id -or $c.issue_url-notmatch'^https://api.github.com/repos/dreamingfree09/secure-file-drop/issues/([0-9]+)$'){throw 'STOP:approval-identity'}
  $n=[int]$Matches[1];if($null-ne$issue-and$issue-ne$n){throw 'STOP:approval-thread'};$issue=$n
  $request=@{mode='approval';comment=@{author_id=$c.user.id;issue=$issue;body=$c.body};role=$spec.role;sha=$source.sha;issue=$issue}|ConvertTo-Json -Depth 12 -Compress
  $reply=Invoke-StagePythonJSON ('-X utf8 -I -B "'+$script:StageRoot+'\packet.py"') $request
  $p=$reply|ConvertFrom-Json
  if($p.schema-ne'SFD_STAGE1_ACCEPTANCE_V1' -or $p.role-ne$spec.role -or $p.decision-ne'GO' -or $p.work_id-ne$script:Policy.work_id -or $p.source_sha-ne$source.sha -or $p.manifest_sha256-ne$source.manifest_sha256 -or $p.issue-ne$issue -or $p.bundle_sha256-ne$bundleHash -or $p.host_identity_sha256-ne$HostIdentity -or $p.media_terms-isnot[bool] -or -not$p.media_terms){throw 'STOP:approval-scope'}
  if(($p.effects|ConvertTo-Json -Compress)-cne($script:Policy.effects|ConvertTo-Json -Compress)){throw 'STOP:approval-effects'}
  $now=[DateTimeOffset]::UtcNow;$start=[DateTimeOffset]::Parse($p.issued_utc);$end=[DateTimeOffset]::Parse($p.expires_utc)
  if($start.Offset-ne[TimeSpan]::Zero -or $end.Offset-ne[TimeSpan]::Zero -or $start-gt$now -or $end-le$now -or ($end-$start).TotalHours-gt8 -or $end-le$start){throw 'STOP:approval-window'}
  $packets+=[pscustomobject]@{id=$c.id;body_sha256=Get-TextHash $c.body;body=$p}
 }
 $pr=Invoke-RestMethod -Uri "https://api.github.com/repos/dreamingfree09/secure-file-drop/pulls/$issue" -Headers @{'User-Agent'='SFD-Stage1-readonly-authority'} -TimeoutSec 15
 if($pr.head.sha-ne$source.sha -or $pr.merged -or $pr.state-ne'open'){throw 'STOP:approval-ref-drift'}
 [pscustomobject]@{source=$source;packets=$packets;bundle=$BundlePath;bundle_hash=$bundleHash;host_identity=$HostIdentity;expires_utc=($packets|ForEach-Object{$_.body.expires_utc}|Sort-Object|Select-Object -First 1)}
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
  for($parent=$x.path;$parent;$parent=[IO.Path]::GetDirectoryName($parent)){if((Get-Item -LiteralPath $parent).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:input-reparse'}}
  if((Get-Hash $x.path)-ne$x.sha256 -or (Get-Item -LiteralPath $x.path).Length-ne$x.bytes){throw 'STOP:input-identity'}
 }
 foreach($role in @('linux_iso','windows_iso','runtime_export','public_payload_iso')){if(@($b.files|Where-Object{$_.role-eq$role}).Count-ne1){throw 'STOP:input-role'}}
 if($b.signed_media_verified-isnot[bool]-or-not$b.signed_media_verified-or$b.windows_licence_verified-isnot[bool]-or-not$b.windows_licence_verified-or$b.runtime_acceptance_verified-isnot[bool]-or-not$b.runtime_acceptance_verified){throw 'STOP:media-runtime-gates'}
 $run=Invoke-RestMethod -Uri ("https://api.github.com/repos/dreamingfree09/secure-file-drop/actions/runs/"+[long]$b.runtime_CI_run) -Headers @{'User-Agent'='SFD-Stage1-readonly-runtime-proof'} -TimeoutSec 15
 if($run.head_sha-ne$script:Approval.source.sha-or$run.conclusion-ne'success'-or$run.path-ne'.github/workflows/stage1.yml'){throw 'STOP:runtime-CI-not-accepted'}
 return $b
}
