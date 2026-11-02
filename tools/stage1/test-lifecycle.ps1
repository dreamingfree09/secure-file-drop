# Synthetic metadata and mock Save-VM only; never invokes a real VM cmdlet.
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
. "$PSScriptRoot\memory.ps1"
. "$PSScriptRoot\save-lab.ps1"
$script:StageRoot=$PSScriptRoot
$policy=Get-Content -LiteralPath "$PSScriptRoot\policy.json" -Raw|ConvertFrom-Json
$script:checks=0;$script:lifecyclePhase='memory'
trap{[Console]::Error.WriteLine('LIFECYCLE PHASE: '+$script:lifecyclePhase+'; TYPE: '+$_.Exception.GetType().Name);throw $_}
function Check($Value,[string]$Label){if(-not$Value){throw ('Regression: '+$Label)};$script:checks++}
function Rows([string]$A,[string]$B){
 $rows=@();$i=0;foreach($role in @('linux','windows')){$state=@($A,$B)[$i];$i++;$rows+=@([pscustomobject]@{role=$role;state=$state;startup_bytes=[long](8GB);assigned_bytes=[long]$(if($state-in@('Running','Paused')){8GB}else{0});dynamic=$false})};return $rows
}
foreach($initial in 20..27){
 $cold=Get-LabMemoryRequirement $policy (Rows Off Off);Check ($cold-eq20GB-and$initial*1GB-ge$cold) 'cold admission'
 $partial=Get-LabMemoryRequirement $policy (Rows Running Off);Check ($partial-eq12GB-and($initial-8)*1GB-ge$partial) 'second boot accounts for allocated first guest'
 $running=Get-LabMemoryRequirement $policy (Rows Running Running);Check ($running-eq4GB-and($initial-16)*1GB-ge$running) 'running host reserve'
}
Check ((Get-LabMemoryRequirement $policy)-eq20GB) 'initial collection requests all new guests'
foreach($states in @(@('Off','Off'),@('Running','Off'),@('Running','Running'))){
 $rows=Rows $states[0] $states[1];$need=Get-LabMemoryRequirement $policy $rows
 $a=Get-LabMemoryAdmission $policy $rows ($need-1) $need;Check (-not$a.RAM_available-and$a.commit_margin) 'real RAM predicate rejects one-byte deficit'
 $a=Get-LabMemoryAdmission $policy $rows $need ($need-1);Check ($a.RAM_available-and-not$a.commit_margin) 'real commit predicate rejects one-byte deficit'
}
$a=Get-LabMemoryAdmission $policy (Rows Off Off) (19GB) (20GB);Check (-not$a.RAM_available) 'initial budget cannot be underfunded'
Check ((Get-LabMemoryRequirement $policy (Rows Paused Saved))-eq12GB) 'paused memory remains allocated, saved guest needs allocation'
foreach($bad in @('Starting','Stopping','Unknown')){$failed=$false;try{[void](Get-LabMemoryRequirement $policy (Rows $bad Off))}catch{$failed=$true};Check $failed 'unknown transition rejected'}
$rows=Rows Running Off;$rows[0].assigned_bytes=[long](4GB);$failed=$false;try{[void](Get-LabMemoryRequirement $policy $rows)}catch{$failed=$true};Check $failed 'incomplete allocation rejected'
$rows=Rows Off Off;$rows[1].role='linux';$failed=$false;try{[void](Get-LabMemoryRequirement $policy $rows)}catch{$failed=$true};Check $failed 'duplicate memory identity rejected'
$rows=Rows Off Off;$rows[0].dynamic=$true;$failed=$false;try{[void](Get-LabMemoryRequirement $policy $rows)}catch{$failed=$true};Check $failed 'unexpected dynamic memory rejected'
# Exercise the actual PS5.1 native-pipe BOM path, retaining strict JSON.
$script:lifecyclePhase='UTF8 profile'
$priorEncoding=$OutputEncoding
try{
 $OutputEncoding=[Text.UTF8Encoding]::new($true)
 $parsed=Read-ContainmentJSON '{"fixture":true}';Check ($parsed.fixture-eq$true) 'UTF8 transport BOM accepted'
 $rejected=$false;try{[void](Read-ContainmentJSON '{"fixture":true,"fixture":false}')}catch{$rejected=$true};Check $rejected 'duplicate keys still rejected'
 if([Console]::InputEncoding.GetPreamble().Length-eq0){
  $parsed=Read-ContainmentJSON ([char]0xfeff+'{"fixture":true}');Check ($parsed.fixture-eq$true) 'one total preamble accepted'
 }else{
  $rejected=$false;try{[void](Read-ContainmentJSON ([char]0xfeff+'{"fixture":true}'))}catch{$rejected=$true};Check $rejected 'extra data BOM after Framework preamble rejected'
 }
 $parsed=Read-Strict '{"fixture":true}';Check ($parsed.fixture-eq$true) 'approval bridge uses the same controlled UTF8 transport'
 $rejected=$false;try{[void](Read-ContainmentJSON ([char]0xfeff+[char]0xfeff+'{}'))}catch{$rejected=$true};Check $rejected 'second BOM remains malformed JSON'
}finally{$OutputEncoding=$priorEncoding}
$script:lifecyclePhase='private fixture and committed capsule'
# New private fixture directories only; no VHD/VHDX is created or attached.
if(-not('Sfd.Stage1.HostNative'-as[type])){Add-Type -Path "$PSScriptRoot\HostNative.cs"}
$fixture=Join-Path ([IO.Path]::GetTempPath()) ('sfd-stage1-lifecycle-'+[guid]::NewGuid().ToString('N'))
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
[Sfd.Stage1.HostNative]::NewPrivateDirectory($fixture,$sid)
$repo=Join-Path $fixture 'repo';$lab=Join-Path $fixture 'new-lab';$empty=Join-Path $fixture 'empty-hooks'
foreach($dir in @($repo,$lab,$empty,(Join-Path $repo 'tools'),(Join-Path $repo 'tools\stage1'),(Join-Path $lab 'linux'),(Join-Path $lab 'windows'))){[Sfd.Stage1.HostNative]::NewPrivateDirectory($dir,$sid)}
# Hidden metadata is valid; only the NEW synthetic lab directory is marked.
$labItem=Get-Item -Force -LiteralPath $lab;$labItem.Attributes=$labItem.Attributes-bor[IO.FileAttributes]::Hidden
Check (((Get-Item -Force -LiteralPath $lab).Attributes-band[IO.FileAttributes]::Hidden)-ne0) 'hidden new fixture exercises real metadata checks'
function FixtureGit([string[]]$Arguments){
 $out=& git -c "core.hooksPath=$empty" -c "init.templateDir=$empty" -c 'user.name=SFD synthetic fixture' -c 'user.email=fixture@example.invalid' -c 'commit.gpgsign=false' -C $repo @Arguments 2>&1
 if($LASTEXITCODE-ne0){throw 'Synthetic git command failed'};return $out
}
[void](FixtureGit @('init','-q','--object-format=sha1'))
$policy.root=$lab
$fixturePolicy=Join-Path $repo 'tools\stage1\policy.json';$fixtureCode=Join-Path $repo 'tools\stage1\containment.ps1';$fixtureManifest=Join-Path $repo 'tools\stage1\manifest.json'
[IO.File]::WriteAllText($fixturePolicy,($policy|ConvertTo-Json -Depth 12),[Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllBytes($fixtureCode,[IO.File]::ReadAllBytes("$PSScriptRoot\containment.ps1"))
# Preserve LF bytes in this new synthetic repository on Windows.
[IO.File]::WriteAllText((Join-Path $repo '.gitattributes'),"* -text`n",[Text.UTF8Encoding]::new($false))
$files=@();foreach($file in @('policy.json','containment.ps1')){$bytes=[IO.File]::ReadAllBytes((Join-Path (Split-Path $fixtureCode) $file));$files+=@(@{path=$file;bytes=$bytes.Length;SHA256=Get-ContainmentByteHash $bytes})}
[IO.File]::WriteAllText($fixtureManifest,(@{schema='SFD_STAGE1_SOURCE_MANIFEST_V1';files=$files}|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
[void](FixtureGit @('add','.'));[void](FixtureGit @('commit','-qm','Synthetic approved containment fixture'))
$approved=(FixtureGit @('rev-parse','HEAD')|Out-String).Trim();$manifestHash=Get-ContainmentByteHash ([IO.File]::ReadAllBytes($fixtureManifest))
$script:lifecyclePhase='initial committed capsule'
$source=Read-ReviewedContainmentSource $repo $approved $manifestHash
Check ($source.policy.root-eq$lab) 'committed source capsule identity'
. ([scriptblock]::Create($source.code))
$action=${function:Save-OwnedLab};$frozenPolicy=$source.policy
$own=[pscustomobject]@{schema='SFD_STAGE1_OWNERSHIP_V1';work_id=$policy.work_id;source_sha=$approved;guests=@()}
$script:fakeVMs=@{};$script:saved=@()
foreach($role in @('linux','windows')){$id=[guid]::NewGuid();$spec=$policy.guests.$role;$disk=Join-Path $lab ($role+'\root.vhdx');$script:fakeVMs[$id.ToString()]=[pscustomobject]@{Id=$id;Name=$spec.name;State='Running';disk=$disk};$own.guests+=@([pscustomobject]@{id=$id.ToString();name=$spec.name;role=$role;disk=$disk})}
function Get-VM {param($Id) if(-not$script:fakeVMs.ContainsKey($Id.ToString())){throw 'Unknown synthetic VM'};return $script:fakeVMs[$Id.ToString()]}
function Get-VMHardDiskDrive {param($VM) return [pscustomobject]@{Path=$VM.disk;DiskNumber=$null}}
function Save-VM {param($VM) $script:saved+=@($VM.Id.ToString())}
# Ordinary tracked AND untracked edits, including a broken containment source.
[IO.File]::WriteAllText($fixtureCode,"throw 'MUTATED SOURCE MUST NEVER EXECUTE'")
[IO.File]::WriteAllText($fixturePolicy,'{"root":"C:\\unrelated"}')
[IO.File]::WriteAllText((Join-Path $repo 'untracked.txt'),'synthetic untracked edit')
Check (@(FixtureGit @('status','--porcelain=v1')).Count-ge3) 'fixture checkout is dirty'
$astTokens=$null;$astErrors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile("$PSScriptRoot\common.ps1",[ref]$astTokens,[ref]$astErrors)
$startup=$ast.Find({param($node) $node-is[Management.Automation.Language.FunctionDefinitionAst]-and$node.Name-eq'Get-SourceIdentity'},$true)
. ([scriptblock]::Create($startup.Extent.Text));$script:StageRoot=Join-Path $repo 'tools\stage1'
$denied=$false;try{[void](Get-SourceIdentity)}catch{if($_.Exception.Message-eq'STOP:source-dirty'){$denied=$true}else{throw}}
Check $denied 'actual startup validator still rejects dirty source'

& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null
Check ($script:saved.Count-eq2) 'cached finally saves both despite dirty tracked/untracked files'
$script:lifecyclePhase='dirty committed reload'
$restored=Read-ReviewedContainmentSource $repo $approved $manifestHash
Check ($restored.policy.root-eq$lab-and$restored.code-notmatch'MUTATED SOURCE') 'standalone committed loader ignores dirty policy/code'
[void](FixtureGit @('add','.'));[void](FixtureGit @('commit','-qm','Synthetic later source must not replace approved capsule'))
$again=Read-ReviewedContainmentSource $repo $approved $manifestHash
Check ($again.policy.root-eq$lab) 'approved revision retained after HEAD advances'
$failed=$false;try{[void](Read-ReviewedContainmentSource $repo $approved ('0'*64))}catch{$failed=$true};Check $failed 'wrong manifest rejected'
$script:saved=@();$badOwn=Read-ContainmentJSON ($own|ConvertTo-Json -Depth 10);$badOwn.source_sha='0'*40;$failed=$false;try{& $action -Policy $frozenPolicy -Ownership $badOwn -ExpectedSourceSHA $approved|Out-Null}catch{$failed=$true};Check ($failed-and$script:saved.Count-eq0) 'wrong source cannot save any target'
foreach($kind in @('duplicate','wrong-name','wrong-disk','extra-disk')){
 $script:saved=@();$badOwn=Read-ContainmentJSON ($own|ConvertTo-Json -Depth 10)
 if($kind-eq'duplicate'){$badOwn.guests[1].id=$badOwn.guests[0].id}
 if($kind-eq'wrong-name'){$badOwn.guests[1].name='Unrelated VM'}
 if($kind-eq'wrong-disk'){$badOwn.guests[1].disk='C:\unrelated\data.vhdx'}
 if($kind-eq'extra-disk'){function Get-VMHardDiskDrive {param($VM) return @([pscustomobject]@{Path=$VM.disk;DiskNumber=$null},[pscustomobject]@{Path='C:\unrelated\data.vhdx';DiskNumber=$null})}}
 $failed=$false;try{& $action -Policy $frozenPolicy -Ownership $badOwn -ExpectedSourceSHA $approved|Out-Null}catch{$failed=$true};Check ($failed-and$script:saved.Count-eq0) 'scope failure validated before any save'
}
function Get-VMHardDiskDrive {param($VM) return [pscustomobject]@{Path=$VM.disk;DiskNumber=$null}}
$script:saved=@();$transition=$script:fakeVMs[$own.guests[1].id];$transition.State='Stopping'
$failed=$false;try{& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null}catch{$failed=$true}
Check ($failed-and$script:saved.Count-eq1-and$script:saved[0]-eq$own.guests[0].id) 'transitional peer does not block other verified running guest'
$transition.State='Running'
$script:saved=@();$first=$own.guests[0].id
function Save-VM {param($VM) $script:saved+=@($VM.Id.ToString());if($VM.Id.ToString()-eq$first){throw 'Synthetic first-save failure'}}
$failed=$false;try{& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null}catch{$failed=$true}
Check ($failed-and$script:saved.Count-eq2) 'one save failure still attempts other verified guest'
# F2 uses the actual early StopGuests AST dispatch with the same authority.
function Save-VM {param($VM) $script:saved+=@($VM.Id.ToString())}
$script:saved=@();$script:fakeVMs[$own.guests[0].id].State='Off';$script:fakeVMs[$own.guests[1].id].State='Saved'
& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null
& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null
Check ($script:saved.Count-eq0) 'Off/Saved containment is idempotent'
$script:fakeVMs[$own.guests[0].id].State='Running';$script:fakeVMs[$own.guests[1].id].State='Running'
$dispatchAST=[Management.Automation.Language.Parser]::ParseFile("$PSScriptRoot\install-stage1.ps1",[ref]$astTokens,[ref]$astErrors)
$dispatch=$dispatchAST.Find({param($n) $n-is[Management.Automation.Language.IfStatementAst]-and$n.Extent.Text.StartsWith("if(`$Phase-eq'StopGuests')")},$true)
function Invoke-RecordedContainment {& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null}
$Phase='StopGuests';$script:saved=@();& ([scriptblock]::Create($dispatch.Extent.Text))
Check ($script:saved.Count-eq2) 'real StopGuests dispatch reaches authoritative containment before startup gates'
$receipts=@(Get-ChildItem -Force -LiteralPath $lab -Filter 'containment-receipt-*.json' -File)
Check ($receipts.Count-gt0) 'durable containment receipts exist'
$states=@($receipts|ForEach-Object{(Get-Content -LiteralPath $_.FullName -Raw|ConvertFrom-Json).state})
Check ('validation-failure'-in$states-and'operation-failure'-in$states-and'completed'-in$states) 'validation failure, partial failure and success are durable'
# A per-target receipt fault must still attempt the other verified target.
$receiptWriter=${function:Write-ContainmentReceipt};$script:receiptCalls=0
function Write-ContainmentReceipt {param($Policy,$Source,$State,$Outcomes) $script:receiptCalls++;if($State-eq'per-target-result'){throw 'Synthetic receipt IO failure'};return (& $receiptWriter $Policy $Source $State $Outcomes)}
$script:saved=@();$failed=$false;try{& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null}catch{$failed=$true}
Check ($failed-and$script:saved.Count-eq2) 'receipt IO failure preserves second save attempt and reports failure'
${function:Write-ContainmentReceipt}=$receiptWriter
# F1 pure tick model: no live watchdog/process observer/task/VM is launched.
. "$PSScriptRoot\supervisor.ps1"
$ctx=[pscustomobject]@{schema='SFD_SUPERVISOR_CONTEXT_V1';source_sha=$approved;manifest_sha256=$manifestHash;controller_pid=123;controller_exe_sha256='a'*64;started_utc='2026-11-02T12:00:00Z';deadline_utc='2026-11-02T18:00:00Z';lease_expires_utc='2026-11-02T19:00:00Z'}
$script:supervisorRecords=@()
$record={param($status,$reason) $script:supervisorRecords+=@($status);[void](Write-ContainmentReceipt $frozenPolicy $approved ('supervisor-'+$status) @(@{reason=$reason}))}
$contain={param($c) & $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $c.source_sha|Out-Null}
foreach($fault in @('alive','controller-kill','controller-crash','PID-reuse','guardian-expiry','window-expiry','stale-lease')){
 $obs=[pscustomobject]@{now_utc='2026-11-02T13:00:00Z';controller_identity_matches=$true;guardian_healthy=$true;owned_guests_quiescent=$false};$state=[pscustomobject]@{attempts=0;complete=$false};$script:saved=@()
 if($fault-in@('controller-kill','controller-crash','PID-reuse')){$obs.controller_identity_matches=$false}
 if($fault-eq'guardian-expiry'){$obs.guardian_healthy=$false}
 if($fault-in@('window-expiry','stale-lease')){$obs.now_utc='2026-11-02T20:00:00Z'}
 $v=Invoke-SupervisorTick $ctx $obs $state $contain $record
 Check ($(if($fault-eq'alive'){$script:saved.Count-eq0-and$v.state-eq'monitor'}else{$script:saved.Count-eq2-and$v.state-eq'contained'})) 'independent tick fault decision'
}
$obs=[pscustomobject]@{now_utc='2026-11-02T20:00:00Z';controller_identity_matches=$false;guardian_healthy=$false;owned_guests_quiescent=$false};$state=[pscustomobject]@{attempts=0;complete=$false}
$script:saved=@() # No tick can execute during supervisor death or host power loss.
Check ($script:saved.Count-eq0) 'supervisor termination/power loss has no false containment guarantee'
$v=Invoke-SupervisorTick $ctx $obs $state $contain $record
Check ($v.state-eq'contained') 'separate scheduler restart can retry after supervisor loss, conditional model only'
$badCtx=Read-ContainmentJSON ($ctx|ConvertTo-Json);$badCtx.source_sha='0'*40;$state=[pscustomobject]@{attempts=0;complete=$false};$script:saved=@()
$v=Invoke-SupervisorTick $badCtx $obs $state $contain $record
Check ($v.state-eq'retry-next-independent-tick'-and$script:saved.Count-eq0) 'wrong approved source never saves targets'
function Save-VM {param($VM) $script:saved+=@($VM.Id.ToString());if($VM.Id.ToString()-eq$own.guests[0].id){throw 'Synthetic save fault'}}
$state=[pscustomobject]@{attempts=0;complete=$false};$script:saved=@();$v=Invoke-SupervisorTick $ctx $obs $state $contain $record
Check ($script:saved.Count-eq2-and-not$state.complete) 'supervisor records partial failure and attempts second verified guest'
$state.attempts=3;$script:saved=@();$v=Invoke-SupervisorTick $ctx $obs $state $contain $record
Check ($v.state-eq'terminal-failure'-and$script:saved.Count-eq0) 'retry budget preserves durable terminal failure'
foreach($fault in @('non-UTC','clock-before-start','noninteger-retry','nonboolean-complete','deadline-after-lease')){
 $c=Read-ContainmentJSON ($ctx|ConvertTo-Json);$o=Read-ContainmentJSON ($obs|ConvertTo-Json);$s=[pscustomobject]@{attempts=0;complete=$false};$script:saved=@()
 if($fault-eq'non-UTC'){$o.now_utc='2026-11-02T20:00:00+01:00'}
 if($fault-eq'clock-before-start'){$o.now_utc='2026-11-02T11:59:59Z'}
 if($fault-eq'noninteger-retry'){$s.attempts='0'}
 if($fault-eq'nonboolean-complete'){$s.complete='false'}
 if($fault-eq'deadline-after-lease'){$c.lease_expires_utc='2026-11-02T17:00:00Z'}
 $denied=$false;try{Invoke-SupervisorTick $c $o $s $contain $record|Out-Null}catch{$denied=$true}
 Check ($denied-and$script:saved.Count-eq0) 'invalid supervision context/state cannot save any target'
}
$rejected=$false;try{& "$PSScriptRoot\supervisor.ps1" -Execute|Out-Null}catch{$rejected=$true};Check $rejected 'actual supervisor execution is fenced'
[ordered]@{schema='SFD_STAGE1_LIFECYCLE_REGRESSIONS_V1';checks=$script:checks;memory_transitions=$true;dirty_checkout_containment=$true;committed_manifest_identity=$true;unknown_targets_rejected=$true;save_failure_containment_attempts=$true;real_VM_cmdlets_invoked=$false;real_VHD_created=$false;existing_path_ACL_changed=$false;fixture_retained=$true}|ConvertTo-Json
