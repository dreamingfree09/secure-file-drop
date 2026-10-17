using System;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Diagnostics;
using System.Linq;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
using Microsoft.Win32;
using SecureFileDrop.Desktop;

namespace SecureFileDrop.Tests
{
    static class WindowsTests
    {
        static int passed;
        static void Check(bool value, string label) { if (!value) throw new Exception(label); passed++; Console.WriteLine("PASS: " + label); }
        static void Reject(Action action, string label) { bool rejected = false; try { action(); } catch (ClientFault) { rejected = true; } Check(rejected, label); }
        [STAThread]
        static int Main(string[] args)
        {
            Console.OutputEncoding = new UTF8Encoding(false);
            if (args.Length == 2 && args[0] == "--send") { Console.WriteLine(args[1]); return 0; }
            Console.WriteLine("WINDOWS TEST RUNTIME: " + Environment.Version.ToString());
            string owner = Guid.NewGuid().ToString("N");
            string root = Path.Combine(Path.GetTempPath(), "sfd-desktop-test-" + owner);
            string registryRoot = @"Software\SecureFileDrop.Tests\" + owner;
            string reg = registryRoot + @"\verb";
            bool registryOwned = false;
            if (Directory.Exists(root)) return 2;
            Directory.CreateDirectory(root); File.WriteAllText(Path.Combine(root, "owner"), owner);
            try
            {
                Reject(delegate { new Settings { Backend = "http://example.test" }.Origin(); }, "remote HTTP rejected");
                Reject(delegate { new Settings { Backend = "https://user:password@example.test" }.Origin(); }, "credential-bearing backend rejected");
                Reject(delegate { new Settings { Backend = "https://example.test/path" }.Origin(); }, "non-origin backend rejected");
                Reject(delegate { new Settings { Backend = "https://example.test/?token=secret" }.Origin(); }, "query-bearing backend rejected");
                Check(new Settings { Backend = "http://127.0.0.1:1234", AllowLoopbackHttp = true }.Origin().Scheme == "http", "explicit literal loopback development HTTP");
                string file = Path.Combine(root, "Δοκιμή 世界 space & $(noop).txt"); File.WriteAllText(file, "synthetic bytes");
                Check(WindowsFiles.Validate(file) == file, "spaces and Unicode filename validated");
                Reject(delegate { WindowsFiles.Validate(file + ":hidden"); }, "alternate data stream rejected");
                Reject(delegate { WindowsFiles.Validate(@"\\198.51.100.1\unowned\file"); }, "network argument rejected before access");
                Reject(delegate { WindowsFiles.Validate("--not-a-file"); }, "option-like relative argument rejected");
                var origin = new Uri("https://example.test:8443/");
                var sessions = new ProtectedSessions(root);
                string token = "SYNTHETIC_SESSION_" + owner;
                var cookie = new Cookie("sfd_session", token, "/") { Expires = DateTime.UtcNow.AddMinutes(10), HttpOnly = true, Secure = true };
                sessions.Save(origin, cookie);
                string[] blobs = Directory.GetFiles(Path.Combine(root, "sessions"));
                Check(blobs.Length == 1 && Encoding.UTF8.GetString(File.ReadAllBytes(blobs[0])).IndexOf(token, StringComparison.Ordinal) < 0, "DPAPI file contains no plaintext synthetic token");
                Check(sessions.Load(origin).Value == token, "current-user DPAPI session roundtrip");
                Check(sessions.Load(new Uri("https://other.example.test/")) == null, "session isolated by exact backend origin");
                byte[] corrupted = File.ReadAllBytes(blobs[0]); corrupted[corrupted.Length / 2] ^= 1; File.WriteAllBytes(blobs[0], corrupted);
                Check(sessions.Load(origin) == null, "tampered DPAPI session rejected");
                sessions.Save(origin, cookie); sessions.Clear(origin);
                Check(Directory.GetFiles(Path.Combine(root, "sessions")).Length == 0, "specific protected session removed");
                using (var existingRoot = Registry.CurrentUser.OpenSubKey(registryRoot))
                    if (existingRoot != null) throw new Exception("Test registry namespace already exists; preserved.");
                using (var ownedRoot = Registry.CurrentUser.CreateSubKey(registryRoot)) ownedRoot.SetValue("TestOwner", owner);
                registryOwned = true;
                string exe = Process.GetCurrentProcess().MainModule.FileName;
                ExplorerVerb.InstallAt(reg, exe);
                using (var command = Registry.CurrentUser.OpenSubKey(reg + @"\command"))
                    Check((string)command.GetValue("") == WindowsFiles.Command(exe), "per-user Explorer command fully quotes executable and selected file");
                var start = new ProcessStartInfo(exe, "--send \"" + file + "\"") { UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true, StandardOutputEncoding = Encoding.UTF8 };
                using (var child = Process.Start(start))
                {
                    string result = child.StandardOutput.ReadToEnd().TrimEnd('\r', '\n');
                    Check(child.WaitForExit(5000) && child.ExitCode == 0 && result == file, "actual Windows argument roundtrip preserves Unicode/spaces/metacharacters");
                }
                ExplorerVerb.RemoveAt(reg);
                Check(Registry.CurrentUser.OpenSubKey(reg) == null, "owned test Explorer verb removed");
                using (var existing = Registry.CurrentUser.CreateSubKey(reg)) existing.SetValue("Owner", "unrelated-synthetic-entry");
                Reject(delegate { ExplorerVerb.InstallAt(reg, exe); }, "unowned Explorer entry preserved");
                using (var existing = Registry.CurrentUser.OpenSubKey(reg)) Check((string)existing.GetValue("Owner") == "unrelated-synthetic-entry", "existing registry contents unchanged");
                using (var first = new SingleInstance("-test-" + owner))
                using (var second = new SingleInstance("-test-" + owner))
                using (var received = new ManualResetEventSlim())
                {
                    string path = null;
                    Check(first.Primary && !second.Primary, "single-instance ownership established");
                    Task listening = first.Listen(delegate(string input) { path = input; received.Set(); });
                    second.Forward(file);
                    Check(received.Wait(5000) && path == file, "current-user/session pipe roundtrip preserves selected path");
                }
                var linkOrigin = new Uri("https://example.test:8443/");
                string expiry = DateTime.UtcNow.AddMinutes(5).ToString("o");
                SfdClient.ValidateLink(new Link { Url = "https://example.test:8443/download?token=abc.DEF_123", ExpiresAt = expiry }, linkOrigin);
                Check(true, "valid bounded HTTPS bearer-link schema accepted");
                Reject(delegate { SfdClient.ValidateLink(new Link { Url = "https://example.test:9443/download?token=abc.def", ExpiresAt = expiry }, linkOrigin); }, "different link authority/port rejected");
                Reject(delegate { SfdClient.ValidateLink(new Link { Url = "https://example.test:8443/download?token=abc.def", ExpiresAt = DateTime.UtcNow.AddSeconds(-1).ToString("o") }, linkOrigin); }, "expired download link rejected");
                Reject(delegate { SfdClient.ValidateLink(new Link { Url = "https://example.test:8443/download?token=abc.def" + (char)13 + (char)10 + "extra", ExpiresAt = expiry }, linkOrigin); }, "clipboard link control characters rejected");
                Reject(delegate { SfdClient.ValidateLink(new Link { Url = "https://example.test:8443/download?token=abc.def&next=other", ExpiresAt = expiry }, linkOrigin); }, "extra signed-link query parameters rejected");
                Reject(delegate { SfdClient.ValidateLink(new Link { Url = "https://user:password@example.test:8443/download?token=abc.def", ExpiresAt = expiry }, linkOrigin); }, "credential-bearing download link rejected");
                TestStreaming(root).GetAwaiter().GetResult();
                ProtocolTests.Run(root).GetAwaiter().GetResult();
                Application.EnableVisualStyles(); Application.SetCompatibleTextRenderingDefault(false);
                // A real WinForms context is created and disposed from an STA thread. Visual Explorer/file-picker interaction is a separate manual gate.
                var ownedListener = new TcpListener(IPAddress.Loopback, 0);
                ownedListener.Start();
                int ownedPort = ((IPEndPoint)ownedListener.LocalEndpoint).Port;
                Task ownedService = Task.Run(delegate
                {
                    try
                    {
                        using (var connection = ownedListener.AcceptTcpClient())
                        using (var stream = connection.GetStream())
                        {
                            byte[] response = Encoding.ASCII.GetBytes("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}");
                            stream.Write(response, 0, response.Length);
                        }
                    }
                    catch (SocketException) { }
                });
                Directory.CreateDirectory(Path.Combine(root, "ui"));
                File.WriteAllBytes(Path.Combine(root, "ui", "settings.json"), Json.Encode(new Settings { Backend = "http://127.0.0.1:" + ownedPort, AllowLoopbackHttp = true }));
                using (var single = new SingleInstance("-ui-test-" + owner))
                using (var context = new TrayContext(Path.Combine(root, "ui"), single, ""))
                {
                    var trayField = typeof(TrayContext).GetField("tray", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic);
                    var icon = (NotifyIcon)trayField.GetValue(context);
                    Check(icon.Visible && icon.ContextMenuStrip.Items.OfType<ToolStripMenuItem>().Any(x => x.Text == "Quick Send…"), "native NotifyIcon and Quick Send menu instantiated");
                    var exit = icon.ContextMenuStrip.Items.OfType<ToolStripMenuItem>().Single(x => x.Text == "Exit");
                    var close = new System.Windows.Forms.Timer { Interval = 100 };
                    close.Tick += delegate { close.Stop(); exit.PerformClick(); };
                    // Prevent first-run configuration dialog in this isolated test profile.
                    
                    close.Start(); Application.Run(context); close.Dispose();
                    Check(!icon.Visible, "tray exit event removes notification icon and ends owned context");
                    ownedListener.Stop();
                    Check(ownedService.Wait(5000), "owned readiness fixture stopped; no user service touched");
                }
                Console.WriteLine("WINDOWS CHECKS PASSED: " + passed);
                Console.WriteLine("NOT EXECUTED: visual notification-area/file-picker/Explorer user interaction; production executable in a real user profile.");
                return 0;
            }
            catch (Exception e) { Console.Error.WriteLine("FAIL: " + e.GetType().Name + " / " + e.Message); return 1; }
            finally
            {
                // This key and directory were created fresh with a unique test ID; never enumerate or delete user application data.
                if (registryOwned)
                {
                    bool owned;
                    using (var ownedRoot = Registry.CurrentUser.OpenSubKey(registryRoot))
                        owned = ownedRoot != null && (string)ownedRoot.GetValue("TestOwner") == owner;
                    if (owned) Registry.CurrentUser.DeleteSubKeyTree(registryRoot, false);
                }
                if (Path.GetFullPath(root).StartsWith(Path.GetFullPath(Path.GetTempPath()), StringComparison.OrdinalIgnoreCase) &&
                    File.Exists(Path.Combine(root, "owner")) && File.ReadAllText(Path.Combine(root, "owner")) == owner) Directory.Delete(root, true);
            }
        }
        static async Task TestStreaming(string root)
        {
            string file = Path.Combine(root, "stream.bin");
            const long size = 64L * 1024 * 1024;
            using (var create = File.Create(file)) create.SetLength(size);
            long sent = 0;
            using (var input = File.OpenRead(file))
            using (var sink = new CountingSink())
            using (var content = new ProgressContent(input, size, delegate(long n, long total) { sent = n; }, CancellationToken.None))
            {
                await content.CopyToAsync(sink);
                Check(sink.Bytes == size && sent == size && sink.MaxWrite <= 65536 && content.Digest != null, "64 MiB local streaming mechanism uses bounded writes and real progress/hash; not backend capacity acceptance");
            }
            using (var cancel = new CancellationTokenSource())
            using (var input = File.OpenRead(file))
            using (var sink = new CountingSink())
            using (var content = new ProgressContent(input, size, delegate(long n, long total) { cancel.Cancel(); }, cancel.Token))
            {
                bool cancelled = false;
                try { await content.CopyToAsync(sink); } catch (OperationCanceledException) { cancelled = true; } catch (System.Net.Http.HttpRequestException) { cancelled = cancel.IsCancellationRequested; }
                Check(cancelled && sink.Bytes < size, "cancellation interrupts streaming before all file bytes");
            }
        }
        sealed class CountingSink : Stream
        {
            public long Bytes; public int MaxWrite;
            public override bool CanRead { get { return false; } } public override bool CanSeek { get { return false; } } public override bool CanWrite { get { return true; } }
            public override long Length { get { return Bytes; } } public override long Position { get { return Bytes; } set { throw new NotSupportedException(); } }
            public override void Flush() { } public override int Read(byte[] b, int o, int n) { throw new NotSupportedException(); }
            public override long Seek(long o, SeekOrigin origin) { throw new NotSupportedException(); } public override void SetLength(long n) { throw new NotSupportedException(); }
            public override void Write(byte[] b, int o, int n) { Bytes += n; MaxWrite = Math.Max(MaxWrite, n); }
            public override Task WriteAsync(byte[] b, int o, int n, CancellationToken token) { token.ThrowIfCancellationRequested(); Write(b, o, n); return Task.FromResult(0); }
        }
    }
}
