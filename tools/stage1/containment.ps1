# Loaded from an approved committed blob before boot. Never reads working
# source, checks checkout cleanliness, starts a guest, or changes a service.
function Assert-ContainmentRoot([string]$Root){
 if([IO.Path]::GetFullPath($Root)-ne$Root-or$Root.Substring(2).Contains(':')){throw 'STOP:containment-root-path'}
 for($p=$Root;$p;$p=[IO.Path]::GetDirectoryName($p)){if((Get-Item -Force -LiteralPath $p -ErrorAction Stop).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:containment-reparse'}}
 $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value;$acl=Get-Acl -LiteralPath $Root
 if(-not$acl.AreAccessRulesProtected-or$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value-ne$sid){throw 'STOP:containment-root-custody'}
 foreach($rule in $acl.Access){if($rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value-notin@($sid,'S-1-5-18')){throw 'STOP:containment-ACL'}}
}
function Write-ContainmentReceipt($Policy,[string]$Source,[string]$State,$Outcomes){
 Assert-ContainmentRoot $Policy.root
 $r=[ordered]@{schema='SFD_CONTAINMENT_RECEIPT_V2';source_sha=$Source;utc=[DateTime]::UtcNow.ToString('o');state=$State;outcomes=@($Outcomes)}
 $path=Join-Path $Policy.root ('containment-receipt-'+[guid]::NewGuid().ToString('N')+'.json')
 $f=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
 try{$b=[Text.UTF8Encoding]::new($false).GetBytes(($r|ConvertTo-Json -Depth 8));$f.Write($b,0,$b.Length);$f.Flush($true)}finally{$f.Dispose()}
 return $path
}
function Save-OwnedLab($Policy,$Ownership,[string]$ExpectedSourceSHA){
 if($ExpectedSourceSHA-notmatch'^[a-f0-9]{40}$'-or$Ownership.schema-ne'SFD_STAGE1_OWNERSHIP_V1'-or$Ownership.work_id-ne$Policy.work_id-or$Ownership.source_sha-ne$ExpectedSourceSHA){throw 'STOP:containment-source'}
 Assert-ContainmentRoot $Policy.root
 $outcomes=@();$began=$false
 try{
 $rows=@($Ownership.guests);if($rows.Count-ne2){throw 'STOP:containment-guest-count'}
 $ids=@();$roles=@();$targets=@()
 # Validate ALL identities before the first Save-VM; a scope mismatch saves none.
 foreach($g in $rows){
  if($g.role-notin@('linux','windows')-or$g.role-in$roles-or$g.id-notmatch'^[a-fA-F0-9-]{36}$'-or$g.id-in$ids){throw 'STOP:containment-guest-record'}
  [void][guid]::Parse($g.id);$ids+=@($g.id);$roles+=@($g.role)
  $expected=Join-Path $Policy.root ($g.role+'\root.vhdx');$spec=$Policy.guests.($g.role)
  if($g.disk-ne$expected-or$g.name-ne$spec.name){throw 'STOP:containment-disk-record'}
  for($p=$expected;$p-and$p-ne$Policy.root;$p=[IO.Path]::GetDirectoryName($p)){if((Test-Path -LiteralPath $p)-and((Get-Item -Force -LiteralPath $p -ErrorAction Stop).Attributes-band[IO.FileAttributes]::ReparsePoint)){throw 'STOP:containment-disk-reparse'}}
  $v=Get-VM -Id $g.id -ErrorAction Stop;$disks=@(Get-VMHardDiskDrive -VM $v -ErrorAction Stop)
  if($v.Id.ToString()-ne$g.id-or$v.Name-ne$spec.name-or$disks.Count-ne1-or$disks[0].Path-ne$expected-or$null-ne$disks[0].DiskNumber){throw 'STOP:containment-VM-scope'}
  $targets+=@($v)
 }
 [void](Write-ContainmentReceipt $Policy $ExpectedSourceSHA 'validated-before-save' @())
 $began=$true;$failed=$false
 foreach($v in $targets){
  $state=$v.State.ToString();$outcome='already-off-or-saved'
  if($state-in@('Running','Paused')){try{Save-VM -VM $v -ErrorAction Stop;$outcome='save-command-completed'}catch{$failed=$true;$outcome='save-failed'}}
  elseif($state-notin@('Saved','Off')){$failed=$true;$outcome='transitional-state-not-forced'}
  $outcomes+=@(@{id=$v.Id.ToString();state=$state;outcome=$outcome})
  # A receipt failure after saving begins must not prevent the second save.
  try{[void](Write-ContainmentReceipt $Policy $ExpectedSourceSHA 'per-target-result' $outcomes)}catch{$failed=$true}
 }
 [void](Write-ContainmentReceipt $Policy $ExpectedSourceSHA $(if($failed){'failed'}else{'completed'}) $outcomes)
 if($failed){throw 'STOP:containment-save-failed; preserve all state; no forced shutdown'}
 Write-Output 'VERIFIED CONTAINMENT COMPLETED; inspect receipts. Original services remain inhibited.'
 }catch{
  # Trusted private root only. Never print exception data or touch other paths.
  [void](Write-ContainmentReceipt $Policy $ExpectedSourceSHA $(if($began){'operation-failure'}else{'validation-failure'}) $outcomes)
  throw
 }
}
