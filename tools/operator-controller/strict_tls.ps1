param([Parameter(Mandatory=$true)][string]$ReportPath)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Net.Http
if($null -ne [Net.ServicePointManager]::ServerCertificateValidationCallback -or [int][Net.ServicePointManager]::SecurityProtocol -ne 0){throw 'Unexpected TLS policy'}
if((Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full').Release -lt 528040){throw 'Framework prerequisite'}
function HashBytes([byte[]]$bytes){
 $h=[Security.Cryptography.SHA256]::Create()
 try{return [BitConverter]::ToString($h.ComputeHash($bytes)).Replace('-','').ToLowerInvariant()}finally{$h.Dispose()}
}
$root='0f45e72366dc8ff6e3bed9323ef697bb44b21204d7e05ea303da319b5160bc28'
$trusted=$false
foreach($location in @('CurrentUser','LocalMachine')){
 $s=[Security.Cryptography.X509Certificates.X509Store]::new('Root',$location)
 try{
  $s.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
  foreach($c in $s.Certificates.Find([Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,'A67604D69C6BFC32E77CE2933E0096C14462367C',$false)){
   if((HashBytes $c.RawData) -ceq $root){$trusted=$true}
  }
 }finally{$s.Close()}
}
if(-not $trusted){throw 'Existing trusted root missing'}
$handler=[Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect=$false;$handler.CheckCertificateRevocationList=$true
$http=[Net.Http.HttpClient]::new($handler);$http.Timeout=[TimeSpan]::FromSeconds(10)
try{
 foreach($route in @('health','ready')){
  $status=& "$env:SystemRoot\System32\curl.exe" --disable --silent --show-error --max-time 10 --output NUL --write-out '%{http_code}' "https://localhost:8443/$route" 2>&1
  if($LASTEXITCODE -ne 0 -or (($status -join '').Trim()) -ne '200'){throw 'Schannel HTTPS failed'}
  $r=$http.GetAsync("https://localhost:8443/$route").GetAwaiter().GetResult()
  try{if([int]$r.StatusCode -ne 200){throw 'Revocation-enabled Framework HTTPS failed'}}finally{$r.Dispose()}
 }
}finally{$http.Dispose()}
$web=[Net.WebClient]::new()
try{$hash=HashBytes ($web.DownloadData('http://127.0.0.1:18081/sfd-local-ca.crl'))}finally{$web.Dispose()}
if($hash -cne '69bbe78994b7e6abe9849e65fd8a56fe59dbe0803e6db498f3e8c82ad70f6f1e'){throw 'Published CRL mismatch'}
# Get the served leaf using ordinary SslStream validation and revocation checks.
$tcp=[Net.Sockets.TcpClient]::new()
try{
 $tcp.Connect('localhost',8443)
 $ssl=[Net.Security.SslStream]::new($tcp.GetStream(),$false)
 try{
  $ssl.AuthenticateAsClient('localhost',[Security.Cryptography.X509Certificates.X509CertificateCollection]::new(),[Security.Authentication.SslProtocols]::None,$true)
  if((HashBytes $ssl.RemoteCertificate.GetRawCertData()) -cne '4106ecd1f0bd2d94be7a7fcf2236cb9b0119dde5aa82a97edd2cdab584809b70'){throw 'Served leaf changed'}
 }finally{$ssl.Dispose()}
}finally{$tcp.Dispose()}
$json=@{success=$true;epoch=[DateTimeOffset]::UtcNow.ToUnixTimeSeconds();Schannel200=2;Framework200=2;revocation_enabled=$true;served_leaf_exact=$true;trust_modified=$false}|ConvertTo-Json
$f=[IO.File]::Open($ReportPath,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
try{$bytes=[Text.UTF8Encoding]::new($false).GetBytes($json);$f.Write($bytes,0,$bytes.Length);$f.Flush($true)}finally{$f.Dispose()}
