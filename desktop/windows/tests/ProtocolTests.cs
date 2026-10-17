using System;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Security.Cryptography;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using SecureFileDrop.Desktop;

namespace SecureFileDrop.Tests
{
    static class ProtocolTests
    {
        static void Check(bool good, string name) { if (!good) throw new Exception(name); Console.WriteLine("PROTOCOL PASS: " + name); }
        internal static async Task Run(string root)
        {
            using (var server = new Fixture())
            using (var other = new Fixture())
            using (var client = new SfdClient(new Settings { Backend = server.Address, AllowLoopbackHttp = true, MaxUploadBytes = 2097152 }))
            {
                await client.Login("synthetic", "Synthetic1", CancellationToken.None);
                Check(client.Authenticated, "Windows HTTP cookie login and strict /me readback");
                string file = Path.Combine(root, "HTTP Δ 世界 space & $.bin");
                byte[] bytes = new byte[1048576]; for (int i = 0; i < bytes.Length; i++) bytes[i] = (byte)(i % 241);
                File.WriteAllBytes(file, bytes);
                long progress = 0;
                var result = await client.Upload(file, delegate(long n, long total) { Interlocked.Exchange(ref progress, n); }, CancellationToken.None);
                Check(result.Confirmed && result.Size == bytes.Length && server.Bytes == bytes.Length && Interlocked.Read(ref progress) == bytes.Length, "Windows real HTTP multipart stream/progress/server-hash reconciliation");
                int filenameStart = server.FilenameHeader.IndexOf("filename*=utf-8\'\'", StringComparison.OrdinalIgnoreCase);
                string encodedName = filenameStart < 0 ? "" : server.FilenameHeader.Substring(filenameStart + "filename*=utf-8\'\'".Length).Split(';')[0].Trim();
                Check(filenameStart >= 0 && Uri.UnescapeDataString(encodedName) == Path.GetFileName(file), "Windows RFC5987 decoded Unicode filename is exact");
                bool linkRejected = false; try { await client.CreateLink(CancellationToken.None); } catch (ClientFault) { linkRejected = true; }
                Check(linkRejected, "HTTP download link is not copied as HTTPS");
                server.Expired = true;
                bool expired = false; try { await client.Restore(CancellationToken.None); } catch (ClientFault e) { expired = e.Status == 401; }
                Check(expired && !client.Authenticated, "Windows expired/revoked-session response clears active authentication");
                server.Expired = false; await client.Login("synthetic", "Synthetic1", CancellationToken.None);
                server.Defer = true; server.Uploaded.Reset();
                using (var cancellation = new CancellationTokenSource())
                {
                    Task<TransferAttempt> transfer = client.Upload(file, null, cancellation.Token);
                    Check(server.Uploaded.Wait(5000), "owned protocol fixture received upload before cancellation");
                    cancellation.Cancel();
                    bool stopped = false;
                    try { await transfer; } catch (OperationCanceledException) { stopped = true; }
                    Check(stopped && client.LastAttempt != null, "Windows in-flight cancellation retains known transfer identity");
                    server.Release.Set();
                    var recovered = await client.Reconcile(CancellationToken.None);
                    Check(recovered.Confirmed, "uncertain response reconciles existing hashed bytes without resend or deletion");
                }
                server.Expired = false; server.DeferLogout = true; server.LogoutStarted.Reset();
                Check(!await client.SignOutForConfiguration(CancellationToken.None) && !client.Authenticated, "old-backend logout timeout returns an incomplete result so configuration can continue");
                server.ReleaseLogout.Set();
                using (var replacement = new SfdClient(new Settings { Backend = other.Address, AllowLoopbackHttp = true }))
                    Check(await replacement.Probe(CancellationToken.None), "replacement backend is reachable after old-backend timeout");
                server.Expired = false; server.DeferLogout = false;
                await client.Login("synthetic", "Synthetic1", CancellationToken.None);
                server.DeferLogout = true; server.ReleaseLogout.Reset(); server.LogoutStarted.Reset();
                using (var cancelledConfiguration = new CancellationTokenSource())
                {
                    Task<bool> changing = client.SignOutForConfiguration(cancelledConfiguration.Token);
                    if (!server.LogoutStarted.Wait(5000)) throw new Exception("Owned logout fixture was not contacted.");
                    cancelledConfiguration.Cancel();
                    bool explicitCancel = false; try { await changing; } catch (OperationCanceledException) { explicitCancel = true; }
                    Check(explicitCancel, "explicit user cancellation still aborts reconfiguration");
                    server.ReleaseLogout.Set();
                }
                server.DeferLogout = false; server.Expired = false;
                await client.Login("synthetic", "Synthetic1", CancellationToken.None);
                Interlocked.Exchange(ref other.Requests, 0);
                server.Redirect = other.Address;
                bool redirect = false; try { await client.Login("synthetic", "Synthetic1", CancellationToken.None); } catch (ClientFault e) { redirect = e.Status == 302; }
                Check(redirect && other.Requests == 0, "redirect never receives password, cookie or file bytes");
                server.Redirect = null; server.Expired = false; server.DeferLogout = false; server.LogoutStarted.Reset();
                var protectedStore = new ProtectedSessions(Path.Combine(root, "unverified-session"));
                var rememberedOrigin = new Uri(server.Address);
                protectedStore.Save(rememberedOrigin, new Cookie("sfd_session", "SYNTHETIC_ONLY", "/") { HttpOnly = true, Expires = DateTime.UtcNow.AddMinutes(10) });
                using (var restored = new SfdClient(new Settings { Backend = server.Address, AllowLoopbackHttp = true, RememberSession = true }, protectedStore))
                {
                    bool signedOut = await restored.SignOutForConfiguration(CancellationToken.None);
                    Check(signedOut && server.LogoutStarted.IsSet && protectedStore.Load(rememberedOrigin) == null, "unverified remembered session is signed out and removed before reconfiguration");
                }

            }
            using (var down = new Fixture())
            {
                string address = down.Address; down.Dispose();
                using (var client = new SfdClient(new Settings { Backend = address, AllowLoopbackHttp = true }))
                    Check(!await client.Probe(CancellationToken.None), "Windows unavailable backend is reported unavailable");
            }
        }
        sealed class Fixture : IDisposable
        {
            readonly TcpListener listener = new TcpListener(IPAddress.Loopback, 0);
            readonly Task listening;
            volatile bool stopping;
            string id = Guid.NewGuid().ToString(), hash = "";
            public string FilenameHeader = "";
            public volatile string Redirect;
            public volatile bool Expired, Defer, DeferLogout;
            public long Bytes;
            public int Requests;
            public readonly ManualResetEventSlim Uploaded = new ManualResetEventSlim();
            public readonly ManualResetEventSlim Release = new ManualResetEventSlim();
            public readonly ManualResetEventSlim LogoutStarted = new ManualResetEventSlim();
            public readonly ManualResetEventSlim ReleaseLogout = new ManualResetEventSlim();
            public readonly string Address;
            public Fixture()
            {
                listener.Start(); Address = "http://127.0.0.1:" + ((IPEndPoint)listener.LocalEndpoint).Port + "/";
                listening = Task.Run((Action)Loop);
            }
            void Loop()
            {
                while (!stopping)
                {
                    try { TcpClient client = listener.AcceptTcpClient(); Task.Run(delegate { Handle(client); }); }
                    catch (SocketException) { if (stopping) return; throw; }
                }
            }
            void Handle(TcpClient client)
            {
                try
                {
                    using (client)
                    using (NetworkStream stream = client.GetStream())
                    {
                        stream.ReadTimeout = 10000; stream.WriteTimeout = 10000;
                        var headers = new StringBuilder();
                        while (headers.Length < 16384)
                        {
                            int b = stream.ReadByte(); if (b < 0) return; headers.Append((char)b);
                            if (headers.ToString().EndsWith("\r\n\r\n", StringComparison.Ordinal)) break;
                        }
                        string head = headers.ToString(); string path = head.Split(' ')[1]; int length = 0;
                        foreach (string line in head.Split(new string[] { "\r\n" }, StringSplitOptions.None))
                            if (line.StartsWith("Content-Length:", StringComparison.OrdinalIgnoreCase)) length = Int32.Parse(line.Substring(15).Trim());
                        if (length < 0 || length > 4194304) return;
                        if (head.IndexOf("Expect: 100-continue", StringComparison.OrdinalIgnoreCase) >= 0)
                        { byte[] go = Encoding.ASCII.GetBytes("HTTP/1.1 100 Continue\r\n\r\n"); stream.Write(go, 0, go.Length); }
                        byte[] body = new byte[length]; int offset = 0;
                        while (offset < length) { int n = stream.Read(body, offset, length - offset); if (n == 0) return; offset += n; }
                        Interlocked.Increment(ref Requests);
                        int status = 200; byte[] data = Encoding.UTF8.GetBytes("{\"status\":\"ok\"}"); string extra = "";
                        if (path == "/login")
                        {
                            if (Redirect != null) { status = 302; extra = "Location: " + Redirect + "\r\n"; }
                            else extra = "Set-Cookie: sfd_session=SYNTHETIC_ONLY; Path=/; HttpOnly; Expires=" + DateTime.UtcNow.AddMinutes(10).ToString("R") + "\r\n";
                        }
                        else if (path != "/ready" && Expired) status = 401;
                        else if (path == "/files" && head.StartsWith("POST", StringComparison.Ordinal))
                        { id = Guid.NewGuid().ToString(); hash = ""; Bytes = 0; Uploaded.Reset(); data = Json.Encode(new FileRow { Id = id, Status = "pending" }); status = 201; }
                        else if (path.StartsWith("/upload?", StringComparison.Ordinal))
                        {
                            string text = Encoding.UTF8.GetString(body);
                            int start = text.IndexOf("\r\n\r\n", StringComparison.Ordinal);
                            FilenameHeader = text.Substring(0, start);
                            int byteStart = Encoding.UTF8.GetByteCount(text.Substring(0, start + 4));
                            string boundary = text.Substring(0, text.IndexOf("\r\n", StringComparison.Ordinal));
                            byte[] marker = Encoding.UTF8.GetBytes("\r\n" + boundary + "--");
                            int end = Find(body, marker);
                            if (byteStart < 0 || end < byteStart) throw new Exception("Multipart fixture boundary.");
                            Bytes = end - byteStart;
                            using (var sha = SHA256.Create()) hash = BitConverter.ToString(sha.ComputeHash(body, byteStart, (int)Bytes)).Replace("-", "").ToLowerInvariant();
                            Uploaded.Set();
                            if (Defer) Release.Wait(5000);
                            data = Json.Encode(new FileRow { Id = id, Status = "hashed" });
                        }
                        else if (path == "/files") data = Json.Encode(new FileRow[] { new FileRow { Id = id, Status = "hashed", Hash = hash, Size = Bytes } });
                        else if (path == "/links") data = Json.Encode(new Link { Url = Address + "download?token=SYNTHETIC", ExpiresAt = DateTime.UtcNow.AddMinutes(5).ToString("o") });
                        else if (path == "/logout")
                        {
                            Expired = true; LogoutStarted.Set();
                            if (DeferLogout) ReleaseLogout.Wait(20000);
                        }
                        byte[] prefix = Encoding.ASCII.GetBytes("HTTP/1.1 " + status + " Test\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: " + data.Length + "\r\n" + extra + "\r\n");
                        stream.Write(prefix, 0, prefix.Length); stream.Write(data, 0, data.Length);
                    }
                }
                catch (IOException) { }
                catch (SocketException) { }
            }
            static int Find(byte[] body, byte[] marker)
            {
                for (int i = body.Length - marker.Length; i >= 0; i--)
                { bool same = true; for (int j = 0; j < marker.Length; j++) if (body[i + j] != marker[j]) { same = false; break; } if (same) return i; }
                return -1;
            }
            public void Dispose()
            {
                if (stopping) return; stopping = true; Release.Set(); ReleaseLogout.Set(); listener.Stop(); listening.Wait(5000);
            }
        }
    }
}
