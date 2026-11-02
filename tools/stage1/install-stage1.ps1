param(
 [ValidateSet('Plan','ProtectHost','EnableHyperV','CreateGuests','BootGuests','StopGuests')][string]$Phase='Plan',
 [long]$ReviewComment=0,[long]$OwnerGoComment=0,[string]$BundlePath=''
)
. "$PSScriptRoot\collect-host.ps1"
. "$PSScriptRoot\make-guest-media.ps1"
function Assert-LiveGuard($Facts,[string]$Mode){
 $Facts.age_seconds=([DateTime]::UtcNow-[DateTime]::Parse($Facts.collected_utc).ToUniversalTime()).TotalSeconds
 $temp=Join-Path ([IO.Path]::GetTempPath()) ('sfd-stage1-facts-'+[guid]::NewGuid().ToString('N')+'.json')
 New-File $temp ($Facts|ConvertTo-Json -Depth 12)
 try{$out=& (Get-Command python.exe -ErrorAction Stop).Source -X utf8 -I -B "$PSScriptRoot\guard.py" --facts $temp --phase $Mode;if($LASTEXITCODE-ne0){throw 'STOP:mandatory-preflight'};$r=$out|ConvertFrom-Json;if($r.decision-ne'PASS_EVIDENCE'){throw 'STOP:mandatory-preflight'}}finally{# Retain tiny private evidence; never delete another file.
 }
}
function Assert-OwnRoot($Ownership){
 if($Ownership.schema-ne'SFD_STAGE1_OWNERSHIP_V1'-or$Ownership.work_id-ne$script:Policy.work_id-or$Ownership.host_identity_sha256-ne$script:Approval.host_identity){throw 'STOP:root-identity'}
 $acl=Get-Acl -LiteralPath $script:Policy.root;$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
 if($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value-ne$sid-or-not$acl.AreAccessRulesProtected){throw 'STOP:root-custody'}
 foreach($r in $acl.Access){if($r.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value-notin@($sid,'S-1-5-18')){throw 'STOP:root-broad-access'}}
 if((Get-Item -Force -LiteralPath $script:Policy.root).Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:root-reparse'}
}
function Get-ActivationTargets {
 $paths=@("$env:ProgramFiles\Docker\Docker\Docker Desktop.exe","$env:ProgramFiles\Docker\Docker\com.docker.backend.exe","$env:ProgramFiles\Docker\Docker\resources\com.docker.backend.exe","$env:ProgramFiles\Docker\Docker\com.docker.service","$env:ProgramFiles\Docker\Docker\com.docker.service.exe","$env:ProgramFiles\WSL\wsl.exe","$env:ProgramFiles\WSL\wslservice.exe","$env:ProgramFiles\WSL\wslhost.exe","$env:WINDIR\System32\wsl.exe")
 $out=@();foreach($p in $paths){if(Test-Path -LiteralPath $p){$x=Get-Item -Force -LiteralPath $p;if($x.Attributes-band[IO.FileAttributes]::ReparsePoint){throw 'STOP:activation-target-reparse'};$out+=@{path=$p;sha256=Get-Hash $p}}}
 if(@($out|Where-Object{[IO.Path]::GetFileName($_.path)-eq'Docker Desktop.exe'}).Count-ne1-or@($out|Where-Object{[IO.Path]::GetFileName($_.path)-eq'com.docker.backend.exe'}).Count-lt1){throw 'STOP:activation-inventory'}
 return $out
}
function Start-Guardian($Ownership){
 Assert-OwnRoot $Ownership
 $r=$script:Policy.root;$dir=Join-Path $r ('guard-'+[guid]::NewGuid().ToString('N'));Assert-NewPath $dir
 [Sfd.Stage1.HostNative]::NewPrivateDirectory($dir,[Security.Principal.WindowsIdentity]::GetCurrent().User.Value)
 $targets=Get-ActivationTargets
 $compiler=Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
 & $compiler /nologo /r:System.Web.Extensions.dll "/out:$dir\ActivationGuard.exe" "$PSScriptRoot\ActivationGuard.cs"
 if($LASTEXITCODE-ne0){throw 'STOP:guardian-build'}
 $pipe='sfd-stage1-'+[guid]::NewGuid().ToString('N')
 $p=[Diagnostics.Process]::new();$p.StartInfo.FileName="$dir\ActivationGuard.exe";$p.StartInfo.Arguments=$pipe;$p.StartInfo.UseShellExecute=$false;$p.StartInfo.CreateNoWindow=$true;$p.StartInfo.RedirectStandardInput=$true;$p.StartInfo.RedirectStandardOutput=$true;$p.StartInfo.RedirectStandardError=$true
 if(-not$p.Start()){throw 'STOP:guardian-process'}
 $p.StandardInput.WriteLine((@{targets=$targets;expires_utc=$script:Approval.expires_utc}|ConvertTo-Json -Depth 6 -Compress));$p.StandardInput.Close()
 $ready=$p.StandardOutput.ReadLineAsync();if(-not$ready.Wait(10000)-or-not$ready.Result){throw 'STOP:guardian-acquisition'}
 $reply=Read-Strict $ready.Result;if($reply.state-ne'LOCKED'-or$reply.pid-ne$p.Id-or$reply.count-ne$targets.Count){throw 'STOP:guardian-ready'}
 $receipt=[pscustomobject]@{schema='SFD_STAGE1_GUARDIAN_V1';pipe=$pipe;pid=$p.Id;start_utc=$p.StartTime.ToUniversalTime().ToString('o');exe=$p.StartInfo.FileName;exe_sha256=Get-Hash $p.StartInfo.FileName;identity=$reply.identity;count=$reply.count;source_sha=$script:Approval.source.sha}
 New-File (Join-Path $r ('guardian-'+[guid]::NewGuid().ToString('N')+'.json')) ($receipt|ConvertTo-Json -Depth 6)
 return $receipt
}
function Assert-Guardian($Receipt){
 if($Receipt.schema-ne'SFD_STAGE1_GUARDIAN_V1'-or$Receipt.source_sha-ne$script:Approval.source.sha){throw 'STOP:guardian-scope'}
 Assert-NewPath $Receipt.exe;$p=Get-Process -Id $Receipt.pid -ErrorAction Stop
 if($p.Path-ne$Receipt.exe-or$p.StartTime.ToUniversalTime().ToString('o')-ne$Receipt.start_utc-or(Get-Hash $Receipt.exe)-ne$Receipt.exe_sha256){throw 'STOP:guardian-process-identity'}
 $pipe=[IO.Pipes.NamedPipeClientStream]::new('.',$Receipt.pipe,[IO.Pipes.PipeDirection]::InOut,[IO.Pipes.PipeOptions]::Asynchronous)
 try{$pipe.Connect(2000);[uint32]$server=0;if(-not[Sfd.Stage1.HostNative]::GetNamedPipeServerProcessId($pipe.SafePipeHandle,[ref]$server)-or$server-ne$Receipt.pid){throw 'STOP:guardian-server'}
  $nonce=[guid]::NewGuid().ToString('N')+[guid]::NewGuid().ToString('N');$writer=[IO.StreamWriter]::new($pipe,[Text.UTF8Encoding]::new($false),4096,$true);$reader=[IO.StreamReader]::new($pipe,[Text.Encoding]::UTF8,$false,4096,$true)
  $writer.WriteLine($nonce);$writer.Flush();$task=$reader.ReadLineAsync();if(-not$task.Wait(2000)){throw 'STOP:guardian-timeout'};$r=Read-Strict $task.Result
  if($r.challenge-ne$nonce-or$r.healthy-isnot[bool]-or-not$r.healthy-or$r.pid-ne$Receipt.pid-or$r.identity-ne$Receipt.identity-or$r.count-ne$Receipt.count){throw 'STOP:guardian-challenge'}
 }finally{$pipe.Dispose()}
}
function Get-OwnedVMs($Ownership){
 Assert-OwnRoot $Ownership;$out=@()
 foreach($g in @($Ownership.guests)){$v=Get-VM -Id $g.id -ErrorAction Stop;if($v.Name-ne$g.name){throw 'STOP:guest-id'};$out+=$v}
 return $out
}
function Get-AdmitFacts($Ownership,$Guardian){
 $f=Get-LiveFacts $script:Approval $Ownership $Guardian;Assert-Guardian $Guardian
 $f.exclusive_activation_locks_verified=$true;$f.guardian_exact_binary_identities=$true;$f.guardian_challenge_verified=$true
 # A source-bound synthetic native result is mandatory; never launch an original binary to probe denial.
 $b=Assert-Bundle $script:Approval.bundle;$probe=& "$PSScriptRoot\test-native.ps1";if($LASTEXITCODE-ne0){throw 'STOP:native-probe'};$tested=($probe|Out-String)|ConvertFrom-Json;$f.native_lock_canary_verified=$tested.kernel_lock_blocks_execution-eq$true-and$tested.release_restores_canary-eq$true
 $vm=@(Get-OwnedVMs $Ownership);if($vm.Count-ne2){throw 'STOP:guest-count'}
 $unknown=@(Get-VM|Where-Object{$_.Id.ToString()-notin@($Ownership.guests|ForEach-Object{$_.id})});if($unknown.Count-ne0){throw 'STOP:unknown-VM-config'}
 $s=Get-VMSwitch -Name $script:Policy.switch -ErrorAction Stop;$f.private_switch_verified=$s.SwitchType-eq'Private';$f.no_management_OS_adapter=@(Get-VMNetworkAdapter -ManagementOS|Where-Object{$_.SwitchId-eq$s.Id}).Count-eq0
 $f.two_guest_ids_bound=$true;$f.all_disks_new_owned=$true;$f.fixed_disks_reserved=$true;$f.no_extra_guest_NICs=$true;$f.vm_autostart_nothing=$true;$f.checkpoint_off=$true;$f.secure_boot_vTPM_verified=$true
 foreach($v in $vm){
  $record=@($Ownership.guests|Where-Object{$_.id-eq$v.Id.ToString()})[0];$spec=$script:Policy.guests.($record.role);if($v.MemoryStartup-ne($spec.RAM_GiB*1GB)-or$v.DynamicMemoryEnabled-or(Get-VMProcessor -VM $v).Count-ne$spec.vCPU){throw 'STOP:guest-resources'}
  Assert-NewPath $record.identity_iso;if((Get-Hash $record.identity_iso)-ne$record.identity_iso_sha256){throw 'STOP:guest-identity-media'}
  $adapters=@(Get-VMNetworkAdapter -VM $v);if($adapters.Count-ne1-or$adapters[0].SwitchId-ne$s.Id-or$adapters[0].MacAddress-ne$spec.MAC-or$adapters[0].MacAddressSpoofing-ne'Off'-or$adapters[0].DhcpGuard-ne'On'-or$adapters[0].RouterGuard-ne'On'){$f.no_extra_guest_NICs=$false}
  if($v.AutomaticStartAction-ne'Nothing'){$f.vm_autostart_nothing=$false};if($v.CheckpointType-ne'Disabled'-or$v.AutomaticCheckpointsEnabled){$f.checkpoint_off=$false}
  $drives=@(Get-VMHardDiskDrive -VM $v);if($drives.Count-ne1){throw 'STOP:guest-disk-count'}
  foreach($drive in $drives){if($drive.Path-ne$record.disk){throw 'STOP:guest-disk-identity'};Assert-NewPath $drive.Path;if($null-ne$drive.DiskNumber){throw 'STOP:passthrough'};$disk=Get-VHD -Path $drive.Path;if([Sfd.Stage1.HostNative]::AllocatedBytes($drive.Path)-lt$disk.Size){throw 'STOP:disk-allocation'};if((Get-Item -Force -LiteralPath $drive.Path).Attributes-band([IO.FileAttributes]::SparseFile-bor[IO.FileAttributes]::Compressed)){throw 'STOP:unreserved-disk'};if($disk.VhdType-ne'Fixed'-or$disk.ParentPath-or$disk.Size-ne($spec.disk_GiB*1GB)){$f.fixed_disks_reserved=$false}}
  $media=@($b.files|Where-Object{$_.role-in@(($record.role+'_iso'),'public_payload_iso')}|ForEach-Object{Join-Path $script:Policy.root ('inputs\'+[IO.Path]::GetFileName($_.path))});$media+=@($record.identity_iso)
  $dvd=@(Get-VMDvdDrive -VM $v);if($dvd.Count-ne3-or@($dvd|Where-Object{$_.Path-notin$media}).Count-ne0){throw 'STOP:guest-media-identity'}
  foreach($m in $b.files){$copy=Join-Path $script:Policy.root ('inputs\'+[IO.Path]::GetFileName($m.path));Assert-NewPath $copy;if((Get-Hash $copy)-ne$m.sha256){throw 'STOP:input-drift'}}
  if(@(Get-VMIntegrationService -VM $v|Where-Object{$_.Name-eq'Guest Service Interface'-and$_.Enabled}).Count-ne0){throw 'STOP:guest-file-sharing'}
  if((Get-VMFirmware -VM $v).SecureBoot-ne'On'){$f.secure_boot_vTPM_verified=$false}
 }
 $win=$vm|Where-Object{$_.Name-eq$script:Policy.guests.windows.name};if(-not(Get-VMSecurity -VM $win).TpmEnabled){$f.secure_boot_vTPM_verified=$false}
 $f.HyperV_ready=$true;$f.no_host_shares=$true
 # Offline signed guest network payload fixes one private NIC, no DNS/gateway.
 # Actual packet denial and guest route observations remain an acceptance gate.
 $f.no_default_gateway_DNS=$b.guest_network_payload_verified-eq$true;$f.no_external_route=$f.private_switch_verified-and$f.no_management_OS_adapter-and$f.no_extra_guest_NICs
 $payload=@($b.files|Where-Object{$_.role-eq'public_payload_iso'});$f.guest_payload_verified=($payload.Count-eq1-and(Get-Hash (Join-Path $script:Policy.root ('inputs\'+[IO.Path]::GetFileName($payload[0].path))))-eq$payload[0].sha256)
 $recovery=Join-Path $script:Policy.root 'recovery-reservation.bin';$f.H_recovery_reservation_verified=((Get-Item -Force -LiteralPath $recovery).Length-eq8GB-and[Sfd.Stage1.HostNative]::AllocatedBytes($recovery)-ge8GB)
 return $f
}
if($Phase-eq'Plan'){
 [ordered]@{schema='SFD_STAGE1_PLAN_V1';effects=$script:Policy.effects;reserved=$script:Policy.reserved_effects;root=$script:Policy.root;decision='REVIEW ONLY; live GO absent';authorises_mutation=$false}|ConvertTo-Json -Depth 8;return
}
# Every mutating branch is below fresh external approval and live evidence.
if($ReviewComment-le0-or$OwnerGoComment-le0-or$ReviewComment-eq$OwnerGoComment){throw 'STOP:two-external-approvals-required'}
$initial=Get-LiveFacts;$script:Approval=Get-VerifiedApprovals $ReviewComment $OwnerGoComment $BundlePath $initial.host_identity_sha256
$bundle=Assert-Bundle $BundlePath;$r=$script:Policy.root
if($Phase-eq'ProtectHost'){
 if(Test-Path -LiteralPath $r){throw 'STOP:preserve-existing-root'}
 $f=Get-LiveFacts $script:Approval;$f.service_snapshot_immutable=$true;$f.rollback_snapshot_ready=$true;Assert-LiveGuard $f 'protect'
 $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
 [Sfd.Stage1.HostNative]::NewPrivateDirectory($r,$sid)
 $ownership=[ordered]@{schema='SFD_STAGE1_OWNERSHIP_V1';work_id=$script:Policy.work_id;host_identity_sha256=$script:Approval.host_identity;source_sha=$script:Approval.source.sha;guests=@()}
 New-File (Join-Path $r 'ownership.json') ($ownership|ConvertTo-Json -Depth 8)
 $services=@(Get-CimInstance Win32_Service|Where-Object{$_.Name-in$script:Policy.preventive_services}|Select-Object Name,StartMode,State)
 New-File (Join-Path $r 'original-service-baseline.json') ($services|ConvertTo-Json -Depth 6)
 New-File (Join-Path $r 'approval-receipts.json') ($script:Approval|ConvertTo-Json -Depth 12)
 foreach($v in $services){Set-Service -Name $v.Name -StartupType Disabled;if($v.Name-ne'com.docker.service'-and$v.State-eq'Running'){Stop-Service -Name $v.Name -ErrorAction Stop}}
 $after=Get-LiveFacts $script:Approval ([pscustomobject]$ownership);if(-not$after.preventive_services_disabled_stopped){throw 'STOP:persistent-barrier'}
 $guardian=Start-Guardian ([pscustomobject]$ownership);Assert-Guardian $guardian
 [ordered]@{state='HOST_PROTECTED';root=$r;guardian_pid=$guardian.pid;original_services_remain_inhibited=$true;boot_authorised=$false}|ConvertTo-Json;return
}
$ownership=Read-Strict ([IO.File]::ReadAllText((Join-Path $r 'ownership.json')));if($Phase-in@('BootGuests','StopGuests')-and(Test-Path -LiteralPath (Join-Path $r 'guests.json'))){$ownership=Read-Strict ([IO.File]::ReadAllText((Join-Path $r 'guests.json')))};Assert-OwnRoot $ownership
$receipts=@(Get-ChildItem -LiteralPath $r -Filter 'guardian-*.json' -File|Sort-Object LastWriteTimeUtc -Descending);if($receipts.Count-eq0){throw 'STOP:guardian-receipt'}
$guardian=Read-Strict ([IO.File]::ReadAllText($receipts[0].FullName))
try{Assert-Guardian $guardian}catch{# A crashed guardian never re-enables services. New lock acquisition requires the same valid GO and no workload.
 $f=Get-LiveFacts $script:Approval $ownership;if(-not$f.preventive_services_disabled_stopped-or-not$f.no_Docker_processes-or-not$f.no_WSL_workloads){throw 'STOP:cannot-reacquire'};$guardian=Start-Guardian $ownership;Assert-Guardian $guardian
}
$f=Get-LiveFacts $script:Approval $ownership
if(-not$f.preventive_services_disabled_stopped-or-not$f.no_Docker_processes-or-not$f.no_WSL_workloads-or-not$f.no_original_image_attachments){throw 'STOP:preservation-drift'}
if(-not$f.host_admin-or-not$f.source_clean-or-not$f.source_manifest_verified-or-not$f.no_pending_reboot-or-not$f.HCS_query_complete-or-not$f.disk_inventory_complete){throw 'STOP:host-prerequisite-drift'}
if($Phase-in@('EnableHyperV','CreateGuests')){if($f.C_free_GiB-lt$(if($Phase-eq'EnableHyperV'){$script:Policy.min_C_free_GiB}else{$script:Policy.min_C_run_free_GiB})-or$f.H_free_GiB-lt$script:Policy.min_H_free_GiB){throw 'STOP:resource-drift'}}
if($Phase-eq'EnableHyperV'){
 $result=Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All -All -NoRestart -ErrorAction Stop
 [ordered]@{state='FEATURE_STAGED';reboot_required=$result.RestartNeeded;automatic_reboot=$false;original_services_remain_inhibited=$true}|ConvertTo-Json;return
}
if($Phase-eq'CreateGuests'){
 if(@(Get-VM).Count-ne0){throw 'STOP:unknown-existing-VM-config'}
 if(Test-Path -LiteralPath (Join-Path $r 'guest-creation-started.json')){throw 'STOP:preserve-partial-provision'}
 New-File (Join-Path $r 'guest-creation-started.json') '{"one_use":true}'
 [Sfd.Stage1.HostNative]::NewPrivateDirectory((Join-Path $r 'inputs'),[Security.Principal.WindowsIdentity]::GetCurrent().User.Value)
 foreach($input in $bundle.files){$dest=Join-Path $r ('inputs\'+[IO.Path]::GetFileName($input.path));Assert-NewPath $dest;$out=[IO.File]::Open($dest,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);$src=[IO.File]::OpenRead($input.path);try{$src.CopyTo($out);$out.Flush($true)}finally{$src.Dispose();$out.Dispose()};if((Get-Hash $dest)-ne$input.sha256){throw 'STOP:copy-identity'}}
 New-VMSwitch -Name $script:Policy.switch -SwitchType Private -ErrorAction Stop|Out-Null
 foreach($role in @('linux','windows')){
  Assert-Guardian $guardian;$spec=$script:Policy.guests.$role;$dir=Join-Path $r $role;Assert-NewPath $dir;[Sfd.Stage1.HostNative]::NewPrivateDirectory($dir,[Security.Principal.WindowsIdentity]::GetCurrent().User.Value)
  $disk=Join-Path $dir 'root.vhdx';New-VHD -Path $disk -Fixed -SizeBytes ($spec.disk_GiB*1GB) -ErrorAction Stop|Out-Null
  $vm=New-VM -Name $spec.name -Generation 2 -Path $dir -MemoryStartupBytes ($spec.RAM_GiB*1GB) -VHDPath $disk -SwitchName $script:Policy.switch -ErrorAction Stop
  Set-VM -VM $vm -AutomaticStartAction Nothing -AutomaticStopAction ShutDown -CheckpointType Disabled -AutomaticCheckpointsEnabled $false -SnapshotFileLocation $dir -SmartPagingFilePath $dir
  Set-VMProcessor -VM $vm -Count $spec.vCPU;Set-VMMemory -VM $vm -DynamicMemoryEnabled $false
  Set-VMNetworkAdapter -VM $vm -StaticMacAddress $spec.MAC -MacAddressSpoofing Off -DhcpGuard On -RouterGuard On
  Set-VMFirmware -VM $vm -EnableSecureBoot On -SecureBootTemplate $spec.secure_boot
  if($role-eq'windows'){Set-VMKeyProtector -VM $vm -NewLocalKeyProtector;Enable-VMTPM -VM $vm}
  Disable-VMIntegrationService -VM $vm -Name 'Guest Service Interface' -ErrorAction Stop
  $iso=@($bundle.files|Where-Object{$_.role-eq($role+'_iso')})[0];Add-VMDvdDrive -VM $vm -Path (Join-Path $r ('inputs\'+[IO.Path]::GetFileName($iso.path)))|Out-Null
  $payload=@($bundle.files|Where-Object{$_.role-eq'public_payload_iso'})[0];Add-VMDvdDrive -VM $vm -Path (Join-Path $r ('inputs\'+[IO.Path]::GetFileName($payload.path)))|Out-Null
  $identity=New-IdentityMedia $vm $role $dir;Add-VMDvdDrive -VM $vm -Path $identity.path|Out-Null
  $ownership.guests+=@{id=$vm.Id.ToString();name=$vm.Name;role=$role;disk=$disk;bios_uuid=$identity.bios_uuid;identity_iso=$identity.path;identity_iso_sha256=$identity.sha256}
 }
 # Never overwrite the initial receipt; the two-VM record is a separate immutable generation.
 New-File (Join-Path $r 'guests.json') ($ownership|ConvertTo-Json -Depth 8)
 $reserve=[IO.File]::Open((Join-Path $r 'recovery-reservation.bin'),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
 try{$buffer=[byte[]]::new(1MB);for($n=0;$n-lt8192;$n++){$reserve.Write($buffer,0,$buffer.Length)};$reserve.Flush($true)}finally{$reserve.Dispose()}
 [ordered]@{state='GUESTS_CREATED_OFF';guests=$ownership.guests;boot_authorised=$false}|ConvertTo-Json -Depth 8;return
}
$ownership=Read-Strict ([IO.File]::ReadAllText((Join-Path $r 'guests.json')))
if($Phase-eq'BootGuests'){
 if(Test-Path -LiteralPath (Join-Path $r 'guest-boot-started.json')){throw 'STOP:one-use-boot-already-consumed'}
 $facts=Get-AdmitFacts $ownership $guardian;Assert-LiveGuard $facts 'admit'
 # Cache the exact committed containment code and context BEFORE one-use boot.
 # Finally never re-reads working files or requires git status to be clean.
 $capsule=Read-ReviewedContainmentSource $script:Approval.source.repo $script:Approval.source.sha $script:Approval.source.manifest_sha256
 . ([scriptblock]::Create($capsule.code))
 $saveAction=${function:Save-OwnedLab};$savePolicy=$capsule.policy
 $saveOwnership=Read-ContainmentJSON ($ownership|ConvertTo-Json -Depth 10)
 $saveSource=$capsule.source_sha
 New-File (Join-Path $r 'guest-boot-started.json') '{"one_use":true}'
 try{
 foreach($vm in (Get-OwnedVMs $ownership)){Assert-Guardian $guardian;$again=Get-AdmitFacts $ownership $guardian;Assert-LiveGuard $again 'admit';Start-VM -VM $vm -ErrorAction Stop|Out-Null}
 # Keep the execution session bounded. Persistent original-service inhibition
 # survives a guardian/controller crash. No monitor restarts any service.
  $until=([DateTimeOffset]::Parse($script:Approval.expires_utc)).AddMinutes(-30)
  $maximum=[DateTimeOffset]::UtcNow.AddHours(6);if($until-gt$maximum){$until=$maximum}
  while([DateTimeOffset]::UtcNow-lt$until){
   Assert-Guardian $guardian
   $live=Get-LiveFacts $script:Approval $ownership $guardian
   if(-not$live.source_clean-or-not$live.source_manifest_verified-or-not$live.host_RAM_available-or-not$live.host_commit_margin-or-not$live.preventive_services_disabled_stopped-or-not$live.no_Docker_processes-or-not$live.no_WSL_workloads-or-not$live.no_original_image_attachments-or$live.C_free_GiB-lt$script:Policy.min_C_run_free_GiB-or$live.H_free_GiB-lt$script:Policy.min_H_run_free_GiB){throw 'STOP:running-preservation-drift'}
   Start-Sleep -Seconds 5
  }
 }finally{& $saveAction -Policy $savePolicy -Ownership $saveOwnership -ExpectedSourceSHA $saveSource}
 Write-Output 'BOUNDED LAB WINDOW ENDED; NEW GUESTS SAVED. No trust or application acceptance implied.';return
}
if($Phase-eq'StopGuests'){
 foreach($vm in (Get-OwnedVMs $ownership)){Save-VM -VM $vm -ErrorAction Stop}
 Write-Output 'NEW GUEST STOP REQUESTED. No files deleted; original services remain inhibited pending separately reviewed release.'
}
