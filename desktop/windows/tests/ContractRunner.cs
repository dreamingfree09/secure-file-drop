using System;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Runtime.Serialization;
using System.Security.Cryptography;
using System.Threading;
using System.Threading.Tasks;
using SecureFileDrop.Desktop;

namespace SecureFileDrop.Contract
{
    [DataContract] public sealed class Registration
    {
        [DataMember(Name = "email")] public string Email;
        [DataMember(Name = "username")] public string Username;
        [DataMember(Name = "password")] public string Password;
    }
    static class ContractRunner
    {
        static int passed;
        static void Check(bool value, string name) { if (!value) throw new Exception(name); passed++; Console.WriteLine("DESKTOP PASS: " + name); }
        static string Hash(byte[] bytes) { using (var sha = SHA256.Create()) return BitConverter.ToString(sha.ComputeHash(bytes)).Replace("-", "").ToLowerInvariant(); }
        static async Task Register(Uri origin, string username, string secret)
        {
            using (var http = new HttpClient(new HttpClientHandler { AllowAutoRedirect = false }))
            using (var body = new ByteArrayContent(Json.Encode(new Registration { Username = username, Email = username + "@example.test", Password = secret })))
            {
                http.Timeout = TimeSpan.FromSeconds(12); body.Headers.ContentType = new MediaTypeHeaderValue("application/json");
                using (var result = await http.PostAsync(new Uri(origin, "register"), body)) Check(result.StatusCode == HttpStatusCode.Created, "synthetic ordinary registration");
            }
        }
        static async Task Run(string[] args)
        {
            if (args.Length != 4 || !args[1].StartsWith("sfd-quality-", StringComparison.Ordinal) ||
                Environment.GetEnvironmentVariable("SFD_DESKTOP_RUN") != args[1]) throw new Exception("Owned synthetic run required.");
            Uri origin = new Uri(args[0]), untrusted = new Uri(args[2]), unavailable = new Uri(args[3]);
            if (origin.Scheme != "https" || origin.Host != "localhost" || !untrusted.IsLoopback || !unavailable.IsLoopback)
                throw new Exception("Owned loopback HTTPS fixtures required.");
            string suffix = Guid.NewGuid().ToString("N");
            string root = Path.Combine(Path.GetTempPath(), "sfd-desktop-contract-" + suffix);
            if (Directory.Exists(root)) throw new Exception("Synthetic file namespace already exists; preserved.");
            Directory.CreateDirectory(root); File.WriteAllText(Path.Combine(root, "owner"), suffix);
            try
            {
                string user = "desktop_" + suffix.Substring(0, 16), other = "desktop_" + suffix.Substring(16);
                byte[] random = new byte[16]; using (var rng = RandomNumberGenerator.Create()) rng.GetBytes(random);
                string secret = "Desktop1" + BitConverter.ToString(random).Replace("-", "");
                await Register(origin, user, secret); await Register(origin, other, secret);
                using (var client = new SfdClient(new Settings { Backend = origin.AbsoluteUri, MaxUploadBytes = 1024 }))
                {
                    Check(await client.Probe(CancellationToken.None), "real backend readiness through verified TLS");
                    await client.Login(user, secret, CancellationToken.None);
                    Check(client.Authenticated, "real authenticated session /me");
                    string file = Path.Combine(root, "Δοκιμή 世界 with spaces.txt");
                    byte[] expected = new byte[1024]; for (int i = 0; i < expected.Length; i++) expected[i] = (byte)(i % 251);
                    File.WriteAllBytes(file, expected);
                    long progress = -1;
                    var receipt = await client.Upload(file, delegate(long done, long total) { Interlocked.Exchange(ref progress, done); }, CancellationToken.None);
                    Check(receipt.Confirmed && receipt.Size == 1024 && receipt.Hash == Hash(expected), "real Unicode exact-limit upload and matching native server SHA-256");
                    Check(Interlocked.Read(ref progress) == 1024, "real HTTP upload byte progress");
                    Link link = await client.CreateLink(CancellationToken.None);
                    Check(new Uri(link.Url).GetLeftPart(UriPartial.Authority) == origin.GetLeftPart(UriPartial.Authority), "real five-minute canonical HTTPS signed link");
                    using (var download = new HttpClient())
                    {
                        download.Timeout = TimeSpan.FromSeconds(12);
                        byte[] bytes = await download.GetByteArrayAsync(link.Url);
                        Check(Hash(bytes) == receipt.Hash && bytes.Length == expected.Length, "actual bearer HTTPS download matches uploaded bytes");
                    }
                    string empty = Path.Combine(root, "empty space Δ.txt"); File.WriteAllBytes(empty, new byte[0]);
                    var zero = await client.Upload(empty, null, CancellationToken.None);
                    Check(zero.Confirmed && zero.Size == 0 && zero.Hash == Hash(new byte[0]), "real empty upload verified");
                    using (var cancelled = new CancellationTokenSource())
                    {
                        cancelled.Cancel(); bool stopped = false;
                        try { await client.Upload(file, null, cancelled.Token); } catch (OperationCanceledException) { stopped = true; }
                        Check(stopped, "pre-cancelled real request does not start a transfer");
                    }
                    await client.Upload(file, null, CancellationToken.None);
                    await client.Login(other, secret, CancellationToken.None);
                    Check(client.Authenticated && client.LastAttempt == null, "account change clears previous-account transfer/link state");
                    bool blocked = false; try { await client.CreateLink(CancellationToken.None); } catch (ClientFault) { blocked = true; }
                    Check(blocked, "previous-account link cannot be copied after switching account");
                    await client.Logout(CancellationToken.None);
                    bool unauthorized = false; try { await client.Restore(CancellationToken.None); } catch (ClientFault e) { unauthorized = e.Status == 401; }
                    Check(unauthorized && !client.Authenticated, "real 401 absent/expired/revoked-session response requires sign-in");
                    await client.Login(user, secret, CancellationToken.None);
                    var retry = await client.Upload(file, null, CancellationToken.None);
                    Check(retry.Confirmed, "clean retry after cancelled request/sign-out");
                }
                using (var limited = new SfdClient(new Settings { Backend = origin.AbsoluteUri, MaxUploadBytes = 1 }))
                {
                    bool blocked = false;
                    try { await limited.Upload(Path.Combine(root, "Δοκιμή 世界 with spaces.txt"), null, CancellationToken.None); }
                    catch (ClientFault e) { blocked = e.Message.Contains("companion limit") && limited.LastAttempt == null; }
                    Check(blocked, "client size preflight fails before metadata creation");
                }
                using (var bad = new SfdClient(new Settings { Backend = untrusted.AbsoluteUri }))
                    Check(!await bad.Probe(CancellationToken.None), "untrusted real TLS certificate rejected; no validation bypass");
                using (var down = new SfdClient(new Settings { Backend = unavailable.AbsoluteUri, AllowSyntheticLoopbackHttp = true }))
                    Check(!await down.Probe(CancellationToken.None), "owned unavailable backend produces unavailable status");
                secret = null;
            }
            finally
            {
                if (File.ReadAllText(Path.Combine(root, "owner")) == suffix &&
                    Path.GetFullPath(root).StartsWith(Path.GetFullPath(Path.GetTempPath()), StringComparison.Ordinal))
                    Directory.Delete(root, true);
            }
            Console.WriteLine("DESKTOP REAL BACKEND CHECKS PASSED: " + passed);
        }
        static int Main(string[] args)
        {
            try { Run(args).GetAwaiter().GetResult(); return 0; }
            catch (Exception e) { Console.Error.WriteLine("DESKTOP FAIL: " + e.GetType().Name); return 1; }
        }
    }
}
