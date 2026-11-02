# Synthetic metadata and mock Save-VM only; never invokes a real VM cmdlet.
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
. "$PSScriptRoot\memory.ps1"
. "$PSScriptRoot\save-lab.ps1"
$policy=Get-Content -LiteralPath "$PSScriptRoot\policy.json" -Raw|ConvertFrom-Json
$script:checks=0
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
# New private fixture directories only; no VHD/VHDX is created or attached.
if(-not('Sfd.Stage1.HostNative'-as[type])){Add-Type -Path "$PSScriptRoot\HostNative.cs"}
$fixture=Join-Path ([IO.Path]::GetTempPath()) ('sfd-stage1-lifecycle-'+[guid]::NewGuid().ToString('N'))
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
[Sfd.Stage1.HostNative]::NewPrivateDirectory($fixture,$sid)
$repo=Join-Path $fixture 'repo';$lab=Join-Path $fixture 'new-lab';$empty=Join-Path $fixture 'empty-hooks'
foreach($dir in @($repo,$lab,$empty,(Join-Path $repo 'tools'),(Join-Path $repo 'tools\stage1'),(Join-Path $lab 'linux'),(Join-Path $lab 'windows'))){[Sfd.Stage1.HostNative]::NewPrivateDirectory($dir,$sid)}
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
$script:saved=@();$first=$own.guests[0].id
function Save-VM {param($VM) $script:saved+=@($VM.Id.ToString());if($VM.Id.ToString()-eq$first){throw 'Synthetic first-save failure'}}
$failed=$false;try{& $action -Policy $frozenPolicy -Ownership $own -ExpectedSourceSHA $approved|Out-Null}catch{$failed=$true}
Check ($failed-and$script:saved.Count-eq2) 'one save failure still attempts other verified guest'
[ordered]@{schema='SFD_STAGE1_LIFECYCLE_REGRESSIONS_V1';checks=$script:checks;memory_transitions=$true;dirty_checkout_containment=$true;committed_manifest_identity=$true;unknown_targets_rejected=$true;save_failure_containment_attempts=$true;real_VM_cmdlets_invoked=$false;real_VHD_created=$false;existing_path_ACL_changed=$false;fixture_retained=$true}|ConvertTo-Json
