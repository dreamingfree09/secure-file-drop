using System;
using System.Linq;
using System.Text;
using System.Reflection;
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
            Console.WriteLine("PRODUCT POLICY CHECKS PASSED: 3");
            return 0;
        }
        catch (Exception) { Console.Error.WriteLine("PRODUCT POLICY FAIL"); return 1; }
    }
}
