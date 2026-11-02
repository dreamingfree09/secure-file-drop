param()
. "$PSScriptRoot\common.ps1"
. "$PSScriptRoot\memory.ps1"
if(-not('Sfd.Stage1.HostNative'-as[type])){Add-Type -Path "$PSScriptRoot\HostNative.cs"}
function Get-LiveFacts($Approval=$null,$Ownership=$null,$Guardian=$null){
 $f=[ordered]@{schema='SFD_STAGE1_LIVE_FACTS_V1';collector='collect-host.ps1';no_protected_byte_access=$true;age_seconds=0;collected_utc=[DateTime]::UtcNow.ToString('o')}
 $all=@('source_clean','source_manifest_verified','authority_exact','owner_go_exact','host_admin','HyperV_capable','host_RAM_available','host_commit_margin','host_identity_bound','maintenance_window','no_pending_reboot','H_identity_bound','H_ntfs','H_root_absent_or_owned','no_reparse','no_broad_lab_write','no_Docker_processes','no_WSL_workloads','HCS_query_complete','no_unknown_compute_systems','disk_inventory_complete','no_file_backed_unknown_disks','no_original_image_attachments','media_verified','media_rights_verified','runtime_bundle_verified','service_snapshot_immutable','Docker_service_stopped','rollback_snapshot_ready','preventive_services_disabled_stopped','exclusive_activation_locks_verified','guardian_exact_binary_identities','native_lock_canary_verified','guardian_challenge_verified','HyperV_ready','fixed_disks_reserved','private_switch_verified','no_management_OS_adapter','two_guest_ids_bound','all_disks_new_owned','no_extra_guest_NICs','no_host_shares','no_default_gateway_DNS','no_external_route','guest_payload_verified','vm_autostart_nothing','checkpoint_off','secure_boot_vTPM_verified','H_recovery_reservation_verified')
 foreach($k in $all){$f[$k]=$false};$f.C_free_GiB=0;$f.H_free_GiB=0;$f.RAM_GiB=0
 $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
 $os=Get-CimInstance Win32_OperatingSystem;$cpu=@(Get-CimInstance Win32_Processor);$cs=Get-CimInstance Win32_ComputerSystem
 $f.HyperV_capable=($os.OSArchitecture-eq'64-bit'-and$os.Caption-match'Windows (10|11) (Pro|Enterprise|Education)'-and($cs.HypervisorPresent-or@($cpu|Where-Object{-not$_.VirtualizationFirmwareEnabled-or-not$_.SecondLevelAddressTranslationExtensions-or-not$_.VMMonitorModeExtensions}).Count-eq0))
 try{
  $observed=$null
  if($null-ne$Ownership-and@($Ownership.guests).Count-gt0){
   $observed=@();foreach($g in $Ownership.guests){
    $v=Get-VM -Id $g.id -ErrorAction Stop
    if($g.role-notin@('linux','windows')-or$v.Name-ne$script:Policy.guests.($g.role).name){throw 'STOP:memory-VM-identity'}
    $observed+=@([pscustomobject]@{role=$g.role;state=$v.State.ToString();startup_bytes=[long]$v.MemoryStartup;assigned_bytes=[long]$v.MemoryAssigned;dynamic=[bool]$v.DynamicMemoryEnabled})
   }
  }
  $memory=Get-LabMemoryAdmission -Policy $script:Policy -ObservedGuests $observed -FreeRAMBytes ([long]($os.FreePhysicalMemory*1KB)) -FreeCommitBytes ([long]($os.FreeVirtualMemory*1KB))
  $f.memory_required_GiB=$memory.required_bytes/1GB
  $f.host_RAM_available=$memory.RAM_available;$f.host_commit_margin=$memory.commit_margin
 }catch{$f.memory_required_GiB=$null}

 $f.host_admin=([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
 try{$src=Get-SourceIdentity;if($null-ne$Approval-and($src.sha-ne$Approval.source.sha-or$src.manifest_sha256-ne$Approval.source.manifest_sha256)){throw 'source-drift'};$f.source_clean=$true;$f.source_manifest_verified=$true}catch{}
 $c=Get-Volume -DriveLetter C;$h=Get-Volume -DriveLetter H;$f.C_free_GiB=$c.SizeRemaining/1GB;$f.H_free_GiB=$h.SizeRemaining/1GB;$f.RAM_GiB=(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1GB
 $identity=Get-TextHash (([ordered]@{schema='SFD_STAGE1_HOST_V1';SID=$sid;C_volume=$c.UniqueId;H_volume=$h.UniqueId;machine_UUID=(Get-CimInstance Win32_ComputerSystemProduct).UUID}|ConvertTo-Json -Compress))
 $f.host_identity_sha256=$identity;$f.H_ntfs=$h.FileSystemType-eq'NTFS'
 $f.no_reparse=-not((Get-Item -Force -LiteralPath 'H:\').Attributes-band[IO.FileAttributes]::ReparsePoint)
 $exists=Test-Path -LiteralPath $script:Policy.root
 if(-not$exists){$f.H_root_absent_or_owned=$true;$f.no_broad_lab_write=$true}
 elseif($null-ne$Ownership){$acl=Get-Acl -LiteralPath $script:Policy.root;$owner=$acl.GetOwner([Security.Principal.SecurityIdentifier]).Value;$bad=@($acl.Access|Where-Object{$_.AccessControlType-eq'Allow'-and$_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value-notin@($sid,'S-1-5-18')});$f.H_root_absent_or_owned=($Ownership.host_identity_sha256-eq$identity-and$owner-eq$sid);$f.no_broad_lab_write=($acl.AreAccessRulesProtected-and$bad.Count-eq0);$f.no_reparse=$f.no_reparse-and-not((Get-Item -Force -LiteralPath $script:Policy.root).Attributes-band[IO.FileAttributes]::ReparsePoint)}
 $proc=@(Get-CimInstance Win32_Process);$f.no_Docker_processes=@($proc|Where-Object{$_.Name-match'^(Docker Desktop|com\.docker\.(backend|service)|dockerd)(\.exe)?$'}).Count-eq0
 $f.no_WSL_workloads=@($proc|Where-Object{$_.Name-match'^(wsl|wslhost|wslrelay|VmmemWSL)(\.exe)?$'}).Count-eq0
 try{if((Get-Service vmcompute -ErrorAction Stop).Status-ne'Running'){throw 'HCS-not-running'};$systems=@([Sfd.Stage1.HostNative]::Enumerate()|ConvertFrom-Json);$f.HCS_query_complete=$true;$allowed=@();if($null-ne$Ownership){$allowed=@($Ownership.guests|ForEach-Object{$_.id})};$f.no_unknown_compute_systems=@($systems|Where-Object{$_.Id-notin$allowed}).Count-eq0}catch{$f.HCS_error_type=$_.Exception.GetType().Name;$e=$_.Exception;if($e.InnerException){$e=$e.InnerException};$f.HCS_error_hresult=('0x{0:X8}'-f$e.HResult)}
 try{$disks=@(Get-Disk);$f.disk_inventory_complete=$true;$f.no_file_backed_unknown_disks=@($disks|Where-Object{$_.BusType-eq'File Backed Virtual'-or$_.BusType-eq'FileBackedVirtual'-or$_.BusType-eq'Virtual'}).Count-eq0;$f.no_original_image_attachments=$f.HCS_query_complete-and$f.no_unknown_compute_systems-and$f.no_file_backed_unknown_disks}catch{}
 $services=@(Get-CimInstance Win32_Service|Where-Object{$_.Name-in$script:Policy.preventive_services});$docker=@($services|Where-Object{$_.Name-eq'com.docker.service'});$f.Docker_service_stopped=($docker.Count-eq1-and$docker[0].State-eq'Stopped')
 $f.preventive_services_disabled_stopped=($docker.Count-eq1-and@($services|Where-Object{$_.State-ne'Stopped'-or$_.StartMode-ne'Disabled'}).Count-eq0-and@($services|Where-Object{$_.Name-eq'WSLService'-or$_.Name-eq'LxssManager'}).Count-gt0)
 $f.no_pending_reboot=-not((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending')-or(Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'))
 if($null-ne$Approval){$f.authority_exact=$true;$f.owner_go_exact=$true;$f.host_identity_bound=$Approval.host_identity-eq$identity;$f.H_identity_bound=$f.host_identity_bound;$f.maintenance_window=([DateTimeOffset]::Parse($Approval.expires_utc)-gt[DateTimeOffset]::UtcNow);try{[void](Assert-Bundle $Approval.bundle);$f.media_verified=$true;$f.media_rights_verified=$true;$f.runtime_bundle_verified=$true}catch{}}
 # Native guardian and guest identities are never promoted from a JSON report.
 # The installer populates them only after live checks; standalone audit is NO-GO.
 $f.initial_service_metadata=@($services|Select-Object Name,State,StartMode)
 $lastC=Get-Volume -DriveLetter C;$lastH=Get-Volume -DriveLetter H;if($lastH.UniqueId-ne$h.UniqueId-or$lastC.UniqueId-ne$c.UniqueId){$f.H_identity_bound=$false};$f.C_free_GiB=$lastC.SizeRemaining/1GB;$f.H_free_GiB=$lastH.SizeRemaining/1GB;$f.collected_utc=[DateTime]::UtcNow.ToString('o')
 return [pscustomobject]$f
}
if($MyInvocation.InvocationName-ne'.'){Get-LiveFacts|ConvertTo-Json -Depth 8}
