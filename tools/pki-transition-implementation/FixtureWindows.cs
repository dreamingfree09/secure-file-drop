using System;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Net.Security;
using System.Net.Sockets;
using System.Security.Authentication;
using System.Security.Cryptography;
using System.Security.AccessControl;
using System.Security.Cryptography.X509Certificates;
using System.Security.Principal;
using System.Runtime.Versioning;
using System.Web.Script.Serialization;
using System.Windows.Forms;
[assembly: TargetFramework(".NETFramework,Version=v4.8", FrameworkDisplayName=".NET Framework 4.8")]
namespace SfdPkiPreparation {
    public static class Program {
        static void Require(bool value) { if(!value) throw new InvalidOperationException("FIXTURE_GATE"); }
        static string Fixture(string path) {
            string full=Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar);
            string parent=Path.GetDirectoryName(full).TrimEnd(Path.DirectorySeparatorChar);
            Require(String.Equals(parent,Path.GetTempPath().TrimEnd(Path.DirectorySeparatorChar),StringComparison.OrdinalIgnoreCase));
            string name=Path.GetFileName(full); Guid nonce=Guid.Empty;
            Require(name.StartsWith("sfd-pki-fixtures-",StringComparison.Ordinal) && Guid.TryParse(name.Substring(17),out nonce));
            Require((File.GetAttributes(full)&FileAttributes.ReparsePoint)==0);
            var meta=new JavaScriptSerializer().DeserializeObject(File.ReadAllText(Path.Combine(full,"fixture.json"))) as System.Collections.Generic.Dictionary<string,object>;
            Require(meta!=null && meta.Count==3 && (string)meta["schema"]=="SFD_DISPOSABLE_PKI_FIXTURE_V1" &&
                    (string)meta["nonce"]==nonce.ToString() && (bool)meta["synthetic_only"]);
            return full;
        }
        static bool AuthenticationFailure(Exception e) {
            while(e!=null) {
                if(e is AuthenticationException) return true;
                var w=e as WebException;
                if(w!=null && (w.Status==WebExceptionStatus.TrustFailure || w.Status==WebExceptionStatus.SecureChannelFailure)) return true;
                e=e.InnerException;
            }
            return false;
        }
        static object StrictReject(string uriText,string rootPath) {
            Uri uri=new Uri(uriText);
            Require(uri.Scheme=="https" && (uri.Host=="localhost" || uri.Host=="127.0.0.1") &&
                !new[]{8443,18081,18082,18443}.Contains(uri.Port) && uri.Port>0 && uri.UserInfo=="" && uri.Query=="");
            Fixture(Path.GetDirectoryName(rootPath));
            var root=new X509Certificate2(File.ReadAllBytes(rootPath));
            Require(root.Subject.StartsWith("CN=SyntheticSFDFixture-",StringComparison.Ordinal) && !root.HasPrivateKey);
            bool untrusted=false;
            using(var chain=new X509Chain()) {
                chain.ChainPolicy.RevocationMode=X509RevocationMode.Online;
                chain.ChainPolicy.VerificationFlags=X509VerificationFlags.NoFlag;
                chain.ChainPolicy.UrlRetrievalTimeout=TimeSpan.FromSeconds(3);
                bool ok=chain.Build(root);
                untrusted=!ok && chain.ChainStatus.Any(s=>(s.Status&X509ChainStatusFlags.UntrustedRoot)!=0);
            }
            Require(untrusted && ServicePointManager.ServerCertificateValidationCallback==null &&
                    ServicePointManager.SecurityProtocol==SecurityProtocolType.SystemDefault);
            bool http=false,stream=false;
            using(var handler=new HttpClientHandler { CheckCertificateRevocationList=true,AllowAutoRedirect=false }) {
                using(var client=new HttpClient(handler)) {
                    client.Timeout=TimeSpan.FromSeconds(10);
                    try { using(var response=client.GetAsync(uri).GetAwaiter().GetResult()){} }
                    catch(Exception e) { http=AuthenticationFailure(e); }
                }
            }
            using(var tcp=new TcpClient()) {
                tcp.ReceiveTimeout=5000;tcp.SendTimeout=5000;
                tcp.Connect(uri.Host,uri.Port);
                using(var ssl=new SslStream(tcp.GetStream(),false)) {
                    try { ssl.AuthenticateAsClient(uri.Host,null,SslProtocols.None,true); }
                    catch(AuthenticationException) { stream=true; }
                }
            }
            Require(http && stream && ServicePointManager.ServerCertificateValidationCallback==null);
            return new {target=".NET Framework 4.8",RootUntrusted=untrusted,HttpClientRejected=http,
                SslStreamRejected=stream,RevocationEnabled=true,CustomCallbacks=false,
                PositiveTrustedWindowsTLSExecuted=false,TrustWrites=0};
        }
        static object SecureFixtureDirectory(string path) {
            path=Fixture(path);
            // ACL only the freshly marked disposable directory; no inherited user/system trust settings.
            var user=WindowsIdentity.GetCurrent().User;
            var security=new DirectorySecurity();
            security.SetOwner(user);security.SetAccessRuleProtection(true,false);
            foreach(var sid in new[]{user,new SecurityIdentifier(WellKnownSidType.LocalSystemSid,null)}) {
                security.AddAccessRule(new FileSystemAccessRule(sid,FileSystemRights.FullControl,
                    InheritanceFlags.ContainerInherit|InheritanceFlags.ObjectInherit,PropagationFlags.None,AccessControlType.Allow));
            }
            Directory.SetAccessControl(path,security);
            var actual=Directory.GetAccessControl(path);
            Require(actual.AreAccessRulesProtected && actual.GetOwner(typeof(SecurityIdentifier)).Equals(user));
            var rules=actual.GetAccessRules(true,true,typeof(SecurityIdentifier));
            Require(rules.Count==2);
            foreach(FileSystemAccessRule rule in rules) {
                Require(!rule.IsInherited && rule.AccessControlType==AccessControlType.Allow &&
                    (rule.IdentityReference.Equals(user) || rule.IdentityReference.Equals(new SecurityIdentifier(WellKnownSidType.LocalSystemSid,null))));
            }
            return new {FreshFixtureAclVerified=true,RealUserFilesChanged=false};
        }
        public static Form Preview(string scope,string publicIdentity) {
            Require(new[]{"create_issuer","install_current_user_root","select_endpoint","retire_old_root"}.Contains(scope));
            Require(publicIdentity.Length<=8192 && !publicIdentity.Contains("\0"));
            var form=new Form {Text="SFD transition approval — PREPARATION PREVIEW ONLY",Width=760,Height=560};
            var text=new TextBox {Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,
                Text="No action or approval is enabled in this preparation build.\r\nScope: "+scope+
                "\r\nCreation, CurrentUser trust, endpoint selection and old-root retirement are separate future approvals.\r\n"+
                "CurrentUser/Root trust affects other applications in the Owner account. No LocalMachine change.\r\n"+
                publicIdentity};
            var secret=new TextBox {UseSystemPasswordChar=true,Enabled=false,Dock=DockStyle.Bottom,
                AccessibleDescription="Future local-only secret field. Disabled; no secret is accepted by this preview."};
            var close=new Button {Text="Close preview — no action",Dock=DockStyle.Bottom};
            close.Click+=(s,e)=>form.Close();
            form.Controls.Add(text);form.Controls.Add(secret);form.Controls.Add(close);
            return form;
        }
        static object StoreSnapshot() {
            var result=new System.Collections.Generic.List<object>();
            foreach(var loc in new[]{StoreLocation.CurrentUser,StoreLocation.LocalMachine})
                foreach(var name in new[]{"Root","My","CA"})
                    using(var store=new X509Store(name,loc)) {
                        store.Open(OpenFlags.ReadOnly|OpenFlags.OpenExistingOnly);
                        var values=store.Certificates.Cast<X509Certificate2>().Select(x=>Convert.ToBase64String(x.RawData))
                            .OrderBy(x=>x,StringComparer.Ordinal).ToArray();
                        using(var hash=SHA256.Create()) {
                            string digest=BitConverter.ToString(hash.ComputeHash(System.Text.Encoding.UTF8.GetBytes(String.Join("\n",values)))).Replace("-","").ToLowerInvariant();
                            result.Add(new{Location=loc.ToString(),Store=name,Count=values.Length,PublicSetSha256=digest});
                        }
                    }
            return result;
        }
        static object SelfTest() {
            foreach(var scope in new[]{"create_issuer","install_current_user_root","select_endpoint","retire_old_root"})
                using(var form=Preview(scope,"SYNTHETIC PUBLIC IDENTITY; not an approval")) {
                    Require(form.Controls.OfType<TextBox>().Any(x=>x.UseSystemPasswordChar && !x.Enabled));
                    Require(form.Controls.OfType<TextBox>().Any(x=>x.ReadOnly && x.Text.Contains("other applications")));
                }
            bool rejected=false;try{using(var f=Preview("LocalMachine","fixture")){}}catch(InvalidOperationException){rejected=true;}
            Require(rejected);
            return new {FourPreviewScopes=true,MaskedInputDisabled=true,UnknownScopeRejected=true,RealActionsImplemented=false};
        }
        [STAThread] public static int Main(string[] args) {
            try {
                object result;
                if(args.Length==1 && args[0]=="--self-test") result=SelfTest();
                else if(args.Length==1 && args[0]=="--store-snapshot") result=StoreSnapshot();
                else if(args.Length==2 && args[0]=="--secure-fixture-root") result=SecureFixtureDirectory(args[1]);
                else if(args.Length==3 && args[0]=="--strict-reject") result=StrictReject(args[1],args[2]);
                else throw new InvalidOperationException("NO_REAL_EXECUTION_ROUTE");
                Console.WriteLine(new JavaScriptSerializer().Serialize(result));return 0;
            } catch(Exception) { Console.Error.WriteLine("STOP: synthetic Windows gate failed; no secrets disclosed.");return 1; }
        }
    }
}
