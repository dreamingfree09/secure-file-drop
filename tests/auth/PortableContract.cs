// Schema12 adapter of the preserved portable contract; product Client.cs is unchanged.
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
    [DataContract] public sealed class FixtureData
    {
        [DataMember] public string Origin;
        [DataMember] public string FixtureOwner;
        [DataMember] public string Uploader;
        [DataMember] public string UploaderPassword;
        [DataMember] public string Other;
        [DataMember] public string OtherPassword;
        [DataMember] public string UntrustedOrigin;
        [DataMember] public string UnavailableOrigin;
    }
    static class ContractRunner
    {
        static int passed;
        static void Check(bool value, string name) { if (!value) throw new Exception(name); passed++; Console.WriteLine("DESKTOP PASS: " + name); }
        static string Hash(byte[] bytes) { using (var sha = SHA256.Create()) return BitConverter.ToString(sha.ComputeHash(bytes)).Replace("-", "").ToLowerInvariant(); }
        static async Task Run(string[] args)
        {
            if (args.Length != 1) throw new Exception("Private owned fixture file required.");
            var fixture = Json.Decode<FixtureData>(File.ReadAllBytes(args[0]));
            if (fixture.FixtureOwner == null || !System.Text.RegularExpressions.Regex.IsMatch(fixture.FixtureOwner, "^sfd-auth-[0-9a-f]{12}$") ||
                Environment.GetEnvironmentVariable("SFD_DESKTOP_RUN") != fixture.FixtureOwner ||
                fixture.Uploader != "uploader" || fixture.Other != "second") throw new Exception("Owned schema12 fixture required.");
            Uri origin = new Uri(fixture.Origin), untrusted = new Uri(fixture.UntrustedOrigin), unavailable = new Uri(fixture.UnavailableOrigin);
            if (origin.Scheme != "https" || origin.Host != "localhost" || origin.Port == 8443 || !untrusted.IsLoopback || !unavailable.IsLoopback)
                throw new Exception("Owned loopback HTTPS fixtures required.");
            string suffix = Guid.NewGuid().ToString("N");
            string root = Path.Combine(Path.GetTempPath(), "sfd-desktop-contract-" + suffix);
            if (Directory.Exists(root)) throw new Exception("Synthetic file namespace already exists; preserved.");
            Directory.CreateDirectory(root); File.WriteAllText(Path.Combine(root, "owner"), suffix);
            try
            {
                // The real fixture registers both ordinary accounts and completes delivered email
                // verification before exposing readiness; no direct verification/database shortcut.
                string user = fixture.Uploader, other = fixture.Other;
                string secret = fixture.UploaderPassword, otherSecret = fixture.OtherPassword;
                using (var first = new SfdClient(new Settings { Backend = origin.AbsoluteUri }))
                {
                    await first.Login(user, secret, CancellationToken.None);
                    Check(first.Authenticated, "real delivered-verification ordinary uploader identity");
                    await first.Logout(CancellationToken.None);
                }
                using (var second = new SfdClient(new Settings { Backend = origin.AbsoluteUri }))
                {
                    await second.Login(other, otherSecret, CancellationToken.None);
                    Check(second.Authenticated, "real delivered-verification distinct ordinary account identity");
                    await second.Logout(CancellationToken.None);
                }
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
                    await client.Login(other, otherSecret, CancellationToken.None);
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
