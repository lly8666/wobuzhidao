using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace Wbd.Gui {
    // Executed only on hosted Windows Actions. No driver installation or network apply.
    static class GuiTests {
        static readonly List<string> passed = new List<string>();
        static void Assert(bool condition, string label) { if (!condition) throw new Exception(label); passed.Add(label); }
        static void Populate(Profile p) {
            foreach (var v in new Dictionary<string, object> {
                {"server-ip", "198.51.100.10"}, {"server-name", "www.example.com"}, {"account", "test-account"}, {"username", "test-user"}, {"password", " not-real-password "},
                {"route-key-hex", "112233445566778899aabbccddeeff00"}, {"tunnel-id", "00112233445566778899aabbccddeeff"}, {"installation-id", "11112222333344445555666677778888"}, {"lease4", "10.66.0.7/32"}
            }) p.Values[v.Key] = v.Value;
        }
        sealed class FakeSession : IClientSession {
            public event Action<string> Line;
            public bool Active { get; private set; }
            public readonly List<string> Events = new List<string>();
            public bool FailStop;
            public Task<Dictionary<string, object>> ValidateAsync(string path) { Events.Add("validate"); return Task.FromResult(new Dictionary<string, object>()); }
            public Task StartAsync(string config) { Events.Add("start:" + File.ReadAllText(config)); Active = true; if (Line != null) Line("WBD_WINDOWS_CLIENT_READY"); return Task.FromResult(0); }
            public Task StopAsync() { Events.Add("stop"); if (FailStop) throw new IOException("test cleanup unfinished"); Active = false; return Task.FromResult(0); }
            public void Dispose() { }
        }
        static void Wait(Task task) { while (!task.IsCompleted) { Application.DoEvents(); System.Threading.Thread.Sleep(5); } task.GetAwaiter().GetResult(); }
        static Dictionary<string, object> Wait(Task<Dictionary<string, object>> task) { Wait((Task)task); return task.Result; }
        public static int Run(string root) {
            Application.EnableVisualStyles(); Application.SetCompatibleTextRenderingDefault(false);
            string evidence = Path.Combine(root, "gui-test-result.json");
            try {
                var store = new PortableStore(root);
                Assert(store.Fields.Count >= 36, "CLI catalogue complete including all managed operations");
                Profile p = store.Book.Profiles[0]; Populate(p); store.Save();
                using (var real = new ClientSession(store)) {
                    var mutations = new Dictionary<string, object> {
                        {"server-ip","203.0.113.18"},{"server-port",8443},{"source-port",42000},{"account","another-account"},{"server-name","cdn.example.com"},{"username","another-user"},
                        {"password","different-secret"},{"route-key-hex","aabbccddeeff00112233445566778899"},{"tunnel-id","22223333444455556666777788889999"},{"installation-id","aaaabbbbccccddddeeeeffff00001111"},{"lease4","10.66.0.8/32"},
                        {"mtu",1280},{"client-record-limit",1250},{"adapter","WBD Test"},{"dns-hijack",false},{"dns4","9.9.9.9,1.0.0.1"},{"direct4","203.0.113.0/24"},{"route-mode","bypass-lan"},
                        {"lanes",4},{"fec-parity",20},{"tls-startup-padding",true},{"keepalive-interval","10s"},{"dead-after","120s"},{"reconnect-min","2s"},{"reconnect-max","45s"},{"idle-dormant","15m"},{"rotate-min","30m"},{"rotate-max","60m"}, {"china-ip-file", "data/china-test.txt"}
                    };
                    store.WriteAtomic("data/china-test.txt", "1.0.1.0/24\n");
                    using (var configForm = new MainForm(store, new FakeSession(), () => new DependencyState { Admin=true, Npcap=true, Wintun=true, NpcapVersion="config-test" }))
                    foreach (Field f in store.Fields.Where(f => !f.Managed)) {
                        var saved = new Dictionary<string, object>(p.Values); p.Values[f.Key] = mutations[f.Key];
                        if (f.Key == "rotate-min") p.Values["rotate-max"] = "60m";
                        if (f.Key == "rotate-max") p.Values["rotate-min"] = "30m";
                        configForm.LoadEditor(); configForm.SetEditor(f.Key, mutations[f.Key]); configForm.SaveEditor();
                        Assert(string.Equals(Convert.ToString(p.Values[f.Key]),Convert.ToString(mutations[f.Key]),StringComparison.OrdinalIgnoreCase), "actual GUI field applied: " + f.Key);
                        var effective = store.Effective(p); store.WriteAtomic("data/field-check.json", store.Json.Serialize(effective));
                        var actual = Wait(real.ValidateAsync(store.InRoot("data/field-check.json")));
                        string want = f.Secret ? "configured" : Convert.ToString(effective[f.Key]);
                        if (f.Kind == "Duration") { if (want == "120s") want = "2m0s"; if (want == "15m") want = "15m0s"; if (want == "30m") want = "30m0s"; if (want == "60m") want = "1h0m0s"; }
                        Assert(string.Equals(Convert.ToString(actual[f.Key]), want, StringComparison.OrdinalIgnoreCase), "effective core config: " + f.Key);
                        Assert(Convert.ToString(actual["state-path"]) == store.InRoot("data/network-state.json"), "state path portable for " + f.Key);
                        Assert(Convert.ToString(actual["network-script"]) == store.InRoot("windows_client_network.ps1"), "script path portable for " + f.Key);
                        Assert(Convert.ToString(actual["password"])=="configured" && Convert.ToString(actual["route-key-hex"])=="configured", "validator credentials redacted: " + f.Key);
                        p.Values = saved;
                    }
                    foreach (int fec in new[] {0,4,8,10,12,16,20}) foreach (int lane in new[] {1,2,3,4}) {
                        p.Values["fec-parity"] = fec; p.Values["lanes"] = lane;
                        store.WriteAtomic("data/field-check.json", store.Json.Serialize(store.Effective(p))); var result = Wait(real.ValidateAsync(store.InRoot("data/field-check.json")));
                        Assert(Convert.ToInt32(result["fec-parity"]) == fec && Convert.ToInt32(result["lanes"]) == lane, "FEC/lane selector core acceptance " + fec + "/" + lane);
                    }
                    p.Values["fec-parity"] = 0; p.Values["lanes"] = 1;
                    foreach (var pair in new Dictionary<string, object> { {"lanes",5},{"fec-parity",7},{"mtu",90},{"source-port",65000},{"client-record-limit",30},{"dns4","::1"},{"keepalive-interval","0.1s"},{"dead-after","1s"},{"idle-dormant","-1s"},{"direct4","wrong"},{"route-key-hex","xyz"} }) {
                        object previous = p.Values[pair.Key]; p.Values[pair.Key] = pair.Value; bool denied = false;
                        store.WriteAtomic("data/invalid.json", store.Json.Serialize(store.Effective(p))); try { Wait(real.ValidateAsync(store.InRoot("data/invalid.json"))); } catch { denied = true; }
                        Assert(denied, "invalid config rejected: " + pair.Key); p.Values[pair.Key] = previous;
                    }
                    string good = store.Json.Serialize(store.Effective(p));
                    object originalKey = p.Values["route-key-hex"]; p.Values["route-key-hex"]="00";
                    store.WriteAtomic("data/invalid.json",store.Json.Serialize(store.Effective(p))); bool shortDenied=false;
                    try { Wait(real.ValidateAsync(store.InRoot("data/invalid.json"))); } catch { shortDenied=true; }
                    Assert(shortDenied,"read-only admission validator rejects too-short route key"); p.Values["route-key-hex"]=originalKey;
                    foreach (string text in new[] {good.TrimEnd('}') + ",\"mtu\":1280}",good.TrimEnd('}') + ",\"unknown-gui\":1}",good.TrimEnd('}') + ",\"control-stdin\":true}"}) {
                        store.WriteAtomic("data/invalid.json", text); bool denied = false; try { Wait(real.ValidateAsync(store.InRoot("data/invalid.json"))); } catch { denied = true; } Assert(denied, "strict import rejects duplicate/unknown/management keys");
                    }
                    Assert(!File.Exists(store.InRoot("data/network-state.json")), "read-only validator did not create network state");
                }
                bool traversal = false; try { store.InRoot("../outside.json"); } catch { traversal = true; } Assert(traversal, "outside portable path rejected");
                bool absolute = false; try { store.InRoot(Path.GetTempPath()); } catch { absolute = true; } Assert(absolute, "absolute writable path rejected");
                Assert(ClientSession.Quote("C:\\folder with space\\") == "\"C:\\folder with space\\\\\"", "Windows argument quoting trailing backslash");
                store.Book.DriverNoticeAccepted = true;
                var fake = new FakeSession();
                using (var form = new MainForm(store, fake, () => new DependencyState { Admin = true, Npcap = true, Wintun = true, NpcapVersion = "test environment" })) {
                    form.Show(); form.LoadEditor(); Application.DoEvents(); form.SaveEditor();
                    Assert((string)p.Values["password"] == " not-real-password ", "password whitespace preserved by editor");
                    foreach (var pair in new Dictionary<string, object> {{"server-port",8443},{"mtu",1280},{"tls-startup-padding",true},{"dns-hijack",false},{"route-mode","all"},{"fec-parity",20},{"lanes",4},{"idle-dormant","15m"}}) form.SetEditor(pair.Key, pair.Value);
                    form.SaveEditor(); Assert(Convert.ToString(p.Values["mtu"]) == "1280" && Convert.ToBoolean(p.Values["tls-startup-padding"]), "GUI widgets save typed config");
                    Wait(form.ConnectSelectedAsync()); Assert(fake.Active, "GUI connect starts client");
                    var second = form.DuplicateProfile(); second.Name = "备用服务器"; second.Values["server-ip"] = "203.0.113.19"; form.RefreshProfiles(second.Id);
                    int before = fake.Events.Count; Wait(form.ConnectSelectedAsync());
                    Assert(fake.Events.Skip(before).Select(x => x.Split(':')[0]).SequenceEqual(new[]{"validate","stop","start"}), "server switch validates then cleanup then starts");
                    Assert(fake.Events.Last().Contains("203.0.113.19"), "switched server config passed to child");
                    fake.FailStop = true; bool denied = false; int starts = fake.Events.Count(x => x.StartsWith("start:"));
                    try { Wait(form.ConnectSelectedAsync()); } catch { denied = true; }
                    Assert(denied && fake.Active && fake.Events.Count(x => x.StartsWith("start:")) == starts, "unfinished cleanup preserves old client and blocks replacement");
                    fake.FailStop = false; Wait(form.DisconnectAsync()); Assert(!fake.Active, "GUI disconnect completes");
                    Assert(!form.Redact(" not-real-password ").Contains("not-real-password"), "GUI secret redaction");
                    Wait(form.UpdateChinaAsync());
                    Assert(File.ReadAllLines(store.InRoot("data/china-ipv4.txt")).Length>100 && Convert.ToString(second.Values["china-ip-file"])=="data/china-ipv4.txt", "GUI manual China list update writes and selects portable validated list");
                    form.Capture(store.InRoot("gui-screenshot.png"));
                }
                store.Save(); var reopened = new PortableStore(root); Assert(reopened.Book.Profiles.Count >= 2, "server profiles persist after reopening");
                using (var importForm = new MainForm(reopened, new ClientSession(reopened), () => new DependencyState { Admin=true, Npcap=true, Wintun=true, NpcapVersion="import-test" })) {
                    var values = reopened.Effective(reopened.Book.Profiles[0]); values["china-ip-file"] = reopened.InRoot("data/china-test.txt");
                    reopened.WriteAtomic("data/import-test.json",reopened.Json.Serialize(values)); int count = reopened.Book.Profiles.Count;
                    Wait(importForm.ImportAsync(reopened.InRoot("data/import-test.json")));
                    Assert(reopened.Book.Profiles.Count == count+1 && Convert.ToString(reopened.Book.Profiles.Last().Values["china-ip-file"]).StartsWith("data/china-"),"strict real JSON import copies external list into profile path");
                }
                using (var missing = new MainForm(reopened, new FakeSession(), () => new DependencyState { Admin = true, Npcap = false, Wintun = true })) {
                    bool denied = false; try { Wait(missing.ConnectSelectedAsync()); } catch (InvalidOperationException) { denied = true; } Assert(denied, "Npcap missing blocks connect with onboarding");
                }
                using (var busyOwner = new MainForm(reopened,new FakeSession(),()=>new DependencyState {Admin=true,Npcap=true,Wintun=true,ClientBusy=true})) {
                    bool denied=false;try {Wait(busyOwner.RecoverAsync());} catch(InvalidOperationException){denied=true;}
                    Assert(denied,"crash recovery does not race another client's cleanup");
                }
                foreach (string file in new[] { "data/field-check.json", "data/invalid.json", "data/pending.json", "data/active.json", "data/profiles.json", "data/china-test.txt", "data/import-test.json" }) if (File.Exists(store.InRoot(file))) File.Delete(store.InRoot(file));
                File.WriteAllText(evidence, store.Json.Serialize(new { source_sha=SourceInfo.SHA, result="PASS", checks=passed, physical="NOT_RUN", driver_install="NOT_RUN" }), new System.Text.UTF8Encoding(false));
                return 0;
            } catch (Exception e) {
                File.WriteAllText(evidence, new System.Web.Script.Serialization.JavaScriptSerializer().Serialize(new { source_sha=SourceInfo.SHA,result="FAIL",checks=passed,error=e.ToString() }), new System.Text.UTF8Encoding(false)); return 1;
            }
        }
    }
}
