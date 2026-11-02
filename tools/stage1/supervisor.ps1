param([switch]$Execute)
# Proposal/fault model only. No scheduler API, loop, process observer or VM call.
function Invoke-SupervisorTick($Context,$Observation,$State,[scriptblock]$Contain,[scriptblock]$Record){
 if($Context.schema-ne'SFD_SUPERVISOR_CONTEXT_V1'-or$Context.source_sha-notmatch'^[a-f0-9]{40}$'-or$Context.manifest_sha256-notmatch'^[a-f0-9]{64}$'-or$Context.controller_pid-le0-or$Context.controller_exe_sha256-notmatch'^[a-f0-9]{64}$'){throw 'STOP:supervisor-context'}
 if($State.attempts-isnot[int]-or$State.complete-isnot[bool]){throw 'STOP:supervisor-state'}
 $began=[DateTimeOffset]::Parse($Context.started_utc);$deadline=[DateTimeOffset]::Parse($Context.deadline_utc);$lease=[DateTimeOffset]::Parse($Context.lease_expires_utc);$now=[DateTimeOffset]::Parse($Observation.now_utc)
 if($began.Offset-ne[TimeSpan]::Zero-or$deadline.Offset-ne[TimeSpan]::Zero-or$lease.Offset-ne[TimeSpan]::Zero-or$now.Offset-ne[TimeSpan]::Zero-or$now-lt$began){throw 'STOP:supervisor-UTC-clock'}
 if($deadline-le$began-or$deadline-gt$began.AddHours(6)-or$deadline-gt$lease-or$State.attempts-lt0-or$State.attempts-gt3){throw 'STOP:supervisor-window'}
 foreach($k in @('controller_identity_matches','guardian_healthy','owned_guests_quiescent')){if($Observation.$k-isnot[bool]){throw 'STOP:supervisor-observation'}}
 $reason=$null
 if($now-ge$deadline-or$now-ge$lease){$reason='window-expired'}
 elseif(-not$Observation.controller_identity_matches){$reason='controller-lost-or-reused'}
 elseif(-not$Observation.guardian_healthy){$reason='guardian-lost-or-expired'}
 if($State.complete-and$Observation.owned_guests_quiescent){return [pscustomobject]@{state='already-contained';attempts=$State.attempts}}
 if($State.complete){$State.complete=$false}
 if($null-eq$reason){return [pscustomobject]@{state='monitor';attempts=$State.attempts}}
 if($State.attempts-ge3){& $Record 'terminal-failure' $reason;return [pscustomobject]@{state='terminal-failure';attempts=3}}
 $State.attempts++; & $Record 'containment-started' $reason
 try{
  # Future adapter MUST load approved committed capsule and call ONLY the same
  # Save-OwnedLab path. A failed/unknown target never becomes a start/force-off.
  & $Contain $Context | Out-Null
  $State.complete=$true;& $Record 'contained' $reason
 }catch{& $Record 'containment-failed' $reason}
 return [pscustomobject]@{state=$(if($State.complete){'contained'}else{'retry-next-independent-tick'});attempts=$State.attempts}
}
if($MyInvocation.InvocationName-eq'.'){return}
if($Execute){throw 'STOP:supervisor-registration-and-execution-not-authorised'}
[ordered]@{schema='SFD_SUPERVISOR_PROPOSAL_V1';execution_enabled=$false;new_effect='independent scheduled save-only task';consent_required=$true;host_power_failure_guarantee=$false;supervisor_kill_guarantee=$false}|ConvertTo-Json
