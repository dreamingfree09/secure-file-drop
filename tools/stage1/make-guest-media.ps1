# Called only by the authorised host installer after creating a new VM.
function New-IdentityMedia($VM,[string]$Role,[string]$Directory){
 $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
 $payload=Join-Path $Directory 'identity-payload';Assert-NewPath $payload
 [Sfd.Stage1.HostNative]::NewPrivateDirectory($payload,$sid)
 $settings=@(Get-CimInstance -Namespace root/virtualization/v2 -ClassName Msvm_VirtualSystemSettingData|Where-Object{$_.VirtualSystemIdentifier-eq$VM.Id.ToString()-and$_.VirtualSystemType-eq'Microsoft:Hyper-V:System:Realized'})
 if($settings.Count-ne1-or$settings[0].BIOSGUID-notmatch'^\{?[0-9a-fA-F-]{36}\}?$'){throw 'STOP:new-guest-BIOS-identity'}
 $identity=[ordered]@{schema='SFD_STAGE1_GUEST_V1';work_id=$script:Policy.work_id;role=$Role;vm_id=$VM.Id.ToString();bios_uuid=$settings[0].BIOSGUID.Trim('{}').ToLowerInvariant();source_sha=$script:Approval.source.sha;root_thumbprint=$null;root_der_sha256=$null}
 New-File (Join-Path $payload 'identity.json') ($identity|ConvertTo-Json)
 foreach($name in @('guest-windows.ps1','guest-linux.sh','acceptance.json')){
  $src=Join-Path $PSScriptRoot $name;$dest=Join-Path $payload $name;$a=[IO.File]::OpenRead($src);$b=[IO.File]::Open($dest,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);try{$a.CopyTo($b);$b.Flush($true)}finally{$a.Dispose();$b.Dispose()}
 }
 $image=New-Object -ComObject IMAPI2FS.MsftFileSystemImage
 $image.FileSystemsToCreate=3;$image.VolumeName='SFDIDENTITY';$image.Root.AddTree($payload,$false)
 $result=$image.CreateResultImage();$iso=Join-Path $Directory 'identity.iso'
 [Sfd.Stage1.HostNative]::SaveIso($result.ImageStream,$iso)
 return [pscustomobject]@{path=$iso;sha256=Get-Hash $iso;bios_uuid=$identity.bios_uuid}
}
