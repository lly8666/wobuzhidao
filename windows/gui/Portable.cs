using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Security.AccessControl;
using System.Security.Principal;
using System.ServiceProcess;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;

namespace Wbd.Gui {
    public sealed class Profile {
        public string Id { get; set; }
        public string Name { get; set; }
        public Dictionary<string, object> Values { get; set; }
        public override string ToString() { return Name; }
    }
    public sealed class ProfileBook {
        public int Schema { get; set; } = 1;
        public string SelectedId { get; set; }
        public bool DriverNoticeAccepted { get; set; }
        public List<Profile> Profiles { get; set; } = new List<Profile>();
    }
    public sealed class Field {
        public string Key, Kind, Section, Label, Hint;
        public object Default;
        public bool Managed { get { return Section == "操作" || Section == "便携"; } }
        public bool Secret { get { return Key == "password" || Key == "route-key-hex"; } }
    }
    public sealed class PortableStore {
        public readonly string Root;
        public readonly List<Field> Fields = new List<Field>();
        public readonly JavaScriptSerializer Json = new JavaScriptSerializer { MaxJsonLength = 1048576, RecursionLimit = 12 };
        public ProfileBook Book;
        public PortableStore(string root) {
            Root = Path.GetFullPath(root);
            // Reject redirected writable locations rather than silently leaving the folder.
            CheckPath(Root);
            Directory.CreateDirectory(InRoot("data"));
            SecureDirectory(InRoot("data"));
            Directory.CreateDirectory(InRoot("logs"));
            SecureDirectory(InRoot("logs"));
            Directory.CreateDirectory(InRoot("data/tmp"));
            var catalog = Json.Deserialize<Dictionary<string, object>>(ReadBounded(InRoot("PARAMETERS.json")));
            var targets = (Dictionary<string, object>)catalog["targets"];
            var cli = (Dictionary<string, object>)targets["client-windows"];
            var labels = Json.Deserialize<Dictionary<string, string[]>>(ReadBounded(InRoot("gui-fields.json")));
            if (!new HashSet<string>(cli.Keys).SetEquals(labels.Keys)) throw new InvalidDataException("GUI 参数清单与客户端不一致，请重新下载完整版本。");
            foreach (var pair in labels) {
                var meta = (Dictionary<string, object>)cli[pair.Key];
                string[] label = pair.Value;
                Fields.Add(new Field { Key = pair.Key, Kind = (string)meta["type"], Section = label[0], Label = label[1], Hint = label[2], Default = ParseDefault((string)meta["default_expression"]) });
            }
            string file = InRoot("data/profiles.json");
            Book = File.Exists(file) ? Json.Deserialize<ProfileBook>(ReadBounded(file)) : new ProfileBook();
            if (Book == null || Book.Schema != 1 || Book.Profiles == null || Book.Profiles.Count > 100) throw new InvalidDataException("服务器列表格式无效；原文件已保留。");
            var ids = new HashSet<string>();
            foreach (Profile p in Book.Profiles) {
                if (p == null || p.Id == null || !System.Text.RegularExpressions.Regex.IsMatch(p.Id,"^[0-9a-f]{32}$") || !ids.Add(p.Id) || p.Values == null || string.IsNullOrWhiteSpace(p.Name) || p.Name.Length>64) throw new InvalidDataException("服务器列表条目无效；原文件已保留。");
                if (p.Values.Keys.Any(k => !Fields.Any(f => f.Key == k && !f.Managed))) throw new InvalidDataException("服务器包含未知配置，请导入正式客户端配置。");
            }
            if (Book.Profiles.Count == 0) { Profile p = NewProfile("我的服务器"); Book.SelectedId = p.Id; Save(); }
        }
        public string InRoot(string relative) {
            if (Path.IsPathRooted(relative)) throw new InvalidDataException("路径必须位于程序目录内。");
            string full = Path.GetFullPath(Path.Combine(Root, relative));
            if (!full.StartsWith(Root.TrimEnd(Path.DirectorySeparatorChar) + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new InvalidDataException("路径不能离开程序目录。");
            CheckPath(full);
            return full;
        }
        static void CheckPath(string path) {
            for (var p = new DirectoryInfo(Path.GetDirectoryName(path) ?? path); p != null; p = p.Parent)
                if (p.Exists && (p.Attributes & FileAttributes.ReparsePoint) != 0) throw new InvalidDataException("便携目录不能经链接指向其他位置。");
            if ((File.Exists(path) || Directory.Exists(path)) && (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0) throw new InvalidDataException("便携路径不能是链接。");
        }
        static void SecureDirectory(string path) {
            var acl = new DirectorySecurity();
            acl.SetAccessRuleProtection(true, false);
            var inheritance = InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit;
            foreach (SecurityIdentifier sid in new[] { WindowsIdentity.GetCurrent().User, new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null), new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null) })
                acl.AddAccessRule(new FileSystemAccessRule(sid, FileSystemRights.FullControl, inheritance, PropagationFlags.None, AccessControlType.Allow));
            new DirectoryInfo(path).SetAccessControl(acl);
        }
        public static string ReadBounded(string path) {
            if (new FileInfo(path).Length > 1048576) throw new InvalidDataException("文件超过 1 MiB。");
            return File.ReadAllText(path, Encoding.UTF8);
        }
        object ParseDefault(string expression) {
            switch (expression) {
                case "runtimeentry.DefaultKeepaliveInterval": return "15s";
                case "runtimeentry.DefaultDeadAfter": return "90s";
                case "runtimeentry.DefaultReconnectMin": return "1s";
                case "runtimeentry.DefaultReconnectMax": return "30s";
            }
            if (expression == "0") return 0;
            try { return Json.DeserializeObject(expression); }
            catch { throw new InvalidDataException("客户端出现未登记的默认表达式：" + expression); }
        }
        public Profile NewProfile(string name) {
            if (Book.Profiles.Count >= 100) throw new InvalidDataException("最多保存 100 台服务器。");
            var p = new Profile { Id = Guid.NewGuid().ToString("N"), Name = name, Values = new Dictionary<string, object>() };
            foreach (Field f in Fields.Where(f => !f.Managed)) p.Values[f.Key] = f.Kind == "Duration" && Convert.ToString(f.Default) == "0" ? "0s" : f.Default;
            p.Values["installation-id"] = Guid.NewGuid().ToString("N");
            Book.Profiles.Add(p); return p;
        }
        public void Save() { WriteAtomic("data/profiles.json", Json.Serialize(Book)); }
        public void WriteAtomic(string relative, string text) {
            if (Encoding.UTF8.GetByteCount(text) > 1048576) throw new InvalidDataException("配置超过 1 MiB。");
            string target = InRoot(relative), temp = InRoot(relative + ".new");
            File.WriteAllText(temp, text, new UTF8Encoding(false));
            if (File.Exists(target)) File.Replace(temp, target, null); else File.Move(temp, target);
        }
        public Dictionary<string, object> Effective(Profile p) {
            var values = new Dictionary<string, object>();
            foreach (Field f in Fields.Where(f => !f.Managed)) {
                object value; if (!p.Values.TryGetValue(f.Key, out value)) value = f.Default;
                if (f.Kind == "Bool") value = Convert.ToBoolean(value);
                else if (f.Kind == "Int") value = Convert.ToInt32(value);
                else if (f.Kind == "Uint") value = Convert.ToUInt32(value);
                else value = Convert.ToString(value);
                values.Add(f.Key, value);
            }
            string china = (string)values["china-ip-file"];
            if (china.Length > 0) values["china-ip-file"] = InRoot(china);
            values["state-path"] = InRoot("data/network-state.json");
            values["network-script"] = InRoot("windows_client_network.ps1");
            return values;
        }
        public void ImportValues(Profile p, Dictionary<string, object> values) {
            if (values.Keys.Any(k => !Fields.Any(f => f.Key == k && f.Section != "操作"))) throw new InvalidDataException("配置含未知参数。");
            foreach (Field f in Fields.Where(f => !f.Managed)) if (values.ContainsKey(f.Key)) p.Values[f.Key] = values[f.Key];
            // External lists are copied, never become runtime writable destinations.
            string china = Convert.ToString(p.Values["china-ip-file"]);
            if (china.Length > 0 && Path.IsPathRooted(china)) {
                string text = ReadBounded(china); string rel = "data/china-" + p.Id + ".txt";
                WriteAtomic(rel, text); p.Values["china-ip-file"] = rel;
            }
        }
        public void AppendLog(string line) {
            string file = InRoot("logs/client.log");
            if (File.Exists(file) && new FileInfo(file).Length > 2097152) { string old = InRoot("logs/client.previous.log"); if (File.Exists(old)) File.Delete(old); File.Move(file, old); }
            File.AppendAllText(file, DateTime.Now.ToString("HH:mm:ss ") + line + Environment.NewLine, new UTF8Encoding(false));
        }
    }
    public interface IClientSession : IDisposable {
        event Action<string> Line;
        bool Active { get; }
        Task<Dictionary<string, object>> ValidateAsync(string config);
        Task StartAsync(string config);
        Task StopAsync();
    }
    public sealed class ClientSession : IClientSession {
        readonly PortableStore store;
        readonly SemaphoreSlim gate = new SemaphoreSlim(1, 1);
        Process process;
        public event Action<string> Line;
        public bool Active { get { try { return process != null && !process.HasExited; } catch (InvalidOperationException) { return false; } } }
        public ClientSession(PortableStore s) { store = s; }
        ProcessStartInfo Info(string args) {
            var info = new ProcessStartInfo(store.InRoot("wbd-client.exe"), args) { WorkingDirectory = store.Root, UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true, RedirectStandardError = true, RedirectStandardInput = true };
            info.StandardOutputEncoding = info.StandardErrorEncoding = new UTF8Encoding(false);
            info.EnvironmentVariables["TEMP"] = store.InRoot("data/tmp");
            info.EnvironmentVariables["TMP"] = store.InRoot("data/tmp");
            // Do not accidentally activate a developer profile writer outside this folder.
            info.EnvironmentVariables.Remove("WBD_QUALIFICATION_CPU_PROFILE");
            return info;
        }
        public static string Quote(string value) {
            // CommandLineToArgvW quoting, including backslashes before the closing quote.
            var b = new StringBuilder("\""); int slash = 0;
            foreach (char c in value) {
                if (c == '\\') { slash++; continue; }
                if (c == '"') { b.Append('\\', slash * 2 + 1); b.Append(c); slash = 0; continue; }
                b.Append('\\', slash); slash = 0; b.Append(c);
            }
            b.Append('\\', slash * 2); return b.Append('"').ToString();
        }
        public async Task<Dictionary<string, object>> ValidateAsync(string config) {
            using (var p = Process.Start(Info("--check-config --config " + Quote(config)))) {
                Task<string> output = p.StandardOutput.ReadToEndAsync(), error = p.StandardError.ReadToEndAsync();
                await Task.Run(() => { if (!p.WaitForExit(20000)) { p.Kill(); throw new InvalidOperationException("配置校验超时。"); } });
                string text = await output, failure = await error;
                if (p.ExitCode != 0) throw new InvalidDataException("客户端拒绝配置：" + failure.Trim());
                return store.Json.Deserialize<Dictionary<string, object>>(text);
            }
        }
        public async Task StartAsync(string config) {
            await gate.WaitAsync();
            try {
                if (Active) throw new InvalidOperationException("旧客户端仍在运行，请先断开。");
                if (process != null) { process.Dispose(); process = null; }
                process = new Process { StartInfo = Info("--control-stdin --config " + Quote(config)), EnableRaisingEvents = true };
                process.OutputDataReceived += (s, e) => { if (e.Data != null && Line != null) Line(e.Data); };
                process.ErrorDataReceived += (s, e) => { if (e.Data != null && Line != null) Line(e.Data); };
                try { process.Start(); process.BeginOutputReadLine(); process.BeginErrorReadLine(); }
                catch { process.Dispose(); process = null; throw; }
            } finally { gate.Release(); }
        }
        public async Task StopAsync() {
            await gate.WaitAsync();
            try {
                if (process == null) return;
                if (!process.HasExited) {
                    try { await process.StandardInput.WriteLineAsync("stop"); process.StandardInput.Close(); } catch (IOException) { }
                    bool ended = await Task.Run(() => process.WaitForExit(30000));
                    if (!ended) throw new InvalidOperationException("清理仍在进行，已保留原客户端。请稍后重试断开；不会强杀后直接切换服务器。");
                }
                process.WaitForExit(); process.Dispose(); process = null;
            } finally { gate.Release(); }
        }
        public void Dispose() { if (process != null) { try { process.StandardInput.Close(); } catch { } process.Dispose(); process = null; } gate.Dispose(); }
    }
    public sealed class DependencyState {
        public bool Npcap, Wintun, Admin;
        public string NpcapVersion;
        public string Description { get { return "Npcap：" + (Npcap ? NpcapVersion : "未安装或不可加载") + "  ·  Wintun DLL：" + (Wintun ? "已随包提供" : "缺失") + "  ·  权限：" + (Admin ? "管理员" : "普通用户"); } }
    }
    public static class Dependencies {
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)] static extern IntPtr LoadLibraryEx(string name, IntPtr file, uint flags);
        [DllImport("kernel32.dll")] static extern bool FreeLibrary(IntPtr handle);
        [DllImport("kernel32.dll", CharSet = CharSet.Ansi)] static extern IntPtr GetProcAddress(IntPtr handle, string name);
        [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate IntPtr PcapVersion();
        public const string Download = "https://npcap.com/#download";
        public static DependencyState Detect(PortableStore store) {
            var d = new DependencyState { Wintun = File.Exists(store.InRoot("wintun.dll")), Admin = new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator) };
            string dll = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), "Npcap", "wpcap.dll");
            IntPtr lib = File.Exists(dll) ? LoadLibraryEx(dll, IntPtr.Zero, 0x900) : IntPtr.Zero;
            if (lib != IntPtr.Zero) {
                try {
                    IntPtr function = GetProcAddress(lib, "pcap_lib_version");
                    bool service = false; using (var s = new ServiceController("npcap")) { try { var status = s.Status; service = true; } catch (InvalidOperationException) { } }
                    if (function != IntPtr.Zero && service) { d.Npcap = true; d.NpcapVersion = Marshal.PtrToStringAnsi(((PcapVersion)Marshal.GetDelegateForFunctionPointer(function, typeof(PcapVersion)))()); }
                } finally { FreeLibrary(lib); }
            }
            return d;
        }
        public static void OpenDownload() { Process.Start(new ProcessStartInfo(Download) { UseShellExecute = true }); }
    }
}
