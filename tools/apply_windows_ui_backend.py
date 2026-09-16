from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one occurrence, got {count}: {old[:80]!r}")
    p.write_text(text.replace(old, new), encoding="utf-8")

profile_store = r'''package windowsgui

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

const ProfileStoreVersion = 1

type SavedProfile struct {
    ID     string             `json:"id"`
    Name   string             `json:"name"`
    Config RuntimeProfileFile `json:"config"`
}

type ProfileStore struct {
    Version    int            `json:"version"`
    SelectedID string         `json:"selected_id,omitempty"`
    Profiles   []SavedProfile `json:"profiles"`
}

func NewProfileStore() ProfileStore { return ProfileStore{Version: ProfileStoreVersion, Profiles: []SavedProfile{}} }

func newProfileID() (string, error) {
    var b [16]byte
    if _, err := rand.Read(b[:]); err != nil { return "", err }
    return hex.EncodeToString(b[:]), nil
}

func (s *ProfileStore) normalize() error {
    if s.Version == 0 { s.Version = ProfileStoreVersion }
    if s.Version != ProfileStoreVersion { return fmt.Errorf("unsupported profile store version %d", s.Version) }
    seen := map[string]bool{}
    for i := range s.Profiles {
        s.Profiles[i].ID = strings.TrimSpace(s.Profiles[i].ID)
        s.Profiles[i].Name = strings.TrimSpace(s.Profiles[i].Name)
        if s.Profiles[i].ID == "" { return fmt.Errorf("profile %d has empty id", i) }
        if s.Profiles[i].Name == "" { return fmt.Errorf("profile %s has empty name", s.Profiles[i].ID) }
        if seen[s.Profiles[i].ID] { return fmt.Errorf("duplicate profile id %s", s.Profiles[i].ID) }
        seen[s.Profiles[i].ID] = true
    }
    if s.SelectedID != "" && !seen[s.SelectedID] { return fmt.Errorf("selected profile %s does not exist", s.SelectedID) }
    if s.SelectedID == "" && len(s.Profiles) > 0 { s.SelectedID = s.Profiles[0].ID }
    return nil
}

func LoadProfileStore(path string) (ProfileStore, error) {
    f, err := os.Open(path)
    if errors.Is(err, os.ErrNotExist) { return NewProfileStore(), nil }
    if err != nil { return ProfileStore{}, err }
    defer f.Close()
    dec := json.NewDecoder(f)
    dec.DisallowUnknownFields()
    var store ProfileStore
    if err := dec.Decode(&store); err != nil { return ProfileStore{}, fmt.Errorf("decode profile store: %w", err) }
    var extra any
    if err := dec.Decode(&extra); err != io.EOF {
        if err == nil { return ProfileStore{}, errors.New("profile store must contain exactly one JSON object") }
        return ProfileStore{}, fmt.Errorf("decode profile store trailer: %w", err)
    }
    if err := store.normalize(); err != nil { return ProfileStore{}, err }
    return store, nil
}

func SaveProfileStore(path string, store ProfileStore) error {
    if strings.TrimSpace(path) == "" { return errors.New("profile store path is required") }
    if err := store.normalize(); err != nil { return err }
    if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { return err }
    data, err := json.MarshalIndent(store, "", "  ")
    if err != nil { return err }
    data = append(data, '\n')
    tmp, err := os.CreateTemp(filepath.Dir(path), ".profiles-*.json")
    if err != nil { return err }
    tmpName := tmp.Name()
    defer os.Remove(tmpName)
    if err := tmp.Chmod(0o600); err != nil { tmp.Close(); return err }
    if _, err := tmp.Write(data); err != nil { tmp.Close(); return err }
    if err := tmp.Sync(); err != nil { tmp.Close(); return err }
    if err := tmp.Close(); err != nil { return err }
    if err := os.Rename(tmpName, path); err != nil { return err }
    return nil
}

func (s *ProfileStore) Add(name string, cfg RuntimeProfileFile) (SavedProfile, error) {
    id, err := newProfileID(); if err != nil { return SavedProfile{}, err }
    p := SavedProfile{ID: id, Name: strings.TrimSpace(name), Config: cfg}
    if p.Name == "" { p.Name = "新服务器" }
    s.Profiles = append(s.Profiles, p)
    if s.SelectedID == "" { s.SelectedID = id }
    return p, nil
}

func (s *ProfileStore) Upsert(p SavedProfile) error {
    p.ID = strings.TrimSpace(p.ID); p.Name = strings.TrimSpace(p.Name)
    if p.ID == "" || p.Name == "" { return errors.New("profile id and name are required") }
    for i := range s.Profiles {
        if s.Profiles[i].ID == p.ID { s.Profiles[i] = p; return nil }
    }
    s.Profiles = append(s.Profiles, p)
    if s.SelectedID == "" { s.SelectedID = p.ID }
    return nil
}

func (s *ProfileStore) Delete(id string) bool {
    for i := range s.Profiles {
        if s.Profiles[i].ID != id { continue }
        s.Profiles = append(s.Profiles[:i], s.Profiles[i+1:]...)
        if s.SelectedID == id {
            s.SelectedID = ""
            if len(s.Profiles) > 0 { s.SelectedID = s.Profiles[0].ID }
        }
        return true
    }
    return false
}

func (s *ProfileStore) Select(id string) bool {
    for _, p := range s.Profiles { if p.ID == id { s.SelectedID = id; return true } }
    return false
}

func (s ProfileStore) Selected() (SavedProfile, bool) {
    for _, p := range s.Profiles { if p.ID == s.SelectedID { return p, true } }
    return SavedProfile{}, false
}

func (s ProfileStore) Find(id string) (SavedProfile, bool) {
    for _, p := range s.Profiles { if p.ID == id { return p, true } }
    return SavedProfile{}, false
}

func ReadRuntimeProfileFile(path string) (RuntimeProfileFile, error) {
    f, err := os.Open(path); if err != nil { return RuntimeProfileFile{}, err }
    defer f.Close()
    dec := json.NewDecoder(f); dec.DisallowUnknownFields()
    var cfg RuntimeProfileFile
    if err := dec.Decode(&cfg); err != nil { return RuntimeProfileFile{}, fmt.Errorf("decode Windows GUI profile: %w", err) }
    if err := ensureJSONEOF(dec); err != nil { return RuntimeProfileFile{}, err }
    return cfg, nil
}

func ImportRuntimeProfile(path, name string) (SavedProfile, error) {
    cfg, err := ReadRuntimeProfileFile(path); if err != nil { return SavedProfile{}, err }
    id, err := newProfileID(); if err != nil { return SavedProfile{}, err }
    name = strings.TrimSpace(name)
    if name == "" { name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) }
    if name == "" { name = "导入服务器" }
    return SavedProfile{ID: id, Name: name, Config: cfg}, nil
}

func WriteRuntimeProfileFile(path string, cfg RuntimeProfileFile) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { return err }
    data, err := json.MarshalIndent(cfg, "", "  "); if err != nil { return err }
    data = append(data, '\n')
    return os.WriteFile(path, data, 0o600)
}
'''
Path('internal/windowsgui/profile_store.go').write_text(profile_store, encoding='utf-8')

profile_store_test = r'''package windowsgui

import (
    "reflect"
    "testing"
)

func boolp(v bool) *bool { return &v }
func intp(v int) *int { return &v }

func TestProfileStoreRoundTripsAllUserConfigurationFields(t *testing.T) {
    cfg := RuntimeProfileFile{
        ServerIP: "198.51.100.42", ServerPort: 40443, ServerName: "front.example", RouteKey: "0123456789abcdef",
        Username: "alice", Password: "secret", VerifyServer: true, FEC: "20:20", IfName: "WBD-CN", MTU: 1320,
        RouteMode: "Foreign", ProxyLAN: boolp(true), ProxyChina: boolp(false), ProxyOther: boolp(true),
        DNSMode: "Custom", DNSServer: "9.9.9.9", Lanes: 4, IdleTimeout: intp(123), KeepaliveSeconds: intp(0),
        LaneRotationMinSeconds: intp(60), LaneRotationMaxSeconds: intp(120), TunnelIPv4: "10.66.0.2/30",
    }
    store := NewProfileStore()
    p, err := store.Add("东京 1", cfg); if err != nil { t.Fatal(err) }
    store.SelectedID = p.ID
    path := t.TempDir() + "/profiles.json"
    if err := SaveProfileStore(path, store); err != nil { t.Fatal(err) }
    got, err := LoadProfileStore(path); if err != nil { t.Fatal(err) }
    selected, ok := got.Selected(); if !ok { t.Fatal("selected profile lost") }
    if selected.Name != "东京 1" || !reflect.DeepEqual(selected.Config, cfg) { t.Fatalf("round trip=%+v", selected) }
}

func TestProfileStoreSwitchDeleteAndDefaultSelection(t *testing.T) {
    s := NewProfileStore()
    a, _ := s.Add("A", RuntimeProfileFile{ServerIP:"192.0.2.1"})
    b, _ := s.Add("B", RuntimeProfileFile{ServerIP:"192.0.2.2"})
    if !s.Select(b.ID) { t.Fatal("select B failed") }
    if p, _ := s.Selected(); p.ID != b.ID { t.Fatalf("selected=%s", p.ID) }
    if !s.Delete(b.ID) { t.Fatal("delete B failed") }
    if p, ok := s.Selected(); !ok || p.ID != a.ID { t.Fatalf("fallback selected=%+v ok=%v", p, ok) }
}

func TestRuntimeProfileFileReadWritePreservesPointers(t *testing.T) {
    cfg := RuntimeProfileFile{ServerIP:"203.0.113.9", ServerPort:40443, ProxyLAN:boolp(false), ProxyChina:boolp(true), ProxyOther:boolp(false), KeepaliveSeconds:intp(0)}
    path := t.TempDir()+"/wbd.json"
    if err := WriteRuntimeProfileFile(path, cfg); err != nil { t.Fatal(err) }
    got, err := ReadRuntimeProfileFile(path); if err != nil { t.Fatal(err) }
    if !reflect.DeepEqual(got, cfg) { t.Fatalf("got=%+v want=%+v", got, cfg) }
}
'''
Path('internal/windowsgui/profile_store_test.go').write_text(profile_store_test, encoding='utf-8')

replace_once('cmd/wbd-windows-portable/main_windows.go', '\n\t"github.com/lly8666/wobuzhidao/internal/windowsbundle"', '')
replace_once('cmd/wbd-windows-portable/main_windows.go', '''\truntimeInfo, err := windowsbundle.EnsureRuntime()\n\tif err != nil { return fmt.Errorf("prepare embedded WBD runtime: %w", err) }\n\tif installNpcap {\n\t\tscript := filepath.Join(runtimeInfo.Dir, "windows_npcap_prepare.ps1")''', '''\truntimeDir := portableDir\n\tif err := validateInstalledRuntime(runtimeDir); err != nil { return err }\n\tif installNpcap {\n\t\tscript := filepath.Join(runtimeDir, "windows_npcap_prepare.ps1")''')
replace_once('cmd/wbd-windows-portable/main_windows.go', 'windowsgui.LoadRuntimeProfile(profilePath, runtimeInfo.Dir, diagState)', 'windowsgui.LoadRuntimeProfile(profilePath, runtimeDir, diagState)')
replace_once('cmd/wbd-windows-portable/main_windows.go', 'gui := filepath.Join(runtimeInfo.Dir, "wbd-windows-gui.exe")', 'gui := filepath.Join(runtimeDir, "wbd-windows-gui.exe")')
replace_once('cmd/wbd-windows-portable/main_windows.go', '''func showMessage(title, text string, isError bool) {''', '''var requiredInstalledRuntimeFiles = []string{\n\t"wbd-reality-front.exe", "wbd-faketcp.exe", "wbd_dtls_shim.exe", "wbd-link-proxy.exe", "wbd-game-lane-client.exe", "wbd-tun.exe", "wbd-windows-gui.exe",\n\t"wintun.dll", "windows_tun_route.ps1", "windows_tun_rebind.ps1", "windows_ipv6_killswitch.ps1", "windows_faketcp_underlay.ps1", "windows_npcap_prepare.ps1",\n}\n\nfunc validateInstalledRuntime(dir string) error {\n\tif strings.TrimSpace(dir) == "" { return errors.New("WBD 安装目录为空") }\n\tfor _, name := range requiredInstalledRuntimeFiles {\n\t\tinfo, err := os.Stat(filepath.Join(dir, name))\n\t\tif err != nil || info.IsDir() { return fmt.Errorf("WBD 安装目录缺少运行文件 %s: %v", name, err) }\n\t}\n\treturn nil\n}\n\nfunc showMessage(title, text string, isError bool) {''')

p = Path('cmd/wbd-windows-portable/main_windows_test.go')
text = p.read_text(encoding='utf-8')
append = r'''

func TestInstalledRuntimeUsesSiblingFilesWithoutExtraction(t *testing.T) {
    dir := t.TempDir()
    for _, name := range requiredInstalledRuntimeFiles {
        if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o600); err != nil { t.Fatal(err) }
    }
    if err := validateInstalledRuntime(dir); err != nil { t.Fatal(err) }
    if err := os.Remove(filepath.Join(dir, "wbd-link-proxy.exe")); err != nil { t.Fatal(err) }
    if err := validateInstalledRuntime(dir); err == nil || !strings.Contains(err.Error(), "wbd-link-proxy.exe") {
        t.Fatalf("missing sibling runtime was not rejected: %v", err)
    }
}
'''
if 'TestInstalledRuntimeUsesSiblingFilesWithoutExtraction' not in text:
    p.write_text(text + append, encoding='utf-8')

print('windows ui backend/direct-install patch applied')
