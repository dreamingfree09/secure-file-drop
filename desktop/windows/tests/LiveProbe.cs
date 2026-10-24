using System;
using System.Net;
using System.Net.Http;
using System.Reflection;
using System.Runtime.Versioning;
using System.Threading;
using SecureFileDrop.Desktop;
static class LiveProbe
{
    static int passed;
    static void Check(bool value, string name) { if (!value) throw new Exception(name); passed++; Console.WriteLine("LIVE PASS: " + name); }
    static int Main(string[] args)
    {
        try
        {
            if (args.Length != 1) throw new Exception("One authorised local HTTPS origin is required");
            var settings = new Settings { Backend = args[0] };
            Uri origin = settings.Origin();
            if (origin.Scheme != "https" || origin.Host != "localhost") throw new Exception("Local HTTPS acceptance only");
            var target = (TargetFrameworkAttribute)Attribute.GetCustomAttribute(Assembly.GetExecutingAssembly(), typeof(TargetFrameworkAttribute));
            Check(target != null && target.FrameworkName == ".NETFramework,Version=v4.8" && ServicePointManager.SecurityProtocol == SecurityProtocolType.SystemDefault, "native Framework target uses OS-selected TLS");
            using (var client = new SfdClient(settings))
            {
                Check(client.Probe(CancellationToken.None).GetAwaiter().GetResult(), "release client reaches existing HTTPS readiness with revocation enabled");
                bool unauthorized = false;
                try { client.Restore(CancellationToken.None).GetAwaiter().GetResult(); }
                catch (ClientFault fault) { unauthorized = fault.Status == 401; }
                Check(unauthorized && !client.Authenticated, "existing backend returns HTTP 401 for credential-free session check");
            }
            using (var handler = SfdClient.CreateHandler(new CookieContainer()))
            using (var http = new HttpClient(handler))
            {
                http.Timeout = TimeSpan.FromSeconds(10);
                using (var health = http.GetAsync(new Uri(origin,"health")).GetAwaiter().GetResult())
                    Check(handler.CheckCertificateRevocationList && health.StatusCode == HttpStatusCode.OK, "existing HTTPS health passes normal certificate/revocation validation");
            }
            var wrongName = new UriBuilder(origin) { Host = "127.0.0.1" };
            using (var mismatch = new SfdClient(new Settings { Backend = wrongName.Uri.GetLeftPart(UriPartial.Authority) }))
                Check(!mismatch.Probe(CancellationToken.None).GetAwaiter().GetResult(), "existing localhost-only certificate rejects IP-name mismatch");
            Console.WriteLine("LIVE CREDENTIAL-FREE CHECKS PASSED: " + passed);
            Console.WriteLine("NOT EXECUTED: Owner password entry, authenticated live session and visual Windows acceptance.");
            return 0;
        }
        catch (Exception error) { Console.Error.WriteLine("LIVE FAIL: " + error.GetType().Name); return 1; }
    }
}