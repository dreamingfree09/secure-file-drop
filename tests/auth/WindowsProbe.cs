using System;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Reflection;
using System.Runtime.Serialization;
using System.Runtime.Serialization.Json;
using System.Runtime.Versioning;
using System.Security.Cryptography;
using System.Threading;
using SecureFileDrop.Desktop;
[assembly: TargetFramework(".NETFramework,Version=v4.8", FrameworkDisplayName=".NET Framework 4.8")]
[DataContract] public sealed class Fixture {
 [DataMember] public string Origin;
 [DataMember] public string Username;
 [DataMember] public string Password;
}
static class WindowsProbe {
 static int passed;
 static void Check(bool b,string name){if(!b)throw new Exception(name);passed++;Console.WriteLine("REAL BACKEND PASS: "+name);}
 static int Main(string[] args){
  try {
   if(args.Length!=1)throw new Exception("Fixture input required");
   Fixture fixture;using(var f=File.OpenRead(args[0]))fixture=(Fixture)new DataContractJsonSerializer(typeof(Fixture)).ReadObject(f);
   Uri uri=new Uri(fixture.Origin);if(uri.Scheme!="https"||uri.Host!="localhost"||uri.Port==8443)throw new Exception("Only isolated local HTTPS fixture allowed");
   using(var sha=SHA256.Create())using(var file=File.OpenRead(typeof(SfdClient).Assembly.Location))Check(BitConverter.ToString(sha.ComputeHash(file)).Replace("-","").ToLowerInvariant()=="f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca","reviewed distributed executable bytes unchanged");
   Check(ServicePointManager.SecurityProtocol==SecurityProtocolType.SystemDefault,"OS-selected normal Framework TLS");
   var method=typeof(SfdClient).GetMethod("CreateHandler",BindingFlags.Static|BindingFlags.NonPublic);
   using(var handler=(HttpClientHandler)method.Invoke(null,new object[]{new CookieContainer()}))Check(handler.CheckCertificateRevocationList&&!handler.AllowAutoRedirect,"revocation enabled and redirects disabled");
   using(var client=new SfdClient(new Settings{Backend=fixture.Origin,RememberSession=false},null)){
    Check(client.Probe(CancellationToken.None).GetAwaiter().GetResult(),"service readiness over normal trusted HTTPS");
    client.Login(fixture.Username,fixture.Password,CancellationToken.None).GetAwaiter().GetResult();Check(client.Authenticated,"environment administrator login and strict identity verification");
    client.Restore(CancellationToken.None).GetAwaiter().GetResult();Check(client.Authenticated&&client.Probe(CancellationToken.None).GetAwaiter().GetResult(),"identity readback and authenticated readiness");
    client.Logout(CancellationToken.None).GetAwaiter().GetResult();Check(!client.Authenticated,"logout clears client session");
    bool rejected=false;try{client.Restore(CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault e){rejected=e.Status==401;}Check(rejected&&!client.Authenticated,"logged-out identity rejected with HTTP 401");
    rejected=false;try{client.Login(fixture.Username,"invalid-synthetic-only",CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault e){rejected=e.Status==401;}Check(rejected&&!client.Authenticated,"invalid credentials produce HTTP authentication error");
   }
   Console.WriteLine("REAL BACKEND WINDOWS CHECKS PASSED: "+passed);return 0;
  } catch(Exception e){Console.Error.WriteLine("REAL BACKEND WINDOWS FAIL: "+e.GetType().Name);return 1;}
 }
}