using System;
using System.Net;
using System.Net.Http;
using System.Runtime.Versioning;
[assembly:TargetFramework(".NETFramework,Version=v4.8",FrameworkDisplayName=".NET Framework 4.8")]
public static class TlsProbe {
 public static int Main() {
  try {
   if(ServicePointManager.ServerCertificateValidationCallback!=null) return 2;
   using(var h=new HttpClientHandler{CheckCertificateRevocationList=true,UseProxy=false,AllowAutoRedirect=false})
   using(var c=new HttpClient(h)) {
    c.Timeout=TimeSpan.FromSeconds(10);var r=c.GetAsync("https://localhost:8443/ready").GetAwaiter().GetResult();
    if(r.StatusCode!=HttpStatusCode.OK) return 2;
    Console.WriteLine("{\"Framework48_ready\":true,\"revocation_enabled\":true}");return 0;
   }
  } catch { Console.Error.WriteLine("STOP:Framework48-strict-HTTPS");return 2; }
 }
}
