using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace Wbd.Gui {
    public sealed class Choice {
        public string Value, Text;
        public Choice(string value, string text) { Value = value; Text = text; }
        public override string ToString() { return Text; }
    }
    public sealed class MainForm : Form {
        readonly PortableStore store;
        readonly IClientSession session;
        readonly Func<DependencyState> detect;
        readonly ListBox servers = new ListBox();
        readonly TextBox name = new TextBox();
        readonly Dictionary<string, Control> editors = new Dictionary<string, Control>();
        readonly RichTextBox activity = new RichTextBox();
        readonly Label state = new Label(), dependencies = new Label();
        readonly Button connect, stop, save;
        readonly Queue<string> messages = new Queue<string>();
        readonly Timer timer = new Timer { Interval = 250 };
        readonly NotifyIcon tray;
        readonly Control workspace;
        Profile editing;
        string connectedId;
		bool leaseRestart;
        bool busy, updating, exiting;
        public MainForm(PortableStore s, IClientSession client, Func<DependencyState> detector = null) {
            store = s; session = client; detect = detector ?? (() => Dependencies.Detect(store));
            Text = "WBD · 便携客户端"; Font = new Font("Microsoft YaHei UI", 9F);
            ClientSize = new Size(1180, 830); MinimumSize = new Size(1040, 720);
            StartPosition = FormStartPosition.CenterScreen; BackColor = Color.FromArgb(245, 247, 251);
            Icon = SystemIcons.Application; AutoScaleMode = AutoScaleMode.Dpi;
            var root = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, RowCount = 3, Padding = new Padding(22, 16, 22, 14) };
            root.RowStyles.Add(new RowStyle(SizeType.Absolute, 109)); root.RowStyles.Add(new RowStyle(SizeType.Percent, 100)); root.RowStyles.Add(new RowStyle(SizeType.Absolute, 144));
            Controls.Add(root);
            var header = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 3 };
            header.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); header.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 420));
            header.Controls.Add(new Label { Text = "WBD", Font = new Font(Font.FontFamily, 23F, FontStyle.Bold), AutoSize = true, ForeColor = Color.FromArgb(30, 41, 59) }, 0, 0);
            header.Controls.Add(new Label { Text = "弱网连接 · 简单配置 · 文件夹即客户端", AutoSize = true, ForeColor = Color.FromArgb(100, 116, 139) }, 0, 1);
            state.Text = "未连接"; state.AutoSize = true; state.ForeColor = Color.FromArgb(37, 99, 235); state.Padding = new Padding(0, 6, 0, 0);
            header.Controls.Add(state, 0, 2);
            var actions = new FlowLayoutPanel { Dock = DockStyle.Fill, FlowDirection = FlowDirection.RightToLeft, Padding = new Padding(0, 13, 0, 0) };
            connect = Button("连接 / 切换", async () => await RunAsync(ConnectSelectedAsync)); connect.BackColor = Color.FromArgb(37, 99, 235); connect.ForeColor = Color.White;
            stop = Button("断开连接", async () => await RunAsync(DisconnectAsync));
            save = Button("保存配置", () => { Try(() => { SaveEditor(); ShowState("已保存 · 修改在重连后生效"); }); });
            actions.Controls.Add(connect); actions.Controls.Add(stop); actions.Controls.Add(save); header.Controls.Add(actions, 1, 0); header.SetRowSpan(actions, 3); root.Controls.Add(header, 0, 0);
            actions.Controls.Add(Button("校验配置", async () => await RunAsync(ValidateSelectedAsync)));
            var body = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 1 };
            workspace = body;
            body.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 233)); body.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); root.Controls.Add(body, 0, 1);
            var left = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, RowCount = 4, BackColor = Color.White, Padding = new Padding(12), Margin = new Padding(0, 0, 16, 8) };
            left.RowStyles.Add(new RowStyle(SizeType.Absolute, 37)); left.RowStyles.Add(new RowStyle(SizeType.Percent, 100)); left.RowStyles.Add(new RowStyle(SizeType.Absolute, 83)); left.RowStyles.Add(new RowStyle(SizeType.Absolute, 61));
            left.Controls.Add(new Label { Text = "服务器", Font = new Font(Font.FontFamily, 12F, FontStyle.Bold), AutoSize = true, Padding = new Padding(3, 6, 0, 0) }, 0, 0);
            servers.Dock = DockStyle.Fill; servers.BorderStyle = BorderStyle.None; servers.IntegralHeight = false; servers.ItemHeight = 35;
            servers.DrawMode = DrawMode.OwnerDrawFixed;
            servers.DrawItem += DrawServer;
            servers.SelectedIndexChanged += (sender, e) => {
                if (updating) return;
                Try(() => { SaveEditor(); editing = servers.SelectedItem as Profile; if (editing != null) { store.Book.SelectedId = editing.Id; store.Save(); LoadEditor(); } });
            };
            left.Controls.Add(servers, 0, 1);
            var profileActions = new FlowLayoutPanel { Dock = DockStyle.Fill };
            profileActions.Controls.Add(Button("新增", () => Try(() => AddProfile())));
            profileActions.Controls.Add(Button("复制", () => Try(() => DuplicateProfile())));
            profileActions.Controls.Add(Button("删除", () => Try(() => DeleteSelected())));
            left.Controls.Add(profileActions, 0, 2);
            left.Controls.Add(new Label { Text = "选择服务器后点击“连接 / 切换”。\n修改配置不会打断当前连接。", Dock = DockStyle.Fill, ForeColor = Color.DimGray }, 0, 3); body.Controls.Add(left, 0, 0);
            var tabs = new TabControl { Dock = DockStyle.Fill, Padding = new Point(16, 9), Margin = new Padding(0, 0, 0, 8) };
            body.Controls.Add(tabs, 1, 0);
            foreach (string section in new[] { "服务器", "传输", "网络", "生命周期" }) {
                var tab = new TabPage(section) { BackColor = Color.White, Padding = new Padding(16) }; tabs.TabPages.Add(tab);
                var scroll = new Panel { Dock = DockStyle.Fill, AutoScroll = true }; tab.Controls.Add(scroll);
                var fields = new TableLayoutPanel { Dock = DockStyle.Top, AutoSize = true, ColumnCount = 2, Padding = new Padding(0, 0, 16, 12) };
                fields.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 155)); fields.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); scroll.Controls.Add(fields);
                if (section == "服务器") AddRow(fields, "服务器名称", name, "方便识别，例如 香港 · 主用；不影响协议。");
                foreach (Field f in store.Fields.Where(f => f.Section == section)) {
                    Control c = Editor(f); editors.Add(f.Key, c); AddRow(fields, f.Label, c, f.Hint);
                }
                if (section == "服务器") {
                    var bar = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill };
                    bar.Controls.Add(Button("校验配置", async () => await RunAsync(ValidateSelectedAsync)));
                    bar.Controls.Add(Button("导入 JSON", async () => await RunAsync(ImportDialogAsync)));
                    bar.Controls.Add(Button("导出 JSON", () => Try(ExportDialog)));
                    var reveal = new CheckBox { Text = "显示密钥 / 密码", AutoSize = true, Margin = new Padding(8, 9, 0, 0) };
                    reveal.CheckedChanged += (sender, e) => { foreach (Field f in store.Fields.Where(f => f.Secret)) ((TextBox)editors[f.Key]).UseSystemPasswordChar = !reveal.Checked; };
                    bar.Controls.Add(reveal); AddWide(fields, bar);
                }
                if (section == "网络") {
                    var bar = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill };
                    bar.Controls.Add(Button("导入中国 IP 表", () => Try(ImportChinaDialog)));
                    bar.Controls.Add(Button("手动更新 IP 表", async () => await RunAsync(UpdateChinaAsync)));
                    bar.Controls.Add(Button("使用内置 IP 表", () => Try(() => { editors["china-ip-file"].Text = ""; SaveEditor(); })));
                    AddWide(fields, bar); AddWide(fields, Note("IPv6 默认捕获后丢弃，不提供 IPv6 传输。\n保活不计为业务；闲置休眠后真实数据自动唤醒。"));
                }
            }
            var about = new TabPage("便携与驱动") { BackColor = Color.White, Padding = new Padding(22) }; tabs.TabPages.Add(about);
            var details = new FlowLayoutPanel { Dock = DockStyle.Fill, FlowDirection = FlowDirection.TopDown, WrapContents = false, AutoScroll = true };
            about.Controls.Add(details);
            details.Controls.Add(Note("应用文件全部在此文件夹\n配置 data/ · 日志 logs/ · 临时文件 data/tmp/\n不写 AppData，不添加开机启动，不安装应用服务。"));
            details.Controls.Add(Note("驱动是例外\nNpcap 需用户从官网安装，免费版不能随包分发或静默安装。\n本包附带原版 Wintun DLL；首次创建网卡会注册 Windows 驱动。\n系统驱动库 / 网卡 / 路由 / DNS 的修改，不等于应用便携文件。"));
            dependencies.AutoSize = true; dependencies.MaximumSize = new Size(720, 0); dependencies.Margin = new Padding(0, 14, 0, 12); details.Controls.Add(dependencies);
            var driverBar = new FlowLayoutPanel { AutoSize = true };
            driverBar.Controls.Add(Button("Npcap 官方下载", () => Try(Dependencies.OpenDownload)));
            driverBar.Controls.Add(Button("重新检测", () => Try(RefreshDependencies)));
            driverBar.Controls.Add(Button("打开程序文件夹", () => Try(() => Process.Start(new ProcessStartInfo(store.Root) { UseShellExecute = true }))));
            driverBar.Controls.Add(Button("恢复残留网络状态", async () => await RunAsync(RecoverAsync)));
            details.Controls.Add(driverBar);
            details.Controls.Add(Note("新系统：安装 Npcap → 重新检测 → 填入服务端信息 → 校验 → 连接。\n无需安装无线监听支持；使用管理员权限运行本程序。\n切换服务器会等待旧客户端完成 owned-only 清理；不会强杀抢接。\n若异常关机留下路由，可断开后点击“恢复残留网络状态”。"));
            details.Controls.Add(Note("WBD " + SourceInfo.Version + "\n源码 " + SourceInfo.SHA + "\n依赖 Windows 10/11 x64 自带的 .NET Framework 4.8。\n实际物理网卡与驱动收口仍需最终物理验收。"));
            activity.Dock = DockStyle.Fill; activity.ReadOnly = true; activity.BackColor = Color.White; activity.BorderStyle = BorderStyle.None; activity.Font = new Font("Microsoft YaHei UI", 8.5F);
            var logPanel = new TableLayoutPanel { Dock=DockStyle.Fill, ColumnCount=1, RowCount=2, BackColor=Color.White, Padding=new Padding(12,7,12,7) };
            logPanel.RowStyles.Add(new RowStyle(SizeType.Absolute,24)); logPanel.RowStyles.Add(new RowStyle(SizeType.Percent,100));
            logPanel.Controls.Add(new Label { Text="运行信息", AutoSize=true, ForeColor=Color.FromArgb(100,116,139) },0,0); logPanel.Controls.Add(activity,0,1); root.Controls.Add(logPanel,0,2);
            tray = new NotifyIcon { Icon = Icon, Text = "WBD 便携客户端", Visible = true };
            tray.DoubleClick += (sender, e) => { Show(); WindowState = FormWindowState.Normal; Activate(); };
            var menu = new ContextMenuStrip(); menu.Items.Add("显示窗口", null, (sender, e) => { Show(); WindowState = FormWindowState.Normal; Activate(); });
            menu.Items.Add("断开连接", null, async (sender, e) => await RunAsync(DisconnectAsync)); menu.Items.Add("退出", null, (sender, e) => Close()); tray.ContextMenuStrip = menu;
            Resize += (sender, e) => { if (WindowState == FormWindowState.Minimized) Hide(); };
            session.Line += Enqueue;
            timer.Tick += (sender, e) => Drain(); timer.Start();
            FormClosing += async (sender, e) => {
                if (exiting) return; e.Cancel = true;
                if (busy) { Enqueue("当前操作尚未结束，请稍后退出。"); return; }
                await RunAsync(async () => { SaveEditor(); await DisconnectAsync(); exiting = true; Close(); });
            };
            RefreshProfiles(); RefreshDependencies(); Enqueue("欢迎使用 WBD。请先填写服务器信息并校验配置。");
            if (File.Exists(store.InRoot("data/network-state.json"))) Enqueue("检测到上次的网络状态；连接时会处理 owned 状态，也可在便携与驱动页面恢复。");
        }
        static Button Button(string text, Action action) {
            var b = new Button { Text = text, AutoSize = true, Height = 34, MinimumSize = new Size(68, 32), FlatStyle = FlatStyle.Flat, BackColor = Color.White, Margin = new Padding(3, 3, 7, 5), Padding = new Padding(9, 3, 9, 3) };
            b.FlatAppearance.BorderColor = Color.FromArgb(220, 226, 235); b.Click += (s, e) => action(); return b;
        }
        static Label Note(string text) { return new Label { Text = text, AutoSize = true, MaximumSize = new Size(710, 0), Margin = new Padding(0, 10, 0, 16), ForeColor = Color.FromArgb(75, 85, 99) }; }
        static void AddWide(TableLayoutPanel table, Control control) { int row = table.RowCount++; table.Controls.Add(control, 0, row); table.SetColumnSpan(control, 2); }
        static void AddRow(TableLayoutPanel table, string text, Control c, string hint) {
            int row = table.RowCount++; c.Dock = DockStyle.Fill; c.Margin = new Padding(0, 6, 6, 0); c.MinimumSize = new Size(120, 28);
            table.Controls.Add(new Label { Text = text, AutoSize = true, Padding = new Padding(0, 9, 0, 0) }, 0, row); table.Controls.Add(c, 1, row);
            AddWide(table, new Label { Text = hint, AutoSize = true, MaximumSize = new Size(720, 0), ForeColor = Color.FromArgb(113, 125, 142), Font = new Font("Microsoft YaHei UI", 8.2F), Margin = new Padding(155, 4, 6, 13) });
        }
        Control Editor(Field f) {
            if (f.Kind == "Bool") return new CheckBox { Text = "启用", AutoSize = true };
            if (f.Key == "route-mode" || f.Key == "fec-parity" || f.Key == "lanes") {
                var c = new ComboBox { DropDownStyle = ComboBoxStyle.DropDownList };
                if (f.Key == "route-mode") c.Items.AddRange(new[] { new Choice("bypass-lan-cn", "局域网与中国 IPv4 直连"), new Choice("bypass-lan", "仅局域网直连"), new Choice("all", "全部 IPv4 经隧道") });
                if (f.Key == "lanes") c.Items.AddRange(new[] { new Choice("1", "Normal · 单 lane"), new Choice("2", "Game · 2 lanes"), new Choice("3", "Game · 3 lanes"), new Choice("4", "Game · 4 lanes") });
                if (f.Key == "fec-parity") c.Items.AddRange(new[] { new Choice("0", "关闭"), new Choice("4", "20:4"), new Choice("8", "20:8"), new Choice("10", "20:10"), new Choice("12", "20:12"), new Choice("16", "20:16"), new Choice("20", "20:20") });
                return c;
            }
            return new TextBox { UseSystemPasswordChar = f.Secret, MaxLength = f.Key == "direct4" ? 16000 : 4096 };
        }
        void DrawServer(object sender, DrawItemEventArgs e) {
            if (e.Index < 0) return; e.DrawBackground(); var p = (Profile)servers.Items[e.Index];
            string text = (p.Id == connectedId ? "●  " : "   ") + p.Name;
            TextRenderer.DrawText(e.Graphics, text, Font, e.Bounds, e.ForeColor, TextFormatFlags.VerticalCenter | TextFormatFlags.EndEllipsis); e.DrawFocusRectangle();
        }
        public void RefreshProfiles(string id = null) {
            updating = true; servers.Items.Clear(); foreach (var p in store.Book.Profiles) servers.Items.Add(p);
            editing = store.Book.Profiles.FirstOrDefault(p => p.Id == (id ?? store.Book.SelectedId)) ?? store.Book.Profiles[0];
            servers.SelectedItem = editing; store.Book.SelectedId = editing.Id; updating = false; LoadEditor();
        }
        public void LoadEditor() {
            name.Text = editing.Name;
            foreach (Field f in store.Fields.Where(f => !f.Managed)) {
                object v; if (!editing.Values.TryGetValue(f.Key, out v)) v = f.Default;
                SetEditor(f.Key, v);
            }
        }
        public void SetEditor(string key, object value) {
            Control c = editors[key];
            if (c is CheckBox) ((CheckBox)c).Checked = Convert.ToBoolean(value);
            else if (c is ComboBox) { var box = (ComboBox)c; box.SelectedItem = box.Items.Cast<Choice>().FirstOrDefault(x => x.Value == Convert.ToString(value)); }
            else c.Text = Convert.ToString(value);
        }
        public void SaveEditor() {
            if (editing == null) return;
            if (string.IsNullOrWhiteSpace(name.Text) || name.Text.Length > 64) throw new InvalidDataException("服务器名称需要 1–64 个字符。");
            editing.Name = name.Text.Trim();
            foreach (var pair in editors) {
                if (pair.Value is CheckBox) editing.Values[pair.Key] = ((CheckBox)pair.Value).Checked;
                else if (pair.Value is ComboBox) { Choice c = ((ComboBox)pair.Value).SelectedItem as Choice; if (c == null) throw new InvalidDataException("请选择有效的" + store.Fields.First(f => f.Key == pair.Key).Label); editing.Values[pair.Key] = c.Value; }
                else editing.Values[pair.Key] = pair.Key == "password" ? pair.Value.Text : pair.Value.Text.Trim();
            }
            store.Save(); servers.Invalidate();
        }
        public Profile AddProfile() { SaveEditor(); var p = store.NewProfile("新服务器"); RefreshProfiles(p.Id); store.Save(); return p; }
        public Profile DuplicateProfile() { SaveEditor(); var p = store.NewProfile(editing.Name + " · 副本"); p.Values = new Dictionary<string, object>(editing.Values); RefreshProfiles(p.Id); store.Save(); return p; }
        public void DeleteSelected() {
            if (editing.Id == connectedId && session.Active) throw new InvalidOperationException("请先断开这台服务器。");
            if (store.Book.Profiles.Count == 1) throw new InvalidOperationException("请至少保留一台服务器。");
            if (MessageBox.Show(this, "删除“" + editing.Name + "”？", "删除服务器", MessageBoxButtons.OKCancel, MessageBoxIcon.Question) != DialogResult.OK) return;
            store.Book.Profiles.Remove(editing); editing = null; RefreshProfiles(); store.Save();
        }
        string Prepare() { SaveEditor(); store.WriteAtomic("data/pending.json", store.Json.Serialize(store.Effective(editing))); return store.InRoot("data/pending.json"); }
        public async Task ValidateSelectedAsync() { await session.ValidateAsync(Prepare()); ShowState("配置校验通过 · 尚未建立网络连接"); Enqueue("真实客户端已接受当前配置；驱动、服务端连通性在连接时检验。"); }
        public async Task ConnectSelectedAsync() {
            string pending = Prepare(); await session.ValidateAsync(pending);
            DependencyState d = detect(); dependencies.Text = d.Description;
            if (!d.Admin) throw new InvalidOperationException("连接需要管理员权限，请以管理员身份重新打开 WBD.exe。");
            if (!d.Npcap) throw new InvalidOperationException("Npcap 未就绪。请到“便携与驱动”页打开官网下载并安装，再点击重新检测。");
            if (!d.Wintun) throw new InvalidOperationException("缺少 wintun.dll，请重新解压完整便携包。");
            if (!store.Book.DriverNoticeAccepted) {
                Enqueue("首次创建虚拟网卡时，Wintun 将向 Windows 注册官方驱动；配置、日志与临时文件仍在本文件夹。");
                store.Book.DriverNoticeAccepted = true; store.Save();
            }
            await DisconnectAsync();
            if (File.Exists(store.InRoot("data/network-state.json"))) await RecoverAsync();
            // The candidate stays separate until old cleanup has really completed.
            store.WriteAtomic("data/active.json", File.ReadAllText(pending, Encoding.UTF8));
            connectedId = editing.Id; ShowState("正在建立连接 · " + editing.Name); servers.Invalidate();
            await session.StartAsync(store.InRoot("data/active.json"));
        }
        public async Task DisconnectAsync() { leaseRestart = false; if (session.Active) ShowState("正在断开并清理网络状态…"); await session.StopAsync(); Drain(); leaseRestart = false; connectedId = null; ShowState("未连接"); servers.Invalidate(); }
        async Task ImportDialogAsync() {
            using (var dialog = new OpenFileDialog { Filter = "客户端配置 (*.json)|*.json", Title = "导入客户端 JSON 配置", RestoreDirectory = true }) if (dialog.ShowDialog(this) == DialogResult.OK) await ImportAsync(dialog.FileName);
        }
        public async Task ImportAsync(string path) {
            string raw = PortableStore.ReadBounded(path);
            // Let the strict Go parser reject duplicate/unknown keys before deserializing.
            await session.ValidateAsync(path);
            var values = store.Json.Deserialize<Dictionary<string, object>>(raw);
            SaveEditor(); Profile p = store.NewProfile(Path.GetFileNameWithoutExtension(path));
            try { store.ImportValues(p, values); store.Effective(p); RefreshProfiles(p.Id); store.Save(); }
            catch { store.Book.Profiles.Remove(p); throw; }
            Enqueue("已导入服务器配置，文件路径已转换为便携目录内路径。");
        }
        void ExportDialog() {
            SaveEditor();
            if (MessageBox.Show(this, "导出的配置包含密码和路由密钥，请妥善保管。\n配置将写入本程序 data/exports/，可自行复制。", "导出配置", MessageBoxButtons.OKCancel, MessageBoxIcon.Information) != DialogResult.OK) return;
            string dir = store.InRoot("data/exports"); Directory.CreateDirectory(dir);
            string rel = "data/exports/server-" + editing.Id + ".json"; store.WriteAtomic(rel, store.Json.Serialize(store.Effective(editing))); Enqueue("已导出到 " + rel);
        }
        void ImportChinaDialog() {
            using (var d = new OpenFileDialog { Filter = "IPv4 CIDR 列表 (*.txt)|*.txt|所有文件 (*.*)|*.*", RestoreDirectory = true }) if (d.ShowDialog(this) == DialogResult.OK) {
                string rel = "data/china-" + editing.Id + ".txt", text = PortableStore.ReadBounded(d.FileName);
                store.WriteAtomic(rel, text); editors["china-ip-file"].Text = rel; SaveEditor(); Enqueue("IP 表已复制进程序目录；请校验并重连应用。");
            }
        }
        public async Task UpdateChinaAsync() {
            string target = store.InRoot("data/china-ipv4.txt");
            var info = new ProcessStartInfo(store.InRoot("wbd-client.exe"), "--update-china-ip " + ClientSession.Quote(target)) { UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = store.Root, RedirectStandardOutput = true, RedirectStandardError = true };
            info.EnvironmentVariables["TEMP"] = store.InRoot("data/tmp"); info.EnvironmentVariables["TMP"] = store.InRoot("data/tmp"); info.EnvironmentVariables.Remove("WBD_QUALIFICATION_CPU_PROFILE");
            using (var p = Process.Start(info)) {
                Task<string> err = p.StandardError.ReadToEndAsync(), output = p.StandardOutput.ReadToEndAsync();
                await Task.Run(() => { if (!p.WaitForExit(45000)) { p.Kill(); throw new IOException("更新超时；已保留原 IP 表。"); } });
                if (p.ExitCode != 0) throw new IOException("更新失败；已保留原 IP 表。" + await err); await output;
            }
            editors["china-ip-file"].Text = "data/china-ipv4.txt"; SaveEditor(); Enqueue("中国 IPv4 列表已校验并更新，重连后生效。");
        }
        public async Task RecoverAsync() {
            if (session.Active) throw new InvalidOperationException("请先断开连接。");
            if (detect().ClientBusy) throw new InvalidOperationException("另一客户端仍在运行或清理，请等它退出后再恢复网络状态。");
            string path = store.InRoot("data/network-state.json");
            if (!File.Exists(path)) { Enqueue("没有残留网络状态。"); return; }
            if (!detect().Admin) throw new InvalidOperationException("恢复网络状态需要管理员权限。");
            // Cleanup validates and consumes only the owned JSON state; no profile guessing.
            string script = store.InRoot("windows_client_network.ps1");
            var info = new ProcessStartInfo("powershell.exe", "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File " + ClientSession.Quote(script) + " -Action Cleanup -StatePath " + ClientSession.Quote(path)) { UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = store.Root, RedirectStandardError = true, RedirectStandardOutput = true };
            info.EnvironmentVariables["TEMP"] = store.InRoot("data/tmp"); info.EnvironmentVariables["TMP"] = store.InRoot("data/tmp");
            using (var p = Process.Start(info)) {
                Task<string> error = p.StandardError.ReadToEndAsync(), output = p.StandardOutput.ReadToEndAsync();
                await Task.Run(() => p.WaitForExit());
                if (p.ExitCode != 0) throw new IOException("状态恢复失败：" + await error); await output;
            }
            Enqueue("本程序创建的残留路由、DNS 和 IPv6 策略已清理。");
        }
        void RefreshDependencies() { dependencies.Text = detect().Description; }
        async Task RunAsync(Func<Task> action) {
            if (busy) return; busy = true; connect.Enabled = stop.Enabled = save.Enabled = workspace.Enabled = false;
            try { await action(); } catch (Exception e) { ShowError(e); }
            finally { busy = false; connect.Enabled = stop.Enabled = save.Enabled = workspace.Enabled = true; }
        }
        void Try(Action action) { try { action(); } catch (Exception e) { ShowError(e); } }
        void ShowError(Exception e) { string text = Redact(e.Message); Enqueue(text); MessageBox.Show(this, text, "WBD", MessageBoxButtons.OK, MessageBoxIcon.Information); }
        public string Redact(string text) {
            foreach (Profile p in store.Book.Profiles) foreach (string k in new[] { "password", "route-key-hex" }) { object v; if (p.Values.TryGetValue(k, out v) && !string.IsNullOrEmpty(Convert.ToString(v))) text = text.Replace(Convert.ToString(v), "[已隐藏]"); }
            return new string(text.Take(2000).Where(c => !char.IsControl(c) || c == '\t').ToArray());
        }
        void Enqueue(string line) { lock (messages) { if (messages.Count >= 256) messages.Dequeue(); messages.Enqueue(line); } }
        void Drain() {
            for (int i = 0; i < 32; i++) {
                string line; lock (messages) { if (messages.Count == 0) break; line = messages.Dequeue(); }
				if (line.Contains("WBD_CLIENT_LEASE_CHANGED")) leaseRestart = true;
                if (line == "WBD_WINDOWS_CLIENT_READY") { if (connectedId!=null && session.Active) ShowState("已连接 · " + (store.Book.Profiles.FirstOrDefault(p => p.Id == connectedId)?.Name ?? "服务器")); line = "隧道已就绪，网络配置已应用。"; }
                line = Redact(line); store.AppendLog(line); activity.AppendText(DateTime.Now.ToString("HH:mm:ss ") + line + Environment.NewLine);
                if (activity.Lines.Length > 400) activity.Lines = activity.Lines.Skip(activity.Lines.Length - 300).ToArray(); activity.SelectionStart = activity.TextLength; activity.ScrollToCaret();
            }
            if (!busy && connectedId != null && !session.Active) {
				if (leaseRestart && !exiting) {
					leaseRestart=false; BeginInvoke(new Action(async () => await RunAsync(RestartAssignedLeaseAsync)));
				} else { connectedId = null; ShowState("连接已结束 · 请查看运行信息"); servers.Invalidate(); }
			}
        }
		async Task RestartAssignedLeaseAsync() {
			string id=connectedId; if (id==null || exiting) return;
			// Use the last active file, not unsaved edits or a newly selected profile.
			string config=store.InRoot("data/active.json");
			await session.StopAsync();
			if (File.Exists(store.InRoot("data/network-state.json"))) await RecoverAsync();
			await session.ValidateAsync(config);
			ShowState("服务端地址已更新 · 正在重新连接"); connectedId=id;
			await session.StartAsync(config);
		}
        void ShowState(string text) { state.Text = text; }
        public void Capture(string path) { using (var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(0, 0, Width, Height)); bitmap.Save(path); } }
        protected override void Dispose(bool disposing) { if (disposing) { timer.Stop(); timer.Dispose(); session.Line -= Enqueue; session.Dispose(); tray.Visible = false; tray.Dispose(); } base.Dispose(disposing); }
    }
    static class Program {
        [STAThread] static int Main(string[] args) {
            string root = AppDomain.CurrentDomain.BaseDirectory;
            try {
                if (args.Length == 1 && args[0] == "--version") { Console.WriteLine("WBD GUI " + SourceInfo.Version + " " + SourceInfo.SHA); return 0; }
                if (args.Length > 0 && args[0] == "--self-test") return GuiTests.Run(root);
                bool first; using (var singleton = new System.Threading.Mutex(true, "Local\\WBD-Portable-GUI", out first)) {
                    if (!first) { MessageBox.Show("WBD 界面已经运行，请从系统托盘打开。", "WBD"); return 0; }
                    Application.EnableVisualStyles(); Application.SetCompatibleTextRenderingDefault(false);
                    var store = new PortableStore(root); Application.Run(new MainForm(store, new ClientSession(store))); return 0;
                }
            } catch (Exception e) { MessageBox.Show("无法启动：" + e.Message + "\n请完整解压到可写的本地文件夹；不要直接在压缩包里运行。", "WBD", MessageBoxButtons.OK, MessageBoxIcon.Error); return 1; }
        }
    }
}
