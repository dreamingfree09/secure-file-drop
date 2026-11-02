param([Parameter(Mandatory)][string]$IdentityPath,[ValidateSet('Audit','Network','TLS')][string]$Phase='Audit')
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
$id=Get-Content -LiteralPath $IdentityPath -Raw|ConvertFrom-Json
$uuid=(Get-CimInstance Win32_ComputerSystemProduct).UUID.ToLowerInvariant().Trim('{}')
if($id.schema-ne'SFD_STAGE1_GUEST_V1'-or$id.role-ne'windows'-or$id.work_id-ne'SFD-STAGE1-INSTALLATION-20261102'-or$id.bios_uuid.ToLowerInvariant().Trim('{}')-ne$uuid-or(Test-Path 'H:\')){throw 'STOP:not-the-new-Windows-guest'}
$disks=@(Get-Disk|Where-Object{$_.BusType-ne'USB'});if(@($disks|Where-Object{$_.IsBoot}).Count-ne1){throw 'STOP:guest-disk'}
$nic=@(Get-NetAdapter|Where-Object{$_.Status-eq'Up'-and$_.HardwareInterface});if($nic.Count-ne1-or($nic[0].MacAddress-replace'-','')-ne'00155DA1B702'){throw 'STOP:guest-NIC'}
if($Phase-eq'Network'){
 # Mutates only the positively identified new guest. No Windows host firewall,
 # certificate store, default switch or original network is addressed.
 $ts='HKLM:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services';New-Item -Path $ts -Force|Out-Null;foreach($name in @('fDisableClip','fDisableCdm','fDisableCpm')){New-ItemProperty -Path $ts -Name $name -Value 1 -PropertyType DWord -Force|Out-Null}
 $index=$nic[0].ifIndex
 Set-NetIPInterface -InterfaceIndex $index -AddressFamily IPv4 -Dhcp Disabled
 if(@(Get-NetIPAddress -InterfaceIndex $index -AddressFamily IPv4|Where-Object{$_.IPAddress-eq'172.31.250.2'}).Count-eq0){New-NetIPAddress -InterfaceIndex $index -IPAddress 172.31.250.2 -PrefixLength 30|Out-Null}
 if(@(Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue).Count-ne0-or@(Get-NetRoute -DestinationPrefix '::/0' -ErrorAction SilentlyContinue).Count-ne0){throw 'STOP:unexpected-default-route'}
 Set-DnsClientServerAddress -InterfaceIndex $index -ServerAddresses @()
 # Guest-only TCP forwarding retains the accepted localhost certificate profile.
 # No SSH package, Windows-host listener or TLS interception is needed.
 foreach($port in @(8443,18082)){
  if(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue){throw 'STOP:guest-loopback-port-busy'}
  & "$env:WINDIR\System32\netsh.exe" interface portproxy add v4tov4 listenaddress=127.0.0.1 "listenport=$port" connectaddress=172.31.250.1 "connectport=$port"
  if($LASTEXITCODE-ne0){throw 'STOP:guest-portproxy'}
 }
}
if($Phase-eq'TLS'){
 # This code never imports a root. The named guest SID/public root approval is
 # a separate gate. CurrentUser chain success is checked with OS revocation.
 if($id.root_thumbprint-notmatch'^[A-Fa-f0-9]{40}$'-or$id.root_der_sha256-notmatch'^[a-f0-9]{64}$'){throw 'STOP:guest-root-approval-missing'}
 $root=Get-Item -LiteralPath ('Cert:\CurrentUser\Root\'+$id.root_thumbprint) -ErrorAction Stop
 $h=[Security.Cryptography.SHA256]::Create();try{$actual=([BitConverter]::ToString($h.ComputeHash($root.RawData))).Replace('-','').ToLowerInvariant()}finally{$h.Dispose()}
 if($actual-ne$id.root_der_sha256){throw 'STOP:guest-root-identity'}
 # Default Schannel curl and Framework4.8 handler: no skip/CRL-off callback.
 & "$env:WINDIR\System32\curl.exe" --fail --silent --show-error --max-time 10 https://localhost:8443/ready|Out-Null
 if($LASTEXITCODE-ne0){throw 'STOP:strict-Schannel'}
 $probe=Join-Path ([IO.Path]::GetDirectoryName($IdentityPath)) 'TlsProbe.exe'
 if(-not(Test-Path -LiteralPath $probe)){throw 'STOP:Framework48-probe-missing'}
 & $probe
 if($LASTEXITCODE-ne0){throw 'STOP:Framework48-ready'}
 # A cold-profile CDP hash and all negative identities are required by ledger;
 # a warm-cache success here alone is not revocation acceptance.
}
[ordered]@{schema='SFD_STAGE1_GUEST_RESULT_V1';role='windows';phase=$Phase;host_trust_changed=$false;actual_visual_acceptance=$false}|ConvertTo-Json
