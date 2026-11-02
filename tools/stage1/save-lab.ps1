param([switch]$Execute)
# Emergency containment is permitted after expiry; it can only SAVE exact new
# guests, never boot, power off, delete or re-enable original services.
. "$PSScriptRoot\common.ps1"
if(-not$Execute){Write-Output 'PLAN: Save only source-bound, privately owned lab VMs; no effect without Execute.';return}
$src=Get-SourceIdentity;$r=$script:Policy.root
$o=Read-Strict ([IO.File]::ReadAllText((Join-Path $r 'guests.json')))
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$acl=Get-Acl -LiteralPath $r
if($o.schema-ne'SFD_STAGE1_OWNERSHIP_V1'-or$o.work_id-ne$script:Policy.work_id-or$o.source_sha-ne$src.sha-or@($o.guests).Count-ne2-or-not$acl.AreAccessRulesProtected-or$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value-ne$sid){throw 'STOP:containment-ownership'}
foreach($rule in $acl.Access){if($rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value-notin@($sid,'S-1-5-18')){throw 'STOP:containment-ACL'}}
foreach($g in $o.guests){
 $spec=$script:Policy.guests.($g.role);$v=Get-VM -Id $g.id -ErrorAction Stop
 Assert-NewPath $g.disk
 $disks=@(Get-VMHardDiskDrive -VM $v)
 if($v.Name-ne$spec.name-or$g.name-ne$spec.name-or$disks.Count-ne1-or$disks[0].Path-ne$g.disk){throw 'STOP:containment-guest'}
 if($v.State-eq'Running'-or$v.State-eq'Paused'){Save-VM -VM $v -ErrorAction Stop}
}
Write-Output 'NEW LAB VMS SAVED. No data deleted. Original services remain inhibited; release needs separate preservation review.'
