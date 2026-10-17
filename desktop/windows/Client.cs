using System;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Runtime.Serialization;
using System.Runtime.Serialization.Json;
using System.Security.Cryptography;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace SecureFileDrop.Desktop
{
    [DataContract]
    public sealed class Settings
    {
        [DataMember] public string Backend = "https://localhost:8443";
        [DataMember] public bool AllowLoopbackHttp;
        [DataMember] public bool RememberSession;
        [DataMember] public long MaxUploadBytes = 10485760;
        public Uri Origin()
        {
            Uri uri;
            if (!Uri.TryCreate(Backend, UriKind.Absolute, out uri) ||
                uri.UserInfo.Length != 0 || uri.Query.Length != 0 || uri.Fragment.Length != 0 ||
                uri.AbsolutePath != "/" || (uri.Scheme != "https" && uri.Scheme != "http"))
                throw new ClientFault("Enter an HTTPS backend origin without a path, password or query.");
            IPAddress ip;
            bool loopback = IPAddress.TryParse(uri.Host.Trim('[', ']'), out ip) && IPAddress.IsLoopback(ip);
            if (uri.Scheme != "https" && !(AllowLoopbackHttp && loopback))
                throw new ClientFault("HTTPS is required. Explicit development HTTP is limited to literal loopback IPs.");
            if (MaxUploadBytes < 1 || MaxUploadBytes > (50L << 30))
                throw new ClientFault("Choose a file limit from 1 byte to 50 GiB, matching your backend. Capacity above tested sizes remains unverified.");
            return new Uri(uri.GetLeftPart(UriPartial.Authority) + "/");
        }
    }

    public sealed class ClientFault : Exception
    {
        public readonly int Status;
        public ClientFault(string message, int status = 0) : base(message) { Status = status; }
    }
    public interface ISessionStore
    {
        Cookie Load(Uri origin);
        void Save(Uri origin, Cookie cookie);
        void Clear(Uri origin);
    }
    public static class Json
    {
        static DataContractJsonSerializer Serializer(Type type)
        {
            return new DataContractJsonSerializer(type, new DataContractJsonSerializerSettings { MaxItemsInObjectGraph = 8192 });
        }
        public static byte[] Encode<T>(T value)
        {
            using (var stream = new MemoryStream()) { Serializer(typeof(T)).WriteObject(stream, value); return stream.ToArray(); }
        }
        public static T Decode<T>(byte[] value)
        {
            if (value.Length > 524288) throw new ClientFault("The backend response exceeds the supported metadata size.");
            try { using (var stream = new MemoryStream(value)) return (T)Serializer(typeof(T)).ReadObject(stream); }
            catch (SerializationException) { throw new ClientFault("The backend returned invalid metadata."); }
        }
    }
    [DataContract] public sealed class LoginBody
    {
        [DataMember(Name = "username")] public string Username;
        [DataMember(Name = "password")] public string Password;
    }
    [DataContract] public sealed class CreateBody
    {
        [DataMember(Name = "orig_name")] public string Name;
        [DataMember(Name = "content_type")] public string ContentType = "application/octet-stream";
        [DataMember(Name = "size_bytes")] public long Size;
    }
    [DataContract] public sealed class FileRow
    {
        [DataMember(Name = "id")] public string Id;
        [DataMember(Name = "status")] public string Status;
        [DataMember(Name = "size_bytes")] public long Size;
        [DataMember(Name = "sha256_hex")] public string Hash;
    }
    [DataContract] public sealed class LinkBody
    {
        [DataMember(Name = "id")] public string Id;
        [DataMember(Name = "ttl_seconds")] public int Ttl = 300;
    }
    [DataContract] public sealed class Link
    {
        [DataMember(Name = "url")] public string Url;
        [DataMember(Name = "expires_at")] public string ExpiresAt;
    }
    public sealed class TransferAttempt
    {
        public string Id, Path, Hash;
        public long Size;
        public bool Confirmed;
    }
    // Only file bytes are streamed. Progress describes bytes written to HTTP, not server completion.
    public sealed class ProgressContent : HttpContent
    {
        readonly Stream input;
        readonly long length;
        readonly Action<long, long> progress;
        readonly CancellationToken cancel;
        public string Digest { get; private set; }
        public ProgressContent(Stream stream, long size, Action<long, long> report, CancellationToken token)
        {
            input = stream; length = size; progress = report; cancel = token;
            Headers.ContentType = new MediaTypeHeaderValue("application/octet-stream");
        }
        protected override bool TryComputeLength(out long value) { value = length; return true; }
        protected override async Task SerializeToStreamAsync(Stream output, TransportContext context)
        {
            byte[] buffer = new byte[65536];
            long sent = 0;
            using (var hash = SHA256.Create())
            {
                while (true)
                {
                    int n = await input.ReadAsync(buffer, 0, buffer.Length, cancel).ConfigureAwait(false);
                    if (n == 0) break;
                    if (sent > length - n) throw new IOException("File changed.");
                    hash.TransformBlock(buffer, 0, n, buffer, 0);
                    await output.WriteAsync(buffer, 0, n, cancel).ConfigureAwait(false);
                    sent += n;
                    if (progress != null) progress(sent, length);
                }
                if (sent != length) throw new IOException("File changed.");
                hash.TransformFinalBlock(new byte[0], 0, 0);
                Digest = BitConverter.ToString(hash.Hash).Replace("-", "").ToLowerInvariant();
            }
        }
    }

    public sealed class SfdClient : IDisposable
    {
        readonly HttpClient http;
        readonly CookieContainer cookies = new CookieContainer();
        readonly ISessionStore store;
        readonly Settings settings;
        public readonly Uri Origin;
        public bool Authenticated { get; private set; }
        public bool SessionRemembered { get; private set; }
        public bool SessionStorageWarning { get; private set; }
        public TransferAttempt LastAttempt { get; private set; }
        public SfdClient(Settings config, ISessionStore sessions = null)
        {
            settings = config; Origin = config.Origin(); store = sessions;
            // Never forward a password, cookie or file body through redirects; retain OS certificate validation.
            var handler = new HttpClientHandler { AllowAutoRedirect = false, CookieContainer = cookies, UseCookies = true };
            http = new HttpClient(handler) { Timeout = Timeout.InfiniteTimeSpan };
            if (config.RememberSession && store != null)
            {
                Cookie cookie = store.Load(Origin);
                if (cookie != null && cookie.Name == "sfd_session" && cookie.Expires.ToUniversalTime() > DateTime.UtcNow)
                    cookies.Add(Origin, cookie);
            }
        }
        void Forget()
        {
            Authenticated = false; SessionRemembered = false;
            foreach (Cookie cookie in cookies.GetCookies(Origin)) cookie.Expired = true;
            if (store != null) store.Clear(Origin);
        }
        async Task<byte[]> Request(string path, HttpMethod method, HttpContent content, CancellationToken token, int seconds)
        {
            using (var budget = CancellationTokenSource.CreateLinkedTokenSource(token))
            using (var request = new HttpRequestMessage(method, new Uri(Origin, path)))
            {
                budget.CancelAfter(TimeSpan.FromSeconds(seconds));
                request.Content = content;
                request.Headers.TryAddWithoutValidation("Origin", Origin.GetLeftPart(UriPartial.Authority));
                HttpResponseMessage response;
                try { response = await http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, budget.Token).ConfigureAwait(false); }
                catch (HttpRequestException) { throw new ClientFault("Cannot securely reach the backend. Check its address, availability and certificate; TLS validation remains enabled."); }
                using (response)
                {
                    int code = (int)response.StatusCode;
                    if (code == 401) { Forget(); throw new ClientFault("Sign in again; the session is absent, expired or revoked.", code); }
                    if (code < 200 || code > 299)
                    {
                        string message = code == 413 ? "The file exceeds the backend or proxy limit." :
                            code == 403 ? "The account cannot perform this operation." :
                            code == 409 ? "The file state conflicts with this action. Check the last transfer before retrying." :
                            code == 429 ? "The service is limiting requests. Wait before trying again." :
                            code >= 300 && code < 400 ? "The backend redirected this request. Configure its final HTTPS origin." :
                            code >= 500 ? "The service failed. A transfer may still have committed; check its status before retrying." :
                            "The backend rejected this request. Check the file and configuration.";
                        throw new ClientFault(message, code);
                    }
                    using (Stream stream = await response.Content.ReadAsStreamAsync().ConfigureAwait(false))
                    using (var buffer = new MemoryStream())
                    {
                        byte[] block = new byte[8192];
                        int n;
                        while ((n = await stream.ReadAsync(block, 0, block.Length, budget.Token).ConfigureAwait(false)) != 0)
                        {
                            if (buffer.Length + n > 524288) throw new ClientFault("The backend response exceeds the metadata limit.");
                            buffer.Write(block, 0, n);
                        }
                        return buffer.ToArray();
                    }
                }
            }
        }
        static HttpContent Body<T>(T value)
        {
            var result = new ByteArrayContent(Json.Encode(value)); result.Headers.ContentType = new MediaTypeHeaderValue("application/json"); return result;
        }
        public async Task<bool> Probe(CancellationToken token)
        {
            try { await Request("ready", HttpMethod.Get, null, token, 5).ConfigureAwait(false); return true; }
            catch (ClientFault) { return false; }
            catch (OperationCanceledException) { return false; }
        }
        public async Task Restore(CancellationToken token)
        {
            FileRow identity = Json.Decode<FileRow>(await Request("me", HttpMethod.Get, null, token, 12).ConfigureAwait(false));
            if (identity == null || identity.Status != "ok") throw new ClientFault("The backend returned an invalid session check.");
            Authenticated = true;
        }
        public async Task Login(string username, string password, CancellationToken token)
        {
            if (Authenticated) await Logout(token).ConfigureAwait(false); else Forget();
            LastAttempt = null; SessionStorageWarning = false;
            var body = new LoginBody { Username = username, Password = password };
            try { await Request("login", HttpMethod.Post, Body(body), token, 12).ConfigureAwait(false); }
            finally { body.Password = null; password = null; }
            Cookie cookie = cookies.GetCookies(Origin)["sfd_session"];
            if (cookie == null || cookie.Expired || !cookie.HttpOnly || (Origin.Scheme == "https" && !cookie.Secure) || cookie.Expires.ToUniversalTime() <= DateTime.UtcNow)
                { Forget(); throw new ClientFault("The backend did not issue a usable session cookie."); }
            await Restore(token).ConfigureAwait(false);
            if (settings.RememberSession && store != null)
            {
                try { store.Save(Origin, cookie); SessionRemembered = true; }
                catch (IOException) { SessionStorageWarning = true; }
                catch (UnauthorizedAccessException) { SessionStorageWarning = true; }
                catch (CryptographicException) { SessionStorageWarning = true; }
            }
        }
        public async Task Logout(CancellationToken token)
        {
            try { await Request("logout", HttpMethod.Post, null, token, 12).ConfigureAwait(false); }
            finally { Forget(); LastAttempt = null; }
        }
        public async Task<TransferAttempt> Upload(string path, Action<long, long> progress, CancellationToken token)
        {
            LastAttempt = null;
            string name = System.IO.Path.GetFileName(path);
            if (string.IsNullOrWhiteSpace(name) || Encoding.UTF8.GetByteCount(name) > 255 || name.Any(char.IsControl))
                throw new ClientFault("Choose a filename of at most 255 UTF-8 bytes without control characters.");
            using (var stream = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 65536, true))
            {
                long size = stream.Length;
                if (size > settings.MaxUploadBytes) throw new ClientFault("The file exceeds the companion limit. Match Settings to your tested backend limit before sending larger files.");
                FileRow created = Json.Decode<FileRow>(await Request("files", HttpMethod.Post, Body(new CreateBody { Name = name, Size = size }), token, 12).ConfigureAwait(false));
                Guid id;
                if (created == null || !Guid.TryParse(created.Id, out id) || created.Status != "pending") throw new ClientFault("The backend returned invalid file metadata.");
                var attempt = new TransferAttempt { Id = id.ToString(), Path = path, Size = size };
                LastAttempt = attempt;
                using (var multipart = new MultipartFormDataContent())
                using (var file = new ProgressContent(stream, size, progress, token))
                {
                    multipart.Add(file, "file", name);
                    try
                    {
                        FileRow result = Json.Decode<FileRow>(await Request("upload?id=" + id, HttpMethod.Post, multipart, token, 315).ConfigureAwait(false));
                        if (result == null || result.Id != attempt.Id || result.Status != "hashed") throw new ClientFault("The backend has not confirmed a hashed upload. Check the last transfer.");
                    }
                    finally { attempt.Hash = file.Digest; }
                }
                return await Reconcile(token).ConfigureAwait(false);
            }
        }
        public async Task<TransferAttempt> Reconcile(CancellationToken token)
        {
            var attempt = LastAttempt;
            if (attempt == null) throw new ClientFault("No known transfer to check. Open the web interface to inspect recent files.");
            FileRow[] rows = Json.Decode<FileRow[]>(await Request("files", HttpMethod.Get, null, token, 12).ConfigureAwait(false));
            FileRow found = rows == null ? null : rows.FirstOrDefault(x => x != null && x.Id == attempt.Id);
            if (found == null || (found.Status != "hashed" && found.Status != "ready"))
                throw new ClientFault("The recent-file list does not confirm completion. Inspect the web interface; pending or failed records use normal server retention. Nothing was deleted or resent.");
            if (attempt.Hash == null)
            {
                using (var file = new FileStream(attempt.Path, FileMode.Open, FileAccess.Read, FileShare.Read))
                using (var sha = SHA256.Create())
                {
                    byte[] buffer = new byte[65536]; int n; long bytes = 0;
                    while ((n = await file.ReadAsync(buffer, 0, buffer.Length, token).ConfigureAwait(false)) > 0) { sha.TransformBlock(buffer, 0, n, buffer, 0); bytes += n; }
                    sha.TransformFinalBlock(new byte[0], 0, 0);
                    if (bytes != attempt.Size) throw new ClientFault("The local file changed. Open the web interface to inspect the upload.");
                    attempt.Hash = BitConverter.ToString(sha.Hash).Replace("-", "").ToLowerInvariant();
                }
            }
            if (found.Size != attempt.Size || !String.Equals(found.Hash, attempt.Hash, StringComparison.OrdinalIgnoreCase))
                throw new ClientFault("The uploaded size or SHA-256 does not match the local file. Do not share it; investigate using the web interface.");
            attempt.Confirmed = true;
            return attempt;
        }
        public async Task<Link> CreateLink(CancellationToken token)
        {
            if (LastAttempt == null || !LastAttempt.Confirmed) throw new ClientFault("A verified completed upload is required before copying a link.");
            Link link = Json.Decode<Link>(await Request("links", HttpMethod.Post, Body(new LinkBody { Id = LastAttempt.Id }), token, 12).ConfigureAwait(false));
            ValidateLink(link, Origin);
            return link;
        }
        internal static void ValidateLink(Link link, Uri origin)
        {
            Uri uri; DateTimeOffset expiry;
            bool valid = link != null && !String.IsNullOrEmpty(link.Url) && link.Url.Length <= 8192 &&
                !link.Url.Any(char.IsWhiteSpace) && !link.Url.Any(char.IsControl) &&
                Uri.TryCreate(link.Url, UriKind.Absolute, out uri) && uri.Scheme == "https" &&
                uri.UserInfo.Length == 0 && uri.Fragment.Length == 0 && uri.AbsolutePath == "/download" &&
                String.Equals(uri.GetLeftPart(UriPartial.Authority), origin.GetLeftPart(UriPartial.Authority), StringComparison.OrdinalIgnoreCase) &&
                ValidTokenQuery(uri.Query) && DateTimeOffset.TryParse(link.ExpiresAt, out expiry) &&
                expiry > DateTimeOffset.UtcNow && expiry <= DateTimeOffset.UtcNow.AddMinutes(6);
            if (!valid) throw new ClientFault("The backend must return a current HTTPS download link for the exact configured origin. Check its canonical public URL.");
        }
        static bool ValidTokenQuery(string query)
        {
            if (query.Length > 4096 || !query.StartsWith("?token=", StringComparison.Ordinal)) return false;
            string[] parts = query.Substring(7).Split('.');
            return parts.Length == 2 && parts.All(part => part.Length > 0 && part.All(c =>
                (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-'));
        }
        public void Dispose() { http.Dispose(); }
    }
}
