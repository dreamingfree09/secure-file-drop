using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace SecureFileDrop.Desktop
{
    static class Program
    {
        [STAThread]
        static int Main(string[] args)
        {
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            try
            {
                if (args.Length != 0 && (args.Length != 2 || args[0] != "--send"))
                    throw new ClientFault("Usage: SecureFileDrop.exe [--send \"full file path\"]");
                string file = args.Length == 2 ? WindowsFiles.Validate(args[1]) : "";
                using (var instance = new SingleInstance())
                {
                    if (!instance.Primary) { instance.Forward(file); return 0; }
                    string root = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "SecureFileDrop");
                    using (var context = new TrayContext(root, instance, file)) Application.Run(context);
                }
                return 0;
            }
            catch (ClientFault e) { MessageBox.Show(e.Message, "Secure File Drop", MessageBoxButtons.OK, MessageBoxIcon.Warning); }
            catch (Exception) { MessageBox.Show("The companion could not start. Check file access and the current Windows session. No service was stopped.", "Secure File Drop", MessageBoxButtons.OK, MessageBoxIcon.Error); }
            return 1;
        }
    }
    public sealed class TransferView : Form
    {
        public readonly Label Message = new Label { AutoSize = false, Dock = DockStyle.Top, Height = 72, Padding = new Padding(8) };
        public readonly ProgressBar Progress = new ProgressBar { Dock = DockStyle.Top, Height = 24, Maximum = 1000 };
        public readonly Button CancelTransfer = new Button { Text = "Cancel transfer", AutoSize = true };
        public TransferView()
        {
            Text = "Secure File Drop — Transfer"; Width = 490; Height = 185;
            StartPosition = FormStartPosition.CenterScreen;
            var buttons = new FlowLayoutPanel { Dock = DockStyle.Bottom, Height = 40 };
            buttons.Controls.Add(CancelTransfer); Controls.Add(buttons); Controls.Add(Progress); Controls.Add(Message);
            FormClosing += delegate(object sender, FormClosingEventArgs e) { if (e.CloseReason == CloseReason.UserClosing) { e.Cancel = true; Hide(); } };
        }
    }
    sealed class LoginDialog : Form
    {
        readonly TextBox username = new TextBox { Width = 290, MaxLength = 320 };
        readonly TextBox password = new TextBox { Width = 290, MaxLength = 256, UseSystemPasswordChar = true };
        readonly Button submit = new Button { Text = "Sign in", AutoSize = true };
        readonly Label error = new Label { Width = 390, Height = 65 };
        readonly CancellationTokenSource stop;
        public LoginDialog(SfdClient client, CancellationToken operation)
        {
            stop = CancellationTokenSource.CreateLinkedTokenSource(operation);
            Text = "Sign in to Secure File Drop"; Width = 445; Height = 320;
            FormBorderStyle = FormBorderStyle.FixedDialog; MaximizeBox = false; MinimizeBox = false;
            StartPosition = FormStartPosition.CenterParent;
            var panel = new FlowLayoutPanel { Dock = DockStyle.Fill, FlowDirection = FlowDirection.TopDown, Padding = new Padding(12), WrapContents = false };
            panel.Controls.Add(new Label { Text = client.Origin.GetLeftPart(UriPartial.Authority), Width = 390, Height = 40 });
            panel.Controls.Add(new Label { Text = "Registered username or email", AutoSize = true }); panel.Controls.Add(username);
            panel.Controls.Add(new Label { Text = "Password (never saved)", AutoSize = true }); panel.Controls.Add(password);
            panel.Controls.Add(submit); panel.Controls.Add(error); Controls.Add(panel); AcceptButton = submit;
            submit.Click += async delegate
            {
                submit.Enabled = false; error.Text = "";
                string secret = password.Text; password.Clear();
                try { await client.Login(username.Text, secret, stop.Token); if (!IsDisposed)
                    {
                        if (client.SessionStorageWarning) MessageBox.Show(this, "Signed in for this run. Windows could not save the session securely, so it will not be remembered.", "Secure File Drop", MessageBoxButtons.OK, MessageBoxIcon.Warning);
                        DialogResult = DialogResult.OK; Close();
                    } }
                catch (ClientFault e) { if (!IsDisposed) error.Text = e.Message; }
                catch (OperationCanceledException) { if (!IsDisposed) error.Text = "Sign-in cancelled or timed out."; }
                catch (Exception) { if (!IsDisposed) error.Text = "Sign-in or secure session storage failed. Check access and try again."; }
                finally { secret = null; if (!IsDisposed) submit.Enabled = true; }
            };
            operation.Register(delegate
            {
                try { if (!IsDisposed && IsHandleCreated) BeginInvoke((Action)delegate { Close(); }); }
                catch (InvalidOperationException) { }
            });
            FormClosed += delegate { stop.Cancel(); password.Clear(); };
        }
    }
    sealed class SettingsDialog : Form
    {
        readonly TextBox backend = new TextBox { Width = 400 };
        readonly CheckBox http = new CheckBox { Text = "Allow isolated development HTTP on literal loopback IPs", Width = 420 };
        readonly CheckBox remember = new CheckBox { Text = "Remember this session using Windows protection (no password)", Width = 420 };
        readonly NumericUpDown limit = new NumericUpDown { Minimum = 1, Maximum = 51200, Width = 150 };
        readonly Label error = new Label { Width = 420, Height = 60 };
        public Settings Result { get; private set; }
        public SettingsDialog(Settings current)
        {
            Text = "Secure File Drop — Settings"; Width = 480; Height = 440;
            FormBorderStyle = FormBorderStyle.FixedDialog; MaximizeBox = false; MinimizeBox = false;
            StartPosition = FormStartPosition.CenterScreen;
            backend.Text = current.Backend; http.Checked = current.AllowLoopbackHttp; remember.Checked = current.RememberSession;
            limit.Value = Math.Max(1, Math.Min(51200, current.MaxUploadBytes / 1048576m));
            var panel = new FlowLayoutPanel { Dock = DockStyle.Fill, FlowDirection = FlowDirection.TopDown, Padding = new Padding(12), WrapContents = false };
            panel.Controls.Add(new Label { Text = "Backend HTTPS origin (local or remote)", AutoSize = true }); panel.Controls.Add(backend);
            panel.Controls.Add(http); panel.Controls.Add(remember);
            panel.Controls.Add(new Label { Text = "Companion file limit in MiB (match the server)", AutoSize = true }); panel.Controls.Add(limit);
            panel.Controls.Add(new Label { Text = "Transfers have a five-minute server budget. Raising this limit does not establish large-file capacity.", Width = 420, Height = 40 });
            var save = new Button { Text = "Save settings", AutoSize = true }; panel.Controls.Add(save);
            var install = new Button { Text = "Enable Explorer “Send with Secure File Drop”", AutoSize = true }; panel.Controls.Add(install);
            var remove = new Button { Text = "Remove this companion's Explorer action", AutoSize = true }; panel.Controls.Add(remove);
            panel.Controls.Add(error); Controls.Add(panel);
            save.Click += delegate
            {
                try { var value = new Settings { Backend = backend.Text.Trim(), AllowLoopbackHttp = http.Checked, RememberSession = remember.Checked, MaxUploadBytes = Decimal.ToInt64(limit.Value * 1048576) }; value.Origin(); Result = value; DialogResult = DialogResult.OK; Close(); }
                catch (ClientFault e) { error.Text = e.Message; }
            };
            install.Click += delegate
            {
                try { ExplorerVerb.Install(Application.ExecutablePath); error.Text = "Enabled for this Windows user. On Windows 11, use Show more options. Keep the executable at this location."; }
                catch (Exception) { error.Text = "Explorer registration failed or an existing unrelated entry was preserved."; }
            };
            remove.Click += delegate
            {
                try { ExplorerVerb.Remove(); error.Text = "This companion's owned Explorer action was removed."; }
                catch (Exception) { error.Text = "Explorer removal failed or the entry is not owned by this companion."; }
            };
        }
    }
    public sealed class TrayContext : ApplicationContext
    {
        readonly NotifyIcon tray;
        readonly Control dispatch = new Control();
        readonly TransferView view = new TransferView();
        readonly System.Windows.Forms.Timer timer = new System.Windows.Forms.Timer { Interval = 30000 };
        readonly string root;
        readonly SingleInstance instance;
        readonly ToolStripMenuItem service = new ToolStripMenuItem("Service: checking") { Enabled = false };
        readonly ToolStripMenuItem status = new ToolStripMenuItem("Transfer: idle");
        readonly ToolStripMenuItem copy = new ToolStripMenuItem("Copy Download Link");
        readonly ToolStripMenuItem quick = new ToolStripMenuItem("Quick Send…");
        readonly ToolStripMenuItem settingsMenu = new ToolStripMenuItem("Settings / Explorer integration…");
        readonly ToolStripMenuItem signIn = new ToolStripMenuItem("Sign in…");
        readonly ToolStripMenuItem signOut = new ToolStripMenuItem("Sign out");
        readonly ToolStripMenuItem recover = new ToolStripMenuItem("Check last transfer");
        readonly CancellationTokenSource lifetime = new CancellationTokenSource();
        CancellationTokenSource transfer;
        SfdClient client, probingClient;
        Settings settings;
        Task active, listener;
        bool closing, running, initializing = true;
        int operationNumber;
        string pendingFile;
        string copied;
        DateTime copiedUntil;
        public TrayContext(string directory, SingleInstance single, string initialFile)
        {
            root = directory; instance = single; pendingFile = initialFile; Directory.CreateDirectory(root);
            try { settings = Json.Decode<Settings>(File.ReadAllBytes(Path.Combine(root, "settings.json"))); settings.Origin(); }
            catch (Exception) { settings = new Settings(); }
            client = new SfdClient(settings, new ProtectedSessions(root));
            var handle = dispatch.Handle;
            var menu = new ContextMenuStrip();
            menu.Items.Add(service); menu.Items.Add(status); menu.Items.Add(new ToolStripSeparator());
            menu.Items.Add(quick); menu.Items.Add(copy); menu.Items.Add(recover);
            var open = new ToolStripMenuItem("Open Secure File Drop"); menu.Items.Add(open);
            menu.Items.Add(signIn); menu.Items.Add(signOut); menu.Items.Add(settingsMenu);
            var exit = new ToolStripMenuItem("Exit"); menu.Items.Add(exit);
            tray = new NotifyIcon { Icon = SystemIcons.Shield, Text = "Secure File Drop", ContextMenuStrip = menu, Visible = true };
            quick.Click += delegate { SelectFile(); }; copy.Click += delegate { Run(Copy); };
            recover.Click += delegate { Run(async delegate { await EnsureLogin(); var result = await client.Reconcile(Token); Success(result); }); };
            status.Click += delegate { view.Show(); view.Activate(); };
            open.Click += delegate { OpenWeb(); }; tray.DoubleClick += delegate { view.Show(); view.Activate(); };
            settingsMenu.Click += delegate { Configure(); };
            signIn.Click += delegate { Run(async delegate { await EnsureLogin(); SetStatus("Signed in."); }); };
            signOut.Click += delegate { Run(async delegate { try { await client.Logout(Token); SetStatus("Signed out and session revoked."); } finally { ClearClipboard(); } }); };
            view.CancelTransfer.Click += delegate { if (transfer != null) transfer.Cancel(); };
            exit.Click += async delegate { await ExitCleanly(); };
            timer.Tick += async delegate { await Probe(); }; timer.Start();
            listener = instance.Listen(delegate(string path)
            {
                if (!closing && !dispatch.IsDisposed) dispatch.BeginInvoke((Action)delegate { if (path.Length == 0) { view.Show(); view.Activate(); } else if (initializing) pendingFile = path; else Send(path); });
            });
            dispatch.BeginInvoke((Action)async delegate
            {
                await Probe();
                if (settings.RememberSession)
                    try { await client.Restore(lifetime.Token); } catch (ClientFault) { } catch (OperationCanceledException) { } catch (Exception) { SetStatus("Saved session could not be checked. Sign in again when the service is available."); }
                if (closing) return;
                initializing = false;
                Update();
                if (pendingFile.Length > 0) Send(pendingFile);
                else if (!File.Exists(Path.Combine(root, "settings.json"))) Configure();
                Update();
            });
            Update();
        }
        CancellationToken Token { get { return transfer == null ? lifetime.Token : transfer.Token; } }
        void Update()
        {
            bool busy = running || initializing;
            quick.Enabled = settingsMenu.Enabled = signIn.Enabled = !busy && !closing;
            signOut.Enabled = !busy && !closing && client.Authenticated;
            recover.Enabled = !busy && !closing && client.LastAttempt != null;
            copy.Enabled = !busy && !closing && client.Authenticated && client.LastAttempt != null && client.LastAttempt.Confirmed;
            view.CancelTransfer.Enabled = busy;
        }
        void SetStatus(string text) { status.Text = text.Length > 64 ? text.Substring(0, 64) : text; view.Message.Text = text; }
        void ClearClipboard()
        {
            // Never replace another application's newer clipboard contents.
            try { if (copied != null && Clipboard.ContainsText() && Clipboard.GetText() == copied) Clipboard.Clear(); } catch (Exception) { }
            copied = null;
        }
        async Task Probe()
        {
            if (closing) return;
            SfdClient target = client;
            if (Object.ReferenceEquals(probingClient, target)) return;
            probingClient = target;
            try { bool ready = await target.Probe(lifetime.Token); if (!closing && Object.ReferenceEquals(client, target)) service.Text = ready ? "Service: ready" : "Service: unavailable"; }
            catch (Exception) { if (!closing && Object.ReferenceEquals(client, target)) service.Text = "Service: unavailable"; }
            finally { if (Object.ReferenceEquals(probingClient, target)) probingClient = null; }
            if (copied != null && DateTime.UtcNow >= copiedUntil) ClearClipboard();
        }
        void Run(Func<Task> operation)
        {
            if (closing || running || initializing) { tray.ShowBalloonTip(3000, "Secure File Drop", "A transfer or account operation is active. Try again when it finishes.", ToolTipIcon.Info); return; }
            operationNumber++; running = true; Update(); active = Execute(operation);
        }
        async Task Execute(Func<Task> operation)
        {
            await Task.Yield();
            transfer = CancellationTokenSource.CreateLinkedTokenSource(lifetime.Token);
            try { Token.ThrowIfCancellationRequested(); await operation(); }
            catch (OperationCanceledException) { SetStatus("Cancelled or timed out. The server may still have completed an upload. Check last transfer before retrying."); }
            catch (ClientFault e) { SetStatus(e.Message); if (!client.Authenticated) ClearClipboard(); }
            catch (IOException) { SetStatus("The file or protected settings could not be accessed. Check availability and permissions; check last transfer before retrying."); }
            catch (Exception) { SetStatus("The operation failed. Check service status and the last transfer. No file was deleted or automatically resent."); }
            finally { transfer.Dispose(); transfer = null; running = false; Update(); }
        }
        async Task EnsureLogin()
        {
            if (client.Authenticated) return;
            ClearClipboard();
            using (var dialog = new LoginDialog(client, Token)) if (dialog.ShowDialog(view.Visible ? view : null) != DialogResult.OK) throw new OperationCanceledException();
            await Task.FromResult(0);
        }
        void SelectFile()
        {
            using (var dialog = new OpenFileDialog { Title = "Quick Send — Secure File Drop", Multiselect = false, CheckFileExists = true, RestoreDirectory = true })
                if (dialog.ShowDialog() == DialogResult.OK) Send(dialog.FileName);
        }
        void Send(string path)
        {
            Run(async delegate
            {
                string full = WindowsFiles.Validate(path);
                await EnsureLogin();
                view.Show(); view.Activate(); view.Progress.Value = 0;
                SetStatus("Sending file…");
                DateTime next = DateTime.MinValue;
                int generation = operationNumber;
                CancellationToken operationToken = Token;
                var report = new Progress<Tuple<long, long>>(delegate(Tuple<long, long> value)
                {
                    if (closing || !running || generation != operationNumber || operationToken.IsCancellationRequested || (client.LastAttempt != null && client.LastAttempt.Confirmed)) return;
                    long done = value.Item1, total = value.Item2;
                    if (DateTime.UtcNow < next && done != total) return;
                    next = DateTime.UtcNow.AddMilliseconds(100);
                    view.Progress.Value = total == 0 ? 0 : Math.Min(999, (int)(999m * done / total));
                    SetStatus(done == total ? "Bytes sent; waiting for server hash verification…" : String.Format("Sending: {0:N0} / {1:N0} bytes", done, total));
                });
                TransferAttempt result = await client.Upload(full, delegate(long n, long total) { ((IProgress<Tuple<long, long>>)report).Report(Tuple.Create(n, total)); }, Token);
                Success(result);
            });
        }
        void Success(TransferAttempt result)
        {
            view.Progress.Value = 1000;
            SetStatus("Upload complete: byte count and SHA-256 match. Copy Download Link creates a five-minute HTTPS link.");
            tray.ShowBalloonTip(3000, "Secure File Drop", "Upload verified. Use Copy Download Link to share it.", ToolTipIcon.Info);
        }
        async Task Copy()
        {
            await EnsureLogin(); Link link = await client.CreateLink(Token);
            Clipboard.SetText(link.Url); copied = link.Url; copiedUntil = DateTime.UtcNow.AddSeconds(120);
            SetStatus("Five-minute HTTPS link copied. Its clipboard value will be cleared after two minutes if unchanged.");
            // Signed URLs are intentionally never displayed in notifications, saved or logged.
        }
        void OpenWeb()
        {
            try { Process.Start(new ProcessStartInfo(client.Origin.AbsoluteUri) { UseShellExecute = true }); }
            catch (Exception) { SetStatus("Windows could not open the web interface. Check your default browser."); }
        }
        void Configure()
        {
            if (running || initializing) return;
            using (var dialog = new SettingsDialog(settings))
            {
                if (dialog.ShowDialog() != DialogResult.OK) return;
                Settings next = dialog.Result;
                Run(async delegate
                {
                    ClearClipboard();
                    bool incompleteSignOut = false;
                    try { if (client.Authenticated) await client.Logout(Token); }
                    catch (ClientFault) { incompleteSignOut = true; }
                    client.Dispose();
                    settings = next; client = new SfdClient(settings, new ProtectedSessions(root)); service.Text = "Service: checking";
                    string target = Path.Combine(root, "settings.json"), temp = target + "." + Guid.NewGuid().ToString("N") + ".tmp";
                    try { File.WriteAllBytes(temp, Json.Encode(settings)); if (File.Exists(target)) File.Replace(temp, target, null); else File.Move(temp, target); }
                    finally { if (File.Exists(temp)) File.Delete(temp); }
                    await Probe();
                    SetStatus(incompleteSignOut ? "Settings saved; complete sign-out from the previous service could not be confirmed. Check its web interface and protected-session file permissions." : "Settings saved. Sign in to the configured service before sending.");
                });
            }
        }
        async Task ExitCleanly()
        {
            if (closing) return;
            if (active != null && !active.IsCompleted && MessageBox.Show("Cancel the current operation and exit? An upload may still finish on the server; inspect it before retrying.", "Secure File Drop", MessageBoxButtons.YesNo, MessageBoxIcon.Question) != DialogResult.Yes) return;
            closing = true; Update(); lifetime.Cancel();
            if (transfer != null) transfer.Cancel();
            if (active != null) await active;
            ClearClipboard(); timer.Stop(); tray.Visible = false;
            ExitThread();
        }
        protected override void Dispose(bool disposing)
        {
            if (disposing)
            {
                closing = true; lifetime.Cancel(); timer.Dispose(); tray.Visible = false; tray.Dispose(); view.Dispose(); dispatch.Dispose(); client.Dispose();
            }
            base.Dispose(disposing);
        }
    }
}
