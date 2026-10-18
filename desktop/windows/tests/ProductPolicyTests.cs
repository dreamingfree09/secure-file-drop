using System;
using System.Linq;
using System.Text;
using System.Reflection;
using System.Net;
using System.Net.Http;
using System.Runtime.Versioning;
using SecureFileDrop.Desktop;
static class ProductPolicyTests
{
    static int Main()
    {
        try
        {
            if (new Settings { Backend = "https://example.test:8443" }.Origin().Scheme != "https") throw new Exception("HTTPS policy");
            Console.WriteLine("PRODUCT POLICY PASS: valid HTTPS origin accepted");
            var forged = Json.Decode<Settings>(Encoding.UTF8.GetBytes("{\"Backend\":\"http://127.0.0.1:1234\",\"AllowLoopbackHttp\":true,\"AllowSyntheticLoopbackHttp\":true,\"MaxUploadBytes\":10485760}"));
            bool rejected = false;
            try { forged.Origin(); } catch (ClientFault) { rejected = true; }
            if (!rejected) throw new Exception("Persisted HTTP override");
            Console.WriteLine("PRODUCT POLICY PASS: configuration cannot enable HTTP");
            if (typeof(Settings).GetFields(BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Instance).Any(f => f.Name.Contains("LoopbackHttp"))) throw new Exception("Synthetic switch leaked into release");
            Console.WriteLine("PRODUCT POLICY PASS: synthetic HTTP capability absent from release client");
            var target = (TargetFrameworkAttribute)Attribute.GetCustomAttribute(Assembly.GetExecutingAssembly(), typeof(TargetFrameworkAttribute));
            if (target == null || target.FrameworkName != ".NETFramework,Version=v4.8" || ServicePointManager.SecurityProtocol != SecurityProtocolType.SystemDefault) throw new Exception("Framework TLS targeting");
            Console.WriteLine("PRODUCT POLICY PASS: compiled Framework 4.8 target selects OS TLS defaults");
            using (var handler = SfdClient.CreateHandler(new CookieContainer()))
                if (!handler.CheckCertificateRevocationList || handler.AllowAutoRedirect || !handler.UseCookies) throw new Exception("Transport protection");
            Console.WriteLine("PRODUCT POLICY PASS: revocation/cookies enabled and redirects disabled");
            string marker = "SYNTHETIC_CREDENTIAL_MARKER";
            var trust = SfdClient.TransportFault(new HttpRequestException(marker, new WebException(marker, WebExceptionStatus.TrustFailure)));
            var dns = SfdClient.TransportFault(new HttpRequestException(marker, new WebException(marker, WebExceptionStatus.NameResolutionFailure)));
            var channel = SfdClient.TransportFault(new HttpRequestException(marker, new WebException(marker, WebExceptionStatus.SecureChannelFailure)));
            var connect = SfdClient.TransportFault(new HttpRequestException(marker, new WebException(marker, WebExceptionStatus.ConnectFailure)));
            var unknown = SfdClient.TransportFault(new HttpRequestException(marker));
            if (!trust.Message.Contains("certificate") || !dns.Message.Contains("resolved") || !channel.Message.Contains("negotiate") || !connect.Message.Contains("Cannot connect") || !unknown.Message.Contains("before an HTTP response") || new [] { trust, dns, channel, connect, unknown }.Any(e => e.Message.Contains(marker))) throw new Exception("Safe transport classification");
            Console.WriteLine("PRODUCT POLICY PASS: transport classes are distinct and redact exception values");
            Console.WriteLine("PRODUCT POLICY CHECKS PASSED: 6");
            return 0;
        }
        catch (Exception) { Console.Error.WriteLine("PRODUCT POLICY FAIL"); return 1; }
    }
}
