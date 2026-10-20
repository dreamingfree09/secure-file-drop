using System;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Reflection;
using System.Runtime.Serialization;
using System.Runtime.Serialization.Json;
using System.Runtime.Versioning;
using System.Security.Cryptography;
using System.Text;
using System.Threading;
using System.Windows.Forms;
using SecureFileDrop.Desktop;
[assembly: TargetFramework(".NETFramework,Version=v4.8", FrameworkDisplayName=".NET Framework 4.8")]
[DataContract] public sealed class ReleaseFixture {
 [DataMember] public string Origin;
 [DataMember] public string FixtureOwner;
 [DataMember] public string Username;
 [DataMember] public string Password;
 [DataMember] public string Uploader;
 [DataMember] public string UploaderPassword;
 [DataMember] public string Other;
 [DataMember] public string OtherPassword;
 [DataMember] public string UntrustedOrigin;
 [DataMember] public string ControlDirectory;
 [DataMember] public bool VerifyLinkExpiry;
}
static class ReleaseBackendProbe {
 static int passed; static string stage="guard";
 static void Check(bool value,string name){stage=name;if(!value)throw new Exception("assertion");passed++;Console.WriteLine("RELEASE BACKEND PASS: "+name);}
 static string Hash(byte[] data){using(var sha=SHA256.Create())return BitConverter.ToString(sha.ComputeHash(data)).Replace("-","").ToLowerInvariant();}
 static HttpClient NewHttp(CookieContainer cookies){return new HttpClient(new HttpClientHandler{CookieContainer=cookies,CheckCertificateRevocationList=true,AllowAutoRedirect=false}){Timeout=TimeSpan.FromSeconds(20)};}
 static SingleInstance Instance(string name){return (SingleInstance)typeof(SingleInstance).GetConstructor(BindingFlags.Instance|BindingFlags.NonPublic,null,new[]{typeof(string)},null).Invoke(new object[]{name});}
 [STAThread] static int Main(string[] args){
  string root=null,owner=Guid.NewGuid().ToString("N");
  try{
   if(args.Length!=1)throw new Exception("one owned fixture input required");
   ReleaseFixture f;using(var input=File.OpenRead(args[0]))f=(ReleaseFixture)new DataContractJsonSerializer(typeof(ReleaseFixture)).ReadObject(input);
   Uri origin=new Uri(f.Origin);
   if(origin.Scheme!="https"||origin.Host!="localhost"||origin.Port==8443||String.IsNullOrEmpty(f.FixtureOwner)||!System.Text.RegularExpressions.Regex.IsMatch(f.FixtureOwner,"^sfd-(auth-[0-9a-f]{12}|quality-[0-9a-f]{16})$"))throw new Exception("owned isolated HTTPS target required");
   Check(Hash(File.ReadAllBytes(typeof(SfdClient).Assembly.Location))=="f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca","exact reviewed executable bytes");
   var target=(TargetFrameworkAttribute)Attribute.GetCustomAttribute(typeof(SfdClient).Assembly,typeof(TargetFrameworkAttribute)); Check(target!=null&&target.FrameworkName==".NETFramework,Version=v4.8"&&ServicePointManager.SecurityProtocol==SecurityProtocolType.SystemDefault,"actual release Framework 4.8 target uses OS TLS policy");
   var factory=typeof(SfdClient).GetMethod("CreateHandler",BindingFlags.NonPublic|BindingFlags.Static);
   using(var handler=(HttpClientHandler)factory.Invoke(null,new object[]{new CookieContainer()}))Check(handler.CheckCertificateRevocationList&&!handler.AllowAutoRedirect,"release certificate revocation and redirect policy");
   root=Path.Combine(Path.GetTempPath(),"sfd-release-e2e-"+owner);if(Directory.Exists(root))throw new Exception("namespace exists; preserved");Directory.CreateDirectory(root);File.WriteAllText(Path.Combine(root,"owner"),owner);
   using(var admin=new SfdClient(new Settings{Backend=f.Origin,RememberSession=false},null)){
    stage="environment administrator authentication";Check(admin.Probe(CancellationToken.None).GetAwaiter().GetResult(),"normal trusted HTTPS readiness");
    admin.Login(f.Username,f.Password,CancellationToken.None).GetAwaiter().GetResult();admin.Restore(CancellationToken.None).GetAwaiter().GetResult();Check(admin.Authenticated,"environment administrator login and verified identity");
    admin.Logout(CancellationToken.None).GetAwaiter().GetResult();Check(!admin.Authenticated,"administrator logout");
   }
   var settings=new Settings{Backend=f.Origin,RememberSession=true,MaxUploadBytes=1024};var sessions=new ProtectedSessions(root);Cookie replay=null;Link expiryLink=null;
   using(var client=new SfdClient(settings,sessions)){
    stage="registered authentication";client.Login(f.Uploader,f.UploaderPassword,CancellationToken.None).GetAwaiter().GetResult();Check(client.Authenticated,"registered login and identity readback");
    replay=sessions.Load(origin);Check(replay!=null&&replay.Secure&&replay.HttpOnly,"DPAPI remembered session retains secure cookie flags");
    string[] blobs=Directory.GetFiles(Path.Combine(root,"sessions"));Check(blobs.Length==1&&Encoding.UTF8.GetString(File.ReadAllBytes(blobs[0])).IndexOf(replay.Value,StringComparison.Ordinal)<0&&Encoding.UTF8.GetString(File.ReadAllBytes(blobs[0])).IndexOf(f.UploaderPassword,StringComparison.Ordinal)<0,"protected session file has no plaintext token or password");
    Check(sessions.Load(new Uri("https://localhost:"+(origin.Port==65535?65534:origin.Port+1)))==null,"remembered sessions isolate exact origins");
    byte[][] data={Encoding.UTF8.GetBytes("synthetic Windows end-to-end data\n"),new byte[0],Enumerable.Range(0,1024).Select(i=>(byte)(i%251)).ToArray()};string[] names={"Δοκιμή 世界 with spaces & plus+.txt","empty space Δ.txt","exact boundary.bin"};
    for(int i=0;i<data.Length;i++){
     stage="actual upload "+i;string file=Path.Combine(root,names[i]);File.WriteAllBytes(file,data[i]);long progress=-1;
     var receipt=client.Upload(file,(done,total)=>Interlocked.Exchange(ref progress,done),CancellationToken.None).GetAwaiter().GetResult();
     Check(receipt.Confirmed&&receipt.Size==data[i].Length&&receipt.Hash==Hash(data[i]),"actual release multipart/native integrity "+i);
     Check(data[i].Length==0 ? Interlocked.Read(ref progress)==-1 : Interlocked.Read(ref progress)==data[i].Length,"actual byte progress (empty files have no byte chunks) "+i);
     var link=client.CreateLink(CancellationToken.None).GetAwaiter().GetResult();if(i==0)expiryLink=link;Check(new Uri(link.Url).GetLeftPart(UriPartial.Authority)==origin.GetLeftPart(UriPartial.Authority),"verified signed HTTPS link "+i);
     using(var http=NewHttp(new CookieContainer())){byte[] got=http.GetByteArrayAsync(link.Url).GetAwaiter().GetResult();Check(got.SequenceEqual(data[i]),"actual byte-equal bearer download "+i);}
    }
    using(var cancel=new CancellationTokenSource()){
     stage="actual in-flight request cancellation";bool rejected=false;
     try{client.Upload(Path.Combine(root,names[2]),(done,total)=>cancel.Cancel(),cancel.Token).GetAwaiter().GetResult();}catch(OperationCanceledException){rejected=true;}
     Check(rejected&&client.LastAttempt!=null&&!client.LastAttempt.Confirmed,"actual in-flight cancellation leaves no confirmed transfer");
     bool blocked=false;try{client.CreateLink(CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault){blocked=true;}Check(blocked,"cancelled transfer cannot create a link without reconciliation");
    }
    if(!String.IsNullOrEmpty(f.ControlDirectory)){
     if(!Directory.Exists(f.ControlDirectory)||File.ReadAllText(Path.Combine(f.ControlDirectory,"owner"))!=f.FixtureOwner)throw new Exception("network control owner mismatch");
     stage="actual backend interruption during request serialization";bool interrupted=false,signalled=false;
     try{client.Upload(Path.Combine(root,names[2]),(done,total)=>{if(signalled)return;signalled=true;using(File.Create(Path.Combine(f.ControlDirectory,"interrupt-backend.request"))){}DateTime until=DateTime.UtcNow.AddSeconds(30);while(!File.Exists(Path.Combine(f.ControlDirectory,"backend-stopped"))&&DateTime.UtcNow<until)Thread.Sleep(50);if(!File.Exists(Path.Combine(f.ControlDirectory,"backend-stopped")))throw new Exception("owned backend stop acknowledgement missing");},CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault){interrupted=true;}
     Check(signalled&&interrupted&&client.LastAttempt!=null&&!client.LastAttempt.Confirmed,"real interrupted request is not reported as a confirmed upload");
     bool blocked=false;try{client.CreateLink(CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault){blocked=true;}Check(blocked,"interrupted transfer cannot create an unverified link");
     Check(!client.Probe(CancellationToken.None).GetAwaiter().GetResult(),"actual unavailable backend readiness is false");
     using(File.Create(Path.Combine(f.ControlDirectory,"restore-backend.request"))){}DateTime deadline=DateTime.UtcNow.AddSeconds(45);while(!File.Exists(Path.Combine(f.ControlDirectory,"backend-restored"))&&DateTime.UtcNow<deadline)Thread.Sleep(50);
     Check(File.Exists(Path.Combine(f.ControlDirectory,"backend-restored")),"only owned synthetic backend restarted with rechecked loopback binding");
     client.Restore(CancellationToken.None).GetAwaiter().GetResult();Check(client.Authenticated&&client.Probe(CancellationToken.None).GetAwaiter().GetResult(),"real backend restart retains valid session and readiness");
    }
    using(var cancelled=new CancellationTokenSource()){cancelled.Cancel();bool rejected=false;try{client.Upload(Path.Combine(root,names[0]),null,cancelled.Token).GetAwaiter().GetResult();}catch(OperationCanceledException){rejected=true;}Check(rejected,"pre-cancelled real transfer rejected");}
   }
   using(var restored=new SfdClient(settings,sessions)){
    restored.Restore(CancellationToken.None).GetAwaiter().GetResult();Check(restored.Authenticated,"new client restores and verifies remembered DPAPI session");
    restored.Logout(CancellationToken.None).GetAwaiter().GetResult();Check(sessions.Load(origin)==null,"logout removes only owned protected session");
   }
   var replayJar=new CookieContainer();replayJar.Add(origin,replay);using(var http=NewHttp(replayJar))Check(http.GetAsync(new Uri(origin,"me")).GetAwaiter().GetResult().StatusCode==HttpStatusCode.Unauthorized,"logged-out synthetic session replay rejected");
   using(var client=new SfdClient(settings,sessions)){
    client.Login(f.Uploader,f.UploaderPassword,CancellationToken.None).GetAwaiter().GetResult();client.Upload(Path.Combine(root,"empty space Δ.txt"),null,CancellationToken.None).GetAwaiter().GetResult();
    client.Login(f.Other,f.OtherPassword,CancellationToken.None).GetAwaiter().GetResult();Check(client.Authenticated&&client.LastAttempt==null,"account switch clears previous transfer/link identity");
    bool blocked=false;try{client.CreateLink(CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault){blocked=true;}Check(blocked,"previous-account link cannot be reused after switching");
    client.Logout(CancellationToken.None).GetAwaiter().GetResult();
   }
   if(!String.IsNullOrEmpty(f.ControlDirectory)){
    if(!Directory.Exists(f.ControlDirectory)||File.ReadAllText(Path.Combine(f.ControlDirectory,"owner"))!=f.FixtureOwner)throw new Exception("expiry control owner mismatch");
    using(var expiring=new SfdClient(settings,sessions)){
     stage="real database session expiry";expiring.Login(f.Uploader,f.UploaderPassword,CancellationToken.None).GetAwaiter().GetResult();var cookie=sessions.Load(origin);if(cookie==null)throw new Exception("expiry fixture session missing");
     using(var request=new FileStream(Path.Combine(f.ControlDirectory,"expire-session.request"),FileMode.CreateNew,FileAccess.Write,FileShare.None)){byte[] value=Encoding.ASCII.GetBytes(Hash(Encoding.UTF8.GetBytes(cookie.Value)));request.Write(value,0,value.Length);}
     DateTime deadline=DateTime.UtcNow.AddSeconds(30);while(!File.Exists(Path.Combine(f.ControlDirectory,"expiry-applied"))&&DateTime.UtcNow<deadline)Thread.Sleep(50);
     Check(File.Exists(Path.Combine(f.ControlDirectory,"expiry-applied")),"owned synthetic PostgreSQL session expiry applied");
     bool rejected=false;try{expiring.Restore(CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault error){rejected=error.Status==401;}
     Check(rejected&&!expiring.Authenticated&&sessions.Load(origin)==null,"actual expired-server session401 clears authentication and DPAPI state");
    }
   }
   using(var invalid=new SfdClient(settings,sessions)){bool rejected=false;try{invalid.Login(f.Uploader,"incorrect-synthetic-only",CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault error){rejected=error.Status==401;}Check(rejected&&!invalid.Authenticated,"actual invalid credentials rejected with401");}
   string oversized=Path.Combine(root,"too large.bin");File.WriteAllBytes(oversized,new byte[1025]);
   using(var limited=new SfdClient(settings,null)){bool blocked=false;try{limited.Upload(oversized,null,CancellationToken.None).GetAwaiter().GetResult();}catch(ClientFault){blocked=limited.LastAttempt==null;}Check(blocked,"companion size limit rejects before metadata");}
   var mismatch=new UriBuilder(origin){Host="127.0.0.1"};using(var invalid=new SfdClient(new Settings{Backend=mismatch.Uri.GetLeftPart(UriPartial.Authority)},null))Check(!invalid.Probe(CancellationToken.None).GetAwaiter().GetResult(),"localhost-only certificate rejects IP hostname mismatch");
   if(!String.IsNullOrEmpty(f.UntrustedOrigin))using(var invalid=new SfdClient(new Settings{Backend=f.UntrustedOrigin},null))Check(!invalid.Probe(CancellationToken.None).GetAwaiter().GetResult(),"untrusted certificate rejected before any authentication");
   if(f.VerifyLinkExpiry){
    stage="actual five-minute signed link expiry";if(expiryLink==null)throw new Exception("owned link missing");
    DateTime until=DateTimeOffset.Parse(expiryLink.ExpiresAt).UtcDateTime.AddSeconds(1);var wait=System.Diagnostics.Stopwatch.StartNew();
    if(until-DateTime.UtcNow>TimeSpan.FromMinutes(6))throw new Exception("unbounded expiry fixture");
    Console.WriteLine("RELEASE EXPIRY WAIT: owned five-minute link; no credentials or URL printed");
    while(DateTime.UtcNow<until){if(wait.Elapsed>TimeSpan.FromSeconds(370))throw new Exception("expiry wait exceeded budget");Thread.Sleep(100);}
    using(var http=NewHttp(new CookieContainer()))Check(http.GetAsync(expiryLink.Url).GetAwaiter().GetResult().StatusCode==HttpStatusCode.Unauthorized,"actual Windows-generated five-minute link expires with401");
   }
   Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
   string ui=Path.Combine(root,"ui");Directory.CreateDirectory(ui);using(var file=File.Create(Path.Combine(ui,"settings.json")))new DataContractJsonSerializer(typeof(Settings)).WriteObject(file,new Settings{Backend=f.Origin,RememberSession=false});
   using(var instance=Instance("-release-e2e-"+owner))using(var context=new TrayContext(ui,instance,"")){
    var icon=(NotifyIcon)typeof(TrayContext).GetField("tray",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(context);
    Check(icon.Visible&&icon.ContextMenuStrip.Items.OfType<ToolStripMenuItem>().Any(x=>x.Text=="Quick Send…"),"actual release tray context uses owned settings/profile and menu");
    int ticks=0;bool ready=false;var timer=new System.Windows.Forms.Timer{Interval=100};timer.Tick+=delegate{ticks++;ready=icon.ContextMenuStrip.Items.OfType<ToolStripMenuItem>().Any(x=>x.Text=="Service: ready");if(ready||ticks>100){timer.Stop();icon.ContextMenuStrip.Items.OfType<ToolStripMenuItem>().Single(x=>x.Text=="Exit").PerformClick();}};
    timer.Start();Application.Run(context);timer.Dispose();Check(ready&&!icon.Visible,"actual release readiness and clean tray exit");
   }
   f.Password=f.UploaderPassword=f.OtherPassword=null;
   Console.WriteLine("RELEASE REAL BACKEND CHECKS PASSED: "+passed);Console.WriteLine("NOT EXECUTED: Owner visual acceptance, native file-picker interaction, real Explorer registration/invocation and OS clipboard.");return 0;
  }catch(Exception error){Console.Error.WriteLine("RELEASE BACKEND FAIL: "+stage+" / "+error.GetType().Name+" / status="+(error is ClientFault ? ((ClientFault)error).Status : 0)+" / reconciliation="+(error.Message=="The recent-file list does not confirm completion. Inspect the web interface; pending or failed records use normal server retention. Nothing was deleted or resent."));return 1;}
  finally{if(root!=null&&Path.GetFullPath(root).StartsWith(Path.GetFullPath(Path.GetTempPath()),StringComparison.OrdinalIgnoreCase)&&File.Exists(Path.Combine(root,"owner"))&&File.ReadAllText(Path.Combine(root,"owner"))==owner)Directory.Delete(root,true);}
 }
}