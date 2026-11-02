# Pure arithmetic over observed metadata. No host or VM operation.
function Get-LabMemoryRequirement($Policy,$ObservedGuests=$null){
 [long]$pending=0
 $roles=@('linux','windows')
 if($null-eq$ObservedGuests){foreach($role in $roles){$pending+=[long]$Policy.guests.$role.RAM_GiB*1GB}}
 else{
  $rows=@($ObservedGuests);if($rows.Count-ne2){throw 'STOP:memory-guest-count'}
  $seen=@()
  foreach($g in $rows){
   if($g.role-notin$roles-or$g.role-in$seen){throw 'STOP:memory-guest-role'};$seen+=@($g.role)
   [long]$expected=[long]$Policy.guests.($g.role).RAM_GiB*1GB
   if($g.dynamic-isnot[bool]-or$g.dynamic-or$g.startup_bytes-isnot[long]-or$g.startup_bytes-ne$expected-or$g.assigned_bytes-isnot[long]){throw 'STOP:memory-guest-allocation'}
   if($g.state-in@('Running','Paused')){if($g.assigned_bytes-ne$expected){throw 'STOP:memory-assignment'}}
   elseif($g.state-in@('Off','Saved')){if($g.assigned_bytes-ne0){throw 'STOP:memory-cold-assignment'};$pending+=$expected}
   else{throw 'STOP:memory-unknown-state'}
  }
 }
 if(($Policy.host_RAM_reserve_GiB-isnot[int]-and$Policy.host_RAM_reserve_GiB-isnot[long])-or$Policy.host_RAM_reserve_GiB-ne4){throw 'STOP:memory-reserve'}
 return [long]($pending+[long]$Policy.host_RAM_reserve_GiB*1GB)
}

function Get-LabMemoryAdmission($Policy,$ObservedGuests,[long]$FreeRAMBytes,[long]$FreeCommitBytes){
 [long]$required=Get-LabMemoryRequirement $Policy $ObservedGuests
 return [pscustomobject]@{required_bytes=$required;RAM_available=($FreeRAMBytes-ge$required);commit_margin=($FreeCommitBytes-ge$required)}
}
