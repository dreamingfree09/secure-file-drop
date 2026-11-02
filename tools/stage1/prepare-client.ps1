param([Parameter(Mandatory)][string]$IdentityPath,[Parameter(Mandatory)][string]$ZipPath)
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
$id=Get-Content -LiteralPath $IdentityPath -Raw|ConvertFrom-Json
$uuid=(Get-CimInstance Win32_ComputerSystemProduct).UUID.Trim('{}').ToLowerInvariant()
if($id.schema-ne'SFD_STAGE1_GUEST_V1'-or$id.role-ne'windows'-or$id.bios_uuid-ne$uuid-or(Test-Path 'H:\')){throw 'STOP:not-new-client-guest'}
if((Get-FileHash -LiteralPath $ZipPath -Algorithm SHA256).Hash.ToLowerInvariant()-ne'f38b9f82dda5c0320c17a3d0a04de924c958750f8975be28d3914d0ec8b90c9e'){throw 'STOP:client-archive'}
if((Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full').Release-lt528040){throw 'STOP:Framework48-required'}
$root='C:\SFD-Lab\client';if(Test-Path -LiteralPath $root){throw 'STOP:preserve-existing-client'}
$expected=Get-Content -LiteralPath "$PSScriptRoot\client-members.json" -Raw|ConvertFrom-Json
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip=[IO.Compression.ZipFile]::OpenRead($ZipPath)
try{
 if($zip.Entries.Count-ne7){throw 'STOP:seven-files-required'}
 New-Item -ItemType Directory -Path $root -ErrorAction Stop|Out-Null
 foreach($e in $zip.Entries){
  $name=[IO.Path]::GetFileName($e.FullName);if($e.FullName-match'(^/|\\|\.\.|:)'-or-not$name){throw 'STOP:zip-path'}
  $row=@($expected|Where-Object{($_.path-split'/')[-1]-eq$name});if($row.Count-ne1){throw 'STOP:unexpected-client-member'}
  $input=$e.Open();$h=[Security.Cryptography.SHA256]::Create();try{$hash=([BitConverter]::ToString($h.ComputeHash($input))).Replace('-','').ToLowerInvariant()}finally{$input.Dispose();$h.Dispose()}
  if($hash-ne$row[0].SHA256-or$e.Length-ne$row[0].bytes){throw 'STOP:client-member-identity'}
  $input=$e.Open();$output=[IO.File]::Open((Join-Path $root $name),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);try{$input.CopyTo($output);$output.Flush($true)}finally{$input.Dispose();$output.Dispose()}
 }
}finally{$zip.Dispose()}
[ordered]@{prepared=$true;directory=$root;exe_SHA256='8f98bd9dca095c827e8ccf38000ae28fecf45df60ec10c9b484ed479748bb28b';launched=$false;Explorer_registered=$false;trust_changed=$false}|ConvertTo-Json
