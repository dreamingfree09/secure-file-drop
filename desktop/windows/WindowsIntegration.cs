using System;
using System.Diagnostics;
using System.IO;
using System.IO.Pipes;
using System.Net;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Runtime.Serialization;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.Win32;

namespace SecureFileDrop.Desktop
{
    [DataContract] sealed class StoredSession
    {
        [DataMember] public string Origin;
        [DataMember] public string Token;
        [DataMember] public string Expires;
        [DataMember] public bool Secure;
    }
    public sealed class ProtectedSessions : ISessionStore
    {
        readonly string directory;
        public ProtectedSessions(string root) { directory = Path.Combine(root, "sessions"); }
        string FileFor(Uri origin)
        {
            using (var hash = SHA256.Create())
                return Path.Combine(directory, BitConverter.ToString(hash.ComputeHash(Encoding.UTF8.GetBytes(origin.AbsoluteUri))).Replace("-", "") + ".dat");
        }
        public Cookie Load(Uri origin)
        {
            string file = FileFor(origin);
            if (!File.Exists(file)) return null;
            try
            {
                if (new FileInfo(file).Length > 16384) return null;
                byte[] plain = ProtectedData.Unprotect(File.ReadAllBytes(file), null, DataProtectionScope.CurrentUser);
                StoredSession value;
                try { value = Json.Decode<StoredSession>(plain); }
                finally { Array.Clear(plain, 0, plain.Length); }
                DateTime expiry;
                if (value == null || value.Origin != origin.AbsoluteUri || String.IsNullOrEmpty(value.Token) ||
                    !DateTime.TryParse(value.Expires, null, System.Globalization.DateTimeStyles.RoundtripKind, out expiry) ||
                    expiry.ToUniversalTime() <= DateTime.UtcNow || (origin.Scheme == "https" && !value.Secure)) return null;
                return new Cookie("sfd_session", value.Token, "/") { Expires = expiry, Secure = value.Secure, HttpOnly = true };
            }
            catch (CookieException) { return null; }
            catch (CryptographicException) { return null; }
            catch (ClientFault) { return null; }
            catch (IOException) { return null; }
            catch (UnauthorizedAccessException) { return null; }
        }
        public void Save(Uri origin, Cookie cookie)
        {
            Directory.CreateDirectory(directory);
            byte[] plain = Json.Encode(new StoredSession { Origin = origin.AbsoluteUri, Token = cookie.Value, Expires = cookie.Expires.ToUniversalTime().ToString("o"), Secure = cookie.Secure });
            byte[] encrypted;
            try { encrypted = ProtectedData.Protect(plain, null, DataProtectionScope.CurrentUser); }
            finally { Array.Clear(plain, 0, plain.Length); }
            string target = FileFor(origin), temp = target + "." + Guid.NewGuid().ToString("N") + ".tmp";
            try
            {
                using (var stream = new FileStream(temp, FileMode.CreateNew, FileAccess.Write, FileShare.None)) stream.Write(encrypted, 0, encrypted.Length);
                if (File.Exists(target)) File.Replace(temp, target, null); else File.Move(temp, target);
            }
            finally { if (File.Exists(temp)) File.Delete(temp); }
        }
        public void Clear(Uri origin)
        {
            string file = FileFor(origin);
            try { if (File.Exists(file)) File.Delete(file); }
            catch (IOException) { throw new ClientFault("The protected session file could not be removed. Check permissions and retry sign-out."); }
            catch (UnauthorizedAccessException) { throw new ClientFault("The protected session file could not be removed. Check permissions and retry sign-out."); }
        }
    }
    public static class WindowsFiles
    {
        public static string Validate(string input)
        {
            if (String.IsNullOrWhiteSpace(input) || input.Length > 32700 || input.IndexOfAny(new char[] { '\0', '\r', '\n', '"' }) >= 0)
                throw new ClientFault("Choose one ordinary file.");
            // A Shell verb accepts a local drive path, never a device, ADS, relative path or network command.
            if (input.Length < 3 || !Char.IsLetter(input[0]) || input[1] != ':' || (input[2] != '\\' && input[2] != '/') || input.IndexOf(':', 2) >= 0)
                throw new ClientFault("Select a local drive file. Network/device paths and alternate data streams are unsupported.");
            string full = Path.GetFullPath(input);
            FileAttributes attributes = File.GetAttributes(full);
            if ((attributes & (FileAttributes.Directory | FileAttributes.Device | FileAttributes.ReparsePoint)) != 0)
                throw new ClientFault("Select a regular local file; download cloud placeholders locally first.");
            return full;
        }
        public static string Command(string executable)
        {
            if (String.IsNullOrWhiteSpace(executable) || executable.IndexOf('"') >= 0 || !Path.IsPathRooted(executable))
                throw new ClientFault("The companion executable path is invalid.");
            return "\"" + executable + "\" --send \"%1\"";
        }
    }
    public static class ExplorerVerb
    {
        const string Key = @"Software\Classes\*\shell\SecureFileDrop.Send";
        const string Marker = "SFD-Tray-20261017";
        public static void Install(string executable) { InstallAt(Key, executable); }
        internal static void InstallAt(string keyPath, string executable)
        {
            string command = WindowsFiles.Command(executable);
            using (var existing = Registry.CurrentUser.OpenSubKey(keyPath))
                if (existing != null && (string)existing.GetValue("Owner") != Marker)
                    throw new ClientFault("An unrelated Explorer entry already exists; it was preserved.");
            using (var key = Registry.CurrentUser.CreateSubKey(keyPath))
            {
                key.SetValue("Owner", Marker);
                key.SetValue("MUIVerb", "Send with Secure File Drop");
                key.SetValue("Icon", "\"" + executable + "\",0");
                key.SetValue("MultiSelectModel", "Single");
                using (var action = key.CreateSubKey("command")) action.SetValue("", command);
            }
        }
        public static void Remove() { RemoveAt(Key); }
        internal static void RemoveAt(string keyPath)
        {
            using (var key = Registry.CurrentUser.OpenSubKey(keyPath))
            {
                if (key == null) return;
                if ((string)key.GetValue("Owner") != Marker) throw new ClientFault("The Explorer entry is not owned by this companion; it was preserved.");
            }
            Registry.CurrentUser.DeleteSubKeyTree(keyPath);
        }
    }
    // Session-scoped, current-user-only IPC: no shell, credentials, URLs or file contents.
    public sealed class SingleInstance : IDisposable
    {
        readonly Mutex mutex;
        readonly string pipe;
        readonly SecurityIdentifier sid;
        readonly CancellationTokenSource stop = new CancellationTokenSource();
        public readonly bool Primary;
        public SingleInstance() : this("") { }
        internal SingleInstance(string suffix)
        {
            sid = WindowsIdentity.GetCurrent().User;
            string name = "SecureFileDrop-" + sid.Value + "-" + Process.GetCurrentProcess().SessionId + suffix;
            bool created;
            mutex = new Mutex(true, @"Local\" + name, out created);
            Primary = created; pipe = name;
        }
        public void Forward(string path)
        {
            using (var client = new NamedPipeClientStream(".", pipe, PipeDirection.Out, PipeOptions.Asynchronous))
            {
                client.Connect(3000);
                byte[] data = Encoding.UTF8.GetBytes(path ?? "");
                if (data.Length > 131072) throw new ClientFault("The selected path is too long.");
                byte[] size = BitConverter.GetBytes(data.Length);
                client.Write(size, 0, size.Length); client.Write(data, 0, data.Length); client.Flush();
            }
        }
        static async Task ReadExactly(Stream stream, byte[] buffer, CancellationToken token)
        {
            int pos = 0;
            while (pos < buffer.Length)
            {
                int n = await stream.ReadAsync(buffer, pos, buffer.Length - pos, token).ConfigureAwait(false);
                if (n == 0) throw new IOException("Incomplete IPC message.");
                pos += n;
            }
        }
        public async Task Listen(Action<string> received)
        {
            while (!stop.IsCancellationRequested)
            {
                var security = new PipeSecurity();
                security.SetAccessRuleProtection(true, false);
                security.AddAccessRule(new PipeAccessRule(sid, PipeAccessRights.FullControl, AccessControlType.Allow));
                try
                {
                    using (var server = new NamedPipeServerStream(pipe, PipeDirection.In, 1, PipeTransmissionMode.Byte, PipeOptions.Asynchronous, 4096, 4096, security))
                    {
                        await server.WaitForConnectionAsync(stop.Token).ConfigureAwait(false);
                        using (var timeout = CancellationTokenSource.CreateLinkedTokenSource(stop.Token))
                        {
                            timeout.CancelAfter(TimeSpan.FromSeconds(5));
                            byte[] prefix = new byte[4]; await ReadExactly(server, prefix, timeout.Token).ConfigureAwait(false);
                            int size = BitConverter.ToInt32(prefix, 0);
                            if (size < 0 || size > 131072) continue;
                            byte[] body = new byte[size]; await ReadExactly(server, body, timeout.Token).ConfigureAwait(false);
                            received(new UTF8Encoding(false, true).GetString(body));
                        }
                    }
                }
                catch (OperationCanceledException) { }
                catch (IOException) { }
                catch (DecoderFallbackException) { }
            }
        }
        public void Dispose()
        {
            stop.Cancel();
            if (Primary) mutex.ReleaseMutex();
            mutex.Dispose();
            // Listen may still be unwinding an asynchronous wait; its token source stays valid until process exit.
        }
    }
}
