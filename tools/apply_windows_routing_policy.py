from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"{path}: expected one replacement, found {count}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1), encoding="utf-8")


def write(path, content):
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")


# windowsruntime.Profile gains an explicit three-way routing policy while the
# old Full/Foreign/China route_mode remains a compatibility input.
replace_once(
    "internal/windowsruntime/plan.go",
    "\tRouteMode string\n\tPrefix4   []string\n\tCNSetDir  string\n",
    "\tRouteMode string\n\t// RoutingPolicy is the product routing model. Nil preserves the legacy\n\t// Full/Foreign/China route_mode mapping.\n\tRoutingPolicy *RoutingPolicy\n\t// AutoUpdateCN refreshes the verified APNIC CN bundle during product\n\t// preflight when China and Other have different proxy decisions.\n\tAutoUpdateCN bool\n\tPrefix4   []string\n\tCNSetDir  string\n",
)
replace_once(
    "internal/windowsruntime/plan.go",
    "\tif (p.RouteMode == RouteForeign || p.RouteMode == RouteChina) && strings.TrimSpace(p.CNSetDir) == \"\" {\n\t\treturn errors.New(\"China/Foreign route mode requires the WBD CN ipset directory\")\n\t}\n",
    "\tif p.RequiresCNSet() && strings.TrimSpace(p.CNSetDir) == \"\" {\n\t\treturn errors.New(\"routing policy requires the WBD CN ipset directory\")\n\t}\n",
)
replace_once(
    "internal/windowsruntime/plan.go",
    "\tif profile.RouteMode != RouteForeign && profile.RouteMode != RouteChina {\n\t\treturn nil\n\t}\n",
    "\tif !profile.RequiresCNSet() {\n\t\treturn nil\n\t}\n",
)
replace_once(
    "internal/windowsruntime/plan.go",
    "\tpsMode := \"Full\"\n\tvar prefixFile, directFile string\n\tswitch profile.RouteMode {\n\tcase RouteForeign:\n\t\tdirectFile = filepath.Join(profile.CNSetDir, ipset.CNIPv4File)\n\tcase RouteChina:\n\t\tpsMode = \"Split\"\n\t\tprefixFile = filepath.Join(profile.CNSetDir, ipset.CNIPv4File)\n\t}\n",
    "\trouting := buildRoutingPlan(profile)\n\tpsMode := routing.Mode\n\tprefixFile := routing.PrefixFile4\n\tdirectFile := routing.DirectPrefixFile4\n",
)
replace_once(
    "internal/windowsruntime/plan.go",
    "\tif directFile != \"\" {\n\t\trouteArgs = append(routeArgs, \"-DirectPrefixFile4\", directFile)\n\t}\n\tif len(dnsServers) > 0 {\n",
    "\tif directFile != \"\" {\n\t\trouteArgs = append(routeArgs, \"-DirectPrefixFile4\", directFile)\n\t}\n\tif routing.CaptureLAN {\n\t\trouteArgs = append(routeArgs, \"-CaptureLAN\")\n\t}\n\tif len(dnsServers) > 0 {\n",
)
replace_once(
    "internal/windowsruntime/plan.go",
    "\tcase DNSAuto:\n\t\tif profile.RouteMode == RouteChina {\n\t\t\treturn nil\n\t\t}\n\t\treturn []string{\"1.1.1.1\", \"1.0.0.1\"}\n",
    "\tcase DNSAuto:\n\t\tif !profile.EffectiveRoutingPolicy().ProxyOther {\n\t\t\treturn nil\n\t\t}\n\t\treturn []string{\"1.1.1.1\", \"1.0.0.1\"}\n",
)

write(
    "internal/windowsruntime/routing_policy.go",
    r'''package windowsruntime

import (
    "path/filepath"

    "github.com/lly8666/wobuzhidao/internal/ipset"
)

// RoutingPolicy classifies IPv4 destinations into three user-visible groups.
// LAN means RFC1918 private IPv4. China means the verified APNIC CN allocation
// bundle. Other means all remaining IPv4 destinations.
type RoutingPolicy struct {
    ProxyLAN   bool
    ProxyChina bool
    ProxyOther bool
}

// EffectiveRoutingPolicy maps the legacy route_mode values onto the explicit
// three-way model. Historically Windows' more-specific physical RFC1918 routes
// stayed direct even in Full mode, so legacy compatibility keeps ProxyLAN off.
func (p Profile) EffectiveRoutingPolicy() RoutingPolicy {
    if p.RoutingPolicy != nil {
        return *p.RoutingPolicy
    }
    switch p.RouteMode {
    case RouteForeign:
        return RoutingPolicy{ProxyLAN: false, ProxyChina: false, ProxyOther: true}
    case RouteChina:
        return RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: false}
    case RouteFull, "":
        return RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: true}
    default:
        // Validate rejects unknown route modes; returning the Full-compatible
        // policy here keeps helper methods deterministic before validation.
        return RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: true}
    }
}

// RequiresCNSet is true only when the CN classification can change the routing
// decision. This avoids downloading or verifying a country list when China and
// Other are both proxied or both direct.
func (p Profile) RequiresCNSet() bool {
    policy := p.EffectiveRoutingPolicy()
    return policy.ProxyChina != policy.ProxyOther
}

type routingPlan struct {
    Mode              string
    PrefixFile4       string
    DirectPrefixFile4 string
    CaptureLAN        bool
}

func buildRoutingPlan(profile Profile) routingPlan {
    policy := profile.EffectiveRoutingPolicy()
    plan := routingPlan{Mode: "None", CaptureLAN: policy.ProxyLAN}
    switch {
    case policy.ProxyOther:
        plan.Mode = "Full"
    case policy.ProxyChina || policy.ProxyLAN:
        plan.Mode = "Split"
    }
    if policy.ProxyChina != policy.ProxyOther {
        cn4 := filepath.Join(profile.CNSetDir, ipset.CNIPv4File)
        if policy.ProxyChina {
            plan.PrefixFile4 = cn4
        } else {
            plan.DirectPrefixFile4 = cn4
        }
    }
    return plan
}
''',
)

# Product preflight refreshes the CN list only when the route decision needs it.
replace_once(
    "internal/windowsruntime/controller.go",
    "import (\n\t\"encoding/json\"",
    "import (\n\t\"context\"\n\t\"encoding/json\"",
)
replace_once(
    "internal/windowsruntime/controller.go",
    "\t\"github.com/lly8666/wobuzhidao/internal/logicaltunnel\"\n",
    "\t\"github.com/lly8666/wobuzhidao/internal/ipset\"\n\t\"github.com/lly8666/wobuzhidao/internal/logicaltunnel\"\n",
)
replace_once(
    "internal/windowsruntime/controller.go",
    "func(PowerShellUnderlayDiscoverer)Preflight(profile Profile)error{\n\tprofile=profile.normalized();if err:=profile.Validate();err!=nil{return err};if err:=ValidateRoutingAssets(profile);err!=nil{return err}\n",
    "func(PowerShellUnderlayDiscoverer)Preflight(profile Profile)error{\n\tprofile=profile.normalized();if err:=profile.Validate();err!=nil{return err}\n\tif profile.RequiresCNSet()&&profile.AutoUpdateCN{if _,err:=ipset.EnsureCNBundle(context.Background(),profile.CNSetDir);err!=nil{return fmt.Errorf(\"refresh mainland-China IP ranges: %w\",err)}}\n\tif err:=ValidateRoutingAssets(profile);err!=nil{return err}\n",
)

write(
    "internal/ipset/update.go",
    r'''package ipset

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
)

const (
    // DefaultCNSourceURL is APNIC's current delegated-statistics feed. ParseCN
    // filters it down to allocated/assigned CN IPv4/IPv6 prefixes.
    DefaultCNSourceURL = "https://ftp.apnic.net/stats/apnic/delegated-apnic-latest"
    DefaultCNMaxAge    = 24 * time.Hour
    maxCNDownloadBytes = 32 << 20
)

type EnsureCNResult struct {
    Manifest  BundleManifest
    Refreshed bool
    UsedStale bool
}

// EnsureCNBundle keeps a verified CN bundle fresh. A valid but stale bundle is
// an intentional availability fallback if APNIC cannot be reached; an invalid
// or missing bundle never becomes trusted merely because the network failed.
func EnsureCNBundle(ctx context.Context, dir string) (EnsureCNResult, error) {
    client := &http.Client{Timeout: 20 * time.Second}
    return ensureCNBundle(ctx, client, dir, DefaultCNSourceURL, DefaultCNMaxAge, time.Now().UTC())
}

func ensureCNBundle(ctx context.Context, client *http.Client, dir, sourceURL string, maxAge time.Duration, now time.Time) (EnsureCNResult, error) {
    current, currentErr := VerifyCNBundle(dir)
    if currentErr == nil {
        if generated, err := time.Parse(time.RFC3339, current.GeneratedUTC); err == nil {
            age := now.Sub(generated)
            if age >= 0 && age <= maxAge {
                return EnsureCNResult{Manifest: current}, nil
            }
        }
    }

    fallback := func(refreshErr error) (EnsureCNResult, error) {
        if currentErr == nil {
            return EnsureCNResult{Manifest: current, UsedStale: true}, nil
        }
        return EnsureCNResult{}, refreshErr
    }
    if strings.TrimSpace(sourceURL) == "" {
        return fallback(fmt.Errorf("CN IP source URL is empty"))
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
    if err != nil {
        return fallback(fmt.Errorf("create CN IP download request: %w", err))
    }
    req.Header.Set("User-Agent", "WBD-Windows/1 CN-IP-Updater")
    resp, err := client.Do(req)
    if err != nil {
        return fallback(fmt.Errorf("download CN IP ranges: %w", err))
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fallback(fmt.Errorf("download CN IP ranges: HTTP %s", resp.Status))
    }
    body, err := io.ReadAll(io.LimitReader(resp.Body, maxCNDownloadBytes+1))
    if err != nil {
        return fallback(fmt.Errorf("read CN IP ranges: %w", err))
    }
    if len(body) > maxCNDownloadBytes {
        return fallback(fmt.Errorf("CN IP range download exceeds %d bytes", maxCNDownloadBytes))
    }
    prefixes, err := ParseCN(bytes.NewReader(body))
    if err != nil {
        return fallback(fmt.Errorf("parse CN IP ranges: %w", err))
    }
    manifest, err := WriteCNBundle(dir, sourceURL, prefixes)
    if err != nil {
        return fallback(fmt.Errorf("install CN IP ranges: %w", err))
    }
    if _, err := VerifyCNBundle(dir); err != nil {
        return fallback(fmt.Errorf("verify installed CN IP ranges: %w", err))
    }
    return EnsureCNResult{Manifest: manifest, Refreshed: true}, nil
}
''',
)

write(
    "internal/ipset/update_test.go",
    r'''package ipset

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
    "time"
)

const delegatedCNFixture = `2|apnic|20260916|0|0|0|0
apnic|CN|ipv4|1.2.0.0|65536|20200101|allocated
apnic|CN|ipv4|8.8.8.0|256|20200101|assigned
apnic|JP|ipv4|9.9.9.0|256|20200101|allocated
`

func TestEnsureCNBundleDownloadsAndVerifies(t *testing.T) {
    hits := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        hits++
        _, _ = w.Write([]byte(delegatedCNFixture))
    }))
    defer server.Close()
    dir := t.TempDir()
    result, err := ensureCNBundle(context.Background(), server.Client(), dir, server.URL, 24*time.Hour, time.Now().UTC())
    if err != nil { t.Fatal(err) }
    if !result.Refreshed || result.UsedStale || hits != 1 { t.Fatalf("result=%+v hits=%d", result, hits) }
    verified, err := VerifyCNBundle(dir); if err != nil { t.Fatal(err) }
    if verified.IPv4Count != 2 { t.Fatalf("IPv4Count=%d want=2", verified.IPv4Count) }
}

func TestEnsureCNBundleFreshCacheSkipsNetwork(t *testing.T) {
    dir := t.TempDir()
    prefixes, err := ParseCN(stringsNewReader(delegatedCNFixture)); if err != nil { t.Fatal(err) }
    if _, err := WriteCNBundle(dir, "fixture", prefixes); err != nil { t.Fatal(err) }
    client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("network should not be used for fresh cache"); return nil, nil })}
    result, err := ensureCNBundle(context.Background(), client, dir, "https://invalid.test", 24*time.Hour, time.Now().UTC())
    if err != nil { t.Fatal(err) }
    if result.Refreshed || result.UsedStale { t.Fatalf("fresh cache result=%+v", result) }
}

func TestEnsureCNBundleUsesVerifiedStaleCacheOnRefreshFailure(t *testing.T) {
    dir := t.TempDir()
    prefixes, err := ParseCN(stringsNewReader(delegatedCNFixture)); if err != nil { t.Fatal(err) }
    if _, err := WriteCNBundle(dir, "fixture", prefixes); err != nil { t.Fatal(err) }
    manifestPath := filepath.Join(dir, CNManifestFile)
    raw, err := os.ReadFile(manifestPath); if err != nil { t.Fatal(err) }
    var manifest BundleManifest
    if err := json.Unmarshal(raw, &manifest); err != nil { t.Fatal(err) }
    manifest.GeneratedUTC = time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
    raw, _ = json.MarshalIndent(manifest, "", "  "); raw = append(raw, '\n')
    if err := os.WriteFile(manifestPath, raw, 0o600); err != nil { t.Fatal(err) }
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
    defer server.Close()
    result, err := ensureCNBundle(context.Background(), server.Client(), dir, server.URL, 24*time.Hour, time.Now().UTC())
    if err != nil { t.Fatal(err) }
    if !result.UsedStale || result.Refreshed { t.Fatalf("stale fallback result=%+v", result) }
}

func TestEnsureCNBundleFailsWithoutTrustedCache(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
    defer server.Close()
    if _, err := ensureCNBundle(context.Background(), server.Client(), t.TempDir(), server.URL, 24*time.Hour, time.Now().UTC()); err == nil {
        t.Fatal("missing cache plus failed refresh unexpectedly succeeded")
    }
}

type roundTripFunc func(*http.Request) (*http.Response, error)
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type stringReader struct { s string; i int }
func stringsNewReader(s string) *stringReader { return &stringReader{s:s} }
func (r *stringReader) Read(p []byte) (int,error) { if r.i>=len(r.s){return 0,io.EOF}; n:=copy(p,r.s[r.i:]);r.i+=n;return n,nil }
''',
)
# Keep update_test imports simple and idiomatic after generating the file.
replace_once(
    "internal/ipset/update_test.go",
    '    "os"\n    "path/filepath"\n    "testing"\n    "time"\n',
    '    "os"\n    "path/filepath"\n    "strings"\n    "testing"\n    "time"\n',
)
replace_once(
    "internal/ipset/update_test.go",
    "    prefixes, err := ParseCN(stringsNewReader(delegatedCNFixture)); if err != nil { t.Fatal(err) }",
    "    prefixes, err := ParseCN(strings.NewReader(delegatedCNFixture)); if err != nil { t.Fatal(err) }",
)
replace_once(
    "internal/ipset/update_test.go",
    "    prefixes, err := ParseCN(stringsNewReader(delegatedCNFixture)); if err != nil { t.Fatal(err) }",
    "    prefixes, err := ParseCN(strings.NewReader(delegatedCNFixture)); if err != nil { t.Fatal(err) }",
)
replace_once(
    "internal/ipset/update_test.go",
    "\ntype stringReader struct { s string; i int }\nfunc stringsNewReader(s string) *stringReader { return &stringReader{s:s} }\nfunc (r *stringReader) Read(p []byte) (int,error) { if r.i>=len(r.s){return 0,io.EOF}; n:=copy(p,r.s[r.i:]);r.i+=n;return n,nil }\n",
    "\n",
)

# New JSON fields are strict and explicit. Partial policies and mixing with the
# legacy route_mode are rejected instead of silently guessing.
replace_once(
    "internal/windowsgui/config.go",
    "RouteMode string `json:\"route_mode\"`; DNSMode string",
    "RouteMode string `json:\"route_mode\"`; ProxyLAN *bool `json:\"proxy_lan,omitempty\"`; ProxyChina *bool `json:\"proxy_china,omitempty\"`; ProxyOther *bool `json:\"proxy_other,omitempty\"`; DNSMode string",
)
replace_once(
    "internal/windowsgui/config.go",
    "\tserverFront, serverRaw, err := resolveServerEndpoints(cfg); if err != nil { return windowsruntime.Profile{}, err }\n\tif cfg.FEC==\"\"{cfg.FEC=\"off\"};if cfg.IfName==\"\"{cfg.IfName=\"WBD\"};if cfg.MTU==0{cfg.MTU=windowsruntime.DefaultTunnelMTU};if cfg.RouteMode==\"\"{cfg.RouteMode=windowsruntime.RouteFull};if cfg.DNSMode==\"\"{cfg.DNSMode=windowsruntime.DNSAuto};if cfg.Lanes==0{cfg.Lanes=1}\n",
    "\tserverFront, serverRaw, err := resolveServerEndpoints(cfg); if err != nil { return windowsruntime.Profile{}, err }\n\troutingPolicy, err := routingPolicyFromConfig(cfg); if err != nil { return windowsruntime.Profile{}, err }\n\tif cfg.FEC==\"\"{cfg.FEC=\"off\"};if cfg.IfName==\"\"{cfg.IfName=\"WBD\"};if cfg.MTU==0{cfg.MTU=windowsruntime.DefaultTunnelMTU};if cfg.RouteMode==\"\"{cfg.RouteMode=windowsruntime.RouteFull};if cfg.DNSMode==\"\"{cfg.DNSMode=windowsruntime.DNSAuto};if cfg.Lanes==0{cfg.Lanes=1}\n",
)
replace_once(
    "internal/windowsgui/config.go",
    "RouteMode:cfg.RouteMode, CNSetDir:cnSetDir, DNSMode:cfg.DNSMode",
    "RouteMode:cfg.RouteMode, RoutingPolicy:routingPolicy, AutoUpdateCN:true, CNSetDir:cnSetDir, DNSMode:cfg.DNSMode",
)
write(
    "internal/windowsgui/routing_config.go",
    r'''package windowsgui

import (
    "fmt"
    "strings"

    "github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func routingPolicyFromConfig(cfg RuntimeProfileFile) (*windowsruntime.RoutingPolicy, error) {
    supplied := 0
    for _, v := range []*bool{cfg.ProxyLAN, cfg.ProxyChina, cfg.ProxyOther} {
        if v != nil { supplied++ }
    }
    if supplied == 0 { return nil, nil }
    if supplied != 3 { return nil, fmt.Errorf("proxy_lan, proxy_china and proxy_other must be set together") }
    if strings.TrimSpace(cfg.RouteMode) != "" { return nil, fmt.Errorf("new proxy_lan/proxy_china/proxy_other settings cannot be mixed with legacy route_mode") }
    return &windowsruntime.RoutingPolicy{ProxyLAN:*cfg.ProxyLAN, ProxyChina:*cfg.ProxyChina, ProxyOther:*cfg.ProxyOther}, nil
}
''',
)
write(
    "internal/windowsgui/routing_config_test.go",
    r'''package windowsgui

import (
    "path/filepath"
    "testing"
)

func TestLoadRuntimeProfileAcceptsExplicitRoutingPolicy(t *testing.T) {
    t.Setenv("WBD_PORTABLE_DIR", "")
    dir := t.TempDir()
    path := writeTestProfile(t, dir, `,
  "proxy_lan": true,
  "proxy_china": false,
  "proxy_other": true`)
    profile, err := LoadRuntimeProfile(path, filepath.Join(dir,"bin"), filepath.Join(dir,"state")); if err != nil { t.Fatal(err) }
    if profile.RoutingPolicy == nil { t.Fatal("explicit routing policy missing") }
    got := *profile.RoutingPolicy
    if !got.ProxyLAN || got.ProxyChina || !got.ProxyOther { t.Fatalf("routing policy=%+v", got) }
    if !profile.AutoUpdateCN { t.Fatal("Windows product profile must enable automatic CN bundle refresh") }
}

func TestLoadRuntimeProfileRejectsPartialRoutingPolicy(t *testing.T) {
    t.Setenv("WBD_PORTABLE_DIR", "")
    dir := t.TempDir(); path := writeTestProfile(t, dir, `,
  "proxy_lan": true,
  "proxy_other": true`)
    if _, err := LoadRuntimeProfile(path, filepath.Join(dir,"bin"), filepath.Join(dir,"state")); err == nil { t.Fatal("partial routing policy unexpectedly accepted") }
}

func TestLoadRuntimeProfileRejectsMixedLegacyAndExplicitRoutingPolicy(t *testing.T) {
    t.Setenv("WBD_PORTABLE_DIR", "")
    dir := t.TempDir(); path := writeTestProfile(t, dir, `,
  "route_mode": "Foreign",
  "proxy_lan": false,
  "proxy_china": false,
  "proxy_other": true`)
    if _, err := LoadRuntimeProfile(path, filepath.Join(dir,"bin"), filepath.Join(dir,"state")); err == nil { t.Fatal("mixed legacy/new routing policy unexpectedly accepted") }
}
''',
)

write(
    "internal/windowsruntime/routing_policy_test.go",
    r'''package windowsruntime

import (
    "net/netip"
    "os"
    "path/filepath"
    "slices"
    "strings"
    "testing"

    "github.com/lly8666/wobuzhidao/internal/ipset"
)

func TestRoutingPolicyMatrix(t *testing.T) {
    cases := []struct{
        name string
        policy RoutingPolicy
        mode string
        needsCN, cnCapture, cnDirect, captureLAN bool
    }{
        {"000",RoutingPolicy{false,false,false},"None",false,false,false,false},
        {"001",RoutingPolicy{false,false,true},"Full",true,false,true,false},
        {"010",RoutingPolicy{false,true,false},"Split",true,true,false,false},
        {"011",RoutingPolicy{false,true,true},"Full",false,false,false,false},
        {"100",RoutingPolicy{true,false,false},"Split",false,false,false,true},
        {"101",RoutingPolicy{true,false,true},"Full",true,false,true,true},
        {"110",RoutingPolicy{true,true,false},"Split",true,true,false,true},
        {"111",RoutingPolicy{true,true,true},"Full",false,false,false,true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            p := testProfile(); p.CNSetDir = filepath.Join("C:\\", "wbd-cn"); p.RoutingPolicy = &tc.policy
            plan := buildRoutingPlan(p)
            if plan.Mode != tc.mode || p.RequiresCNSet() != tc.needsCN || plan.CaptureLAN != tc.captureLAN { t.Fatalf("plan=%+v needsCN=%v",plan,p.RequiresCNSet()) }
            if (plan.PrefixFile4!="") != tc.cnCapture || (plan.DirectPrefixFile4!="") != tc.cnDirect { t.Fatalf("CN route plan=%+v",plan) }
        })
    }
}

func TestLegacyRouteModesMapToExplicitPolicy(t *testing.T) {
    cases := []struct{mode string; want RoutingPolicy}{{RouteFull,RoutingPolicy{false,true,true}},{RouteForeign,RoutingPolicy{false,false,true}},{RouteChina,RoutingPolicy{false,true,false}}}
    for _, tc := range cases { p:=testProfile();p.RouteMode=tc.mode;if got:=p.EffectiveRoutingPolicy();got!=tc.want{t.Fatalf("mode=%s got=%+v want=%+v",tc.mode,got,tc.want)} }
}

func TestBuildPlanExplicitRoutingPolicyUsesCNAndLANArguments(t *testing.T) {
    dir:=t.TempDir();if _,err:=ipset.WriteCNBundle(dir,"test",[]netip.Prefix{netip.MustParsePrefix("1.2.0.0/16")});err!=nil{t.Fatal(err)}
    policy:=RoutingPolicy{ProxyLAN:true,ProxyChina:false,ProxyOther:true}
    p:=testProfile();p.CNSetDir=dir;p.RoutingPolicy=&policy
    plan,err:=BuildPlan(p,testUnderlay(),strings.Repeat("ab",32));if err!=nil{t.Fatal(err)}
    if !argPair(plan.RouteApply.Args,"-Mode","Full")||!argPair(plan.RouteApply.Args,"-DirectPrefixFile4",filepath.Join(dir,ipset.CNIPv4File))||!slices.Contains(plan.RouteApply.Args,"-CaptureLAN"){t.Fatalf("route args=%v",plan.RouteApply.Args)}
}

func TestRoutingPolicyAutoDNSFollowsOtherTraffic(t *testing.T) {
    for _, tc := range []struct{other bool; want bool}{{false,false},{true,true}} {
        p:=testProfile();policy:=RoutingPolicy{ProxyLAN:false,ProxyChina:!tc.other,ProxyOther:tc.other};p.RoutingPolicy=&policy;p.DNSMode=DNSAuto
        got:=resolvedDNSServers(p);if (len(got)>0)!=tc.want{t.Fatalf("other=%v DNS=%v",tc.other,got)}
    }
}

func TestRoutingPolicyActionCase(t *testing.T) {
    raw:=os.Getenv("WBD_ROUTING_POLICY_CASE");if raw==""{t.Skip("Actions-only policy selector")};if len(raw)!=3{t.Fatalf("case=%q",raw)}
    policy:=RoutingPolicy{ProxyLAN:raw[0]=='1',ProxyChina:raw[1]=='1',ProxyOther:raw[2]=='1'}
    p:=testProfile();p.CNSetDir=`C:\wbd-cn`;p.RoutingPolicy=&policy
    plan:=buildRoutingPlan(p)
    wantMode:="None";if policy.ProxyOther{wantMode="Full"}else if policy.ProxyChina||policy.ProxyLAN{wantMode="Split"}
    if plan.Mode!=wantMode||plan.CaptureLAN!=policy.ProxyLAN||p.RequiresCNSet()!=(policy.ProxyChina!=policy.ProxyOther){t.Fatalf("case=%s plan=%+v needsCN=%v",raw,plan,p.RequiresCNSet())}
}
''',
)

# Windows GUI exposes the three independent choices and freezes them while the
# runtime is active. The checked values are copied into the profile at Connect.
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    "wsOverlappedWindow=0x00CF0000; wsChild=0x40000000; wsVisible=0x10000000; bsPushButton=0x00000000",
    "wsOverlappedWindow=0x00CF0000; wsChild=0x40000000; wsVisible=0x10000000; bsPushButton=0x00000000; bsAutoCheckbox=0x00000003",
)
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    "wmCreate=0x0001; wmDestroy=0x0002; wmSize=0x0005; wmClose=0x0010; wmCommand=0x0111; wmSetFont=0x0030; wmLButtonDbl=0x0203; wmRButtonUp=0x0205; wmApp=0x8000",
    "wmCreate=0x0001; wmDestroy=0x0002; wmSize=0x0005; wmClose=0x0010; wmCommand=0x0111; wmSetFont=0x0030; bmGetCheck=0x00F0; bmSetCheck=0x00F1; bstChecked=1; wmLButtonDbl=0x0203; wmRButtonUp=0x0205; wmApp=0x8000",
)
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    "idConnectButton=1000; idHideButton=1001; idExitButton=1002; idDisconnectButton=1003; idNpcapButton=1004; idReconnectButton=1005",
    "idConnectButton=1000; idHideButton=1001; idExitButton=1002; idDisconnectButton=1003; idNpcapButton=1004; idReconnectButton=1005; idProxyLAN=1006; idProxyChina=1007; idProxyOther=1008",
)
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    "window,status,connectButton,disconnectButton,reconnectButton,npcapButton,hideButton,exitButton,icon,font uintptr",
    "window,status,connectButton,disconnectButton,reconnectButton,npcapButton,hideButton,exitButton,proxyLAN,proxyChina,proxyOther,icon,font uintptr",
)
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    "procCreateWindowExW.Call(0,uintptr(unsafe.Pointer(className)),uintptr(unsafe.Pointer(title)),wsOverlappedWindow,180,140,780,350,0,0,instance,0)",
    "procCreateWindowExW.Call(0,uintptr(unsafe.Pointer(className)),uintptr(unsafe.Pointer(title)),wsOverlappedWindow,180,140,780,445,0,0,instance,0)",
)
old_controls = '''func createControls(hwnd uintptr){
\tstatus:="Status: disconnected; put wbd.json beside wbd.exe to connect";if app.profileReady{status="Status: disconnected; portable profile loaded"}else if app.profileErr!=nil{status="Status: disconnected; profile invalid"}
\tapp.status=createControl(hwnd,"STATIC",status,24,26,710,30,0)
\tcreateControl(hwnd,"STATIC","WBD blocks all device IPv6 while connected. Minimize/X keeps VPN running. Reconnect transport rotates active lanes with make-before-break while Wintun/routes stay online. Disconnect or Exit removes WBD routes, DNS policy and IPv6 block.",24,64,710,70,0)
\tapp.connectButton=createControl(hwnd,"BUTTON","Connect",24,160,105,36,idConnectButton)
\tapp.disconnectButton=createControl(hwnd,"BUTTON","Disconnect",141,160,105,36,idDisconnectButton)
\tapp.reconnectButton=createControl(hwnd,"BUTTON","Reconnect transport",258,160,170,36,idReconnectButton)
\tapp.hideButton=createControl(hwnd,"BUTTON","Minimize to tray",440,160,140,36,idHideButton)
\tapp.exitButton=createControl(hwnd,"BUTTON","Exit WBD",592,160,105,36,idExitButton)
\tapp.npcapButton=createControl(hwnd,"BUTTON","Install / repair Npcap",24,214,170,36,idNpcapButton)
\tcreateControl(hwnd,"STATIC","Npcap setup downloads the pinned official installer, verifies SHA-256 + Nmap signature, opens the official installer, then verifies DLLs and driver service.",206,208,528,58,0)
\trefreshControls()
}
func createControl(parent uintptr,class,text string,x,y,width,height int,id uintptr)uintptr{instance,_,_:=procGetModuleHandleW.Call(0);hwnd,_,_:=procCreateWindowExW.Call(0,uintptr(unsafe.Pointer(utf16Ptr(class))),uintptr(unsafe.Pointer(utf16Ptr(text))),wsChild|wsVisible|bsPushButton,uintptr(x),uintptr(y),uintptr(width),uintptr(height),parent,id,instance,0);if hwnd!=0&&app.font!=0{procSendMessageW.Call(hwnd,wmSetFont,app.font,1)};return hwnd}
'''
new_controls = '''func createControls(hwnd uintptr){
\tstatus:="Status: disconnected; put wbd.json beside wbd.exe to connect";if app.profileReady{status="Status: disconnected; portable profile loaded"}else if app.profileErr!=nil{status="Status: disconnected; profile invalid"}
\tapp.status=createControl(hwnd,"STATIC",status,24,22,710,30,0)
\tcreateControl(hwnd,"STATIC","WBD blocks all device IPv6 while connected. Minimize/X keeps VPN running. Routing choices below apply on the next Connect; CN ranges auto-refresh from APNIC when classification is needed.",24,56,710,44,0)
\tpolicy:=windowsruntime.RoutingPolicy{ProxyLAN:false,ProxyChina:true,ProxyOther:true};if app.profileReady{policy=app.profile.EffectiveRoutingPolicy()}
\tapp.proxyLAN=createCheckbox(hwnd,"Proxy LAN/private IPv4 (RFC1918)",24,108,225,28,idProxyLAN);setChecked(app.proxyLAN,policy.ProxyLAN)
\tapp.proxyChina=createCheckbox(hwnd,"Proxy mainland China IPv4",258,108,205,28,idProxyChina);setChecked(app.proxyChina,policy.ProxyChina)
\tapp.proxyOther=createCheckbox(hwnd,"Proxy all other IPv4",478,108,190,28,idProxyOther);setChecked(app.proxyOther,policy.ProxyOther)
\tcreateControl(hwnd,"STATIC","LAN proxying installs competing routes for existing RFC1918 physical routes; the physical server/gateway escape remains pinned so the tunnel cannot recurse.",24,140,710,44,0)
\tapp.connectButton=createControl(hwnd,"BUTTON","Connect",24,200,105,36,idConnectButton)
\tapp.disconnectButton=createControl(hwnd,"BUTTON","Disconnect",141,200,105,36,idDisconnectButton)
\tapp.reconnectButton=createControl(hwnd,"BUTTON","Reconnect transport",258,200,170,36,idReconnectButton)
\tapp.hideButton=createControl(hwnd,"BUTTON","Minimize to tray",440,200,140,36,idHideButton)
\tapp.exitButton=createControl(hwnd,"BUTTON","Exit WBD",592,200,105,36,idExitButton)
\tapp.npcapButton=createControl(hwnd,"BUTTON","Install / repair Npcap",24,254,170,36,idNpcapButton)
\tcreateControl(hwnd,"STATIC","Npcap setup downloads the pinned official installer, verifies SHA-256 + Nmap signature, opens the official installer, then verifies DLLs and driver service.",206,248,528,58,0)
\trefreshControls()
}
func createControl(parent uintptr,class,text string,x,y,width,height int,id uintptr)uintptr{instance,_,_:=procGetModuleHandleW.Call(0);hwnd,_,_:=procCreateWindowExW.Call(0,uintptr(unsafe.Pointer(utf16Ptr(class))),uintptr(unsafe.Pointer(utf16Ptr(text))),wsChild|wsVisible|bsPushButton,uintptr(x),uintptr(y),uintptr(width),uintptr(height),parent,id,instance,0);if hwnd!=0&&app.font!=0{procSendMessageW.Call(hwnd,wmSetFont,app.font,1)};return hwnd}
func createCheckbox(parent uintptr,text string,x,y,width,height int,id uintptr)uintptr{instance,_,_:=procGetModuleHandleW.Call(0);hwnd,_,_:=procCreateWindowExW.Call(0,uintptr(unsafe.Pointer(utf16Ptr("BUTTON"))),uintptr(unsafe.Pointer(utf16Ptr(text))),wsChild|wsVisible|bsAutoCheckbox,uintptr(x),uintptr(y),uintptr(width),uintptr(height),parent,id,instance,0);if hwnd!=0&&app.font!=0{procSendMessageW.Call(hwnd,wmSetFont,app.font,1)};return hwnd}
func setChecked(hwnd uintptr,checked bool){if hwnd==0{return};var v uintptr;if checked{v=bstChecked};procSendMessageW.Call(hwnd,bmSetCheck,v,0)}
func isChecked(hwnd uintptr)bool{if hwnd==0{return false};v,_,_:=procSendMessageW.Call(hwnd,bmGetCheck,0,0);return v==bstChecked}
'''
replace_once("cmd/wbd-windows-gui/main_windows.go", old_controls, new_controls)
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    'func beginConnect(hwnd uintptr){if app.operation!=""||app.exitRequested{return};if !app.profileReady{if app.profileErr!=nil{messageBox("WBD Windows GUI profile",app.profileErr.Error())}else{messageBox("WBD Windows GUI","No profile loaded. Put wbd.json beside wbd.exe and restart WBD, or launch with -profile <path>.")};return};app.operation="connect";app.cleanupFailed=false;setStatus("Status: connecting; IPv6 will be blocked device-wide before IPv4 capture is enabled");refreshControls();go func(profile windowsruntime.Profile){postRuntimeResult(runtimeResult{action:"connect",err:app.controller.Connect(profile)})}(app.profile)}',
    'func beginConnect(hwnd uintptr){if app.operation!=""||app.exitRequested{return};if !app.profileReady{if app.profileErr!=nil{messageBox("WBD Windows GUI profile",app.profileErr.Error())}else{messageBox("WBD Windows GUI","No profile loaded. Put wbd.json beside wbd.exe and restart WBD, or launch with -profile <path>.")};return};profile:=app.profile;profile.RoutingPolicy=&windowsruntime.RoutingPolicy{ProxyLAN:isChecked(app.proxyLAN),ProxyChina:isChecked(app.proxyChina),ProxyOther:isChecked(app.proxyOther)};profile.AutoUpdateCN=true;app.operation="connect";app.cleanupFailed=false;setStatus("Status: connecting; preparing routing policy and CN IP ranges if required");refreshControls();go func(){postRuntimeResult(runtimeResult{action:"connect",err:app.controller.Connect(profile)})}()}',
)
replace_once(
    "cmd/wbd-windows-gui/main_windows.go",
    'func refreshControls(){busy:=app.operation!="";state:=app.controller.State();disconnected:=state==windowsruntime.RuntimeDisconnected;connected:=state==windowsruntime.RuntimeConnected;connectEnabled:=app.profileReady&&!busy&&!app.exitRequested&&!app.cleanupFailed&&disconnected;disconnectEnabled:=!busy&&!app.exitRequested&&(app.cleanupFailed||connected);reconnectEnabled:=!busy&&!app.exitRequested&&!app.cleanupFailed&&connected;npcapEnabled:=!busy&&!app.exitRequested&&!app.cleanupFailed&&disconnected;setEnabled(app.connectButton,connectEnabled);setEnabled(app.disconnectButton,disconnectEnabled);setEnabled(app.reconnectButton,reconnectEnabled);setEnabled(app.npcapButton,npcapEnabled);setEnabled(app.hideButton,!app.exitRequested);setEnabled(app.exitButton,!app.exitRequested)}',
    'func refreshControls(){busy:=app.operation!="";state:=app.controller.State();disconnected:=state==windowsruntime.RuntimeDisconnected;connected:=state==windowsruntime.RuntimeConnected;connectEnabled:=app.profileReady&&!busy&&!app.exitRequested&&!app.cleanupFailed&&disconnected;disconnectEnabled:=!busy&&!app.exitRequested&&(app.cleanupFailed||connected);reconnectEnabled:=!busy&&!app.exitRequested&&!app.cleanupFailed&&connected;npcapEnabled:=!busy&&!app.exitRequested&&!app.cleanupFailed&&disconnected;routingEnabled:=!busy&&!app.exitRequested&&!app.cleanupFailed&&disconnected;setEnabled(app.connectButton,connectEnabled);setEnabled(app.disconnectButton,disconnectEnabled);setEnabled(app.reconnectButton,reconnectEnabled);setEnabled(app.npcapButton,npcapEnabled);setEnabled(app.proxyLAN,routingEnabled);setEnabled(app.proxyChina,routingEnabled);setEnabled(app.proxyOther,routingEnabled);setEnabled(app.hideButton,!app.exitRequested);setEnabled(app.exitButton,!app.exitRequested)}',
)

# The route script adds an explicit None mode and a CaptureLAN switch. CaptureLAN
# enumerates existing physical RFC1918 routes and mirrors their prefix lengths on
# Wintun with a lower interface metric, so a /24 connected route cannot silently
# defeat the user's LAN-proxy choice.
replace_once(
    "scripts/windows_tun_route.ps1",
    "[ValidateSet('Full','Split')]\n    [string]$Mode = 'Full',",
    "[ValidateSet('Full','Split','None')]\n    [string]$Mode = 'Full',",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    "    [string]$DirectPrefixFile4 = '',\n    # Comma/semicolon separated by design:",
    "    [string]$DirectPrefixFile4 = '',\n    [switch]$CaptureLAN,\n    # Comma/semicolon separated by design:",
)
insert_helpers = r'''function Test-RFC1918Prefix([string]$CIDR) {
    $parsed = Parse-CIDR $CIDR ([System.Net.Sockets.AddressFamily]::InterNetwork) 'RFC1918 route'
    if (-not $parsed) { return $false }
    $octets = @($parsed.IP.Split('.') | ForEach-Object { [int]$_ })
    if ($octets[0] -eq 10) { return $parsed.PrefixLength -ge 8 }
    if ($octets[0] -eq 172 -and $octets[1] -ge 16 -and $octets[1] -le 31) { return $parsed.PrefixLength -ge 12 }
    if ($octets[0] -eq 192 -and $octets[1] -eq 168) { return $parsed.PrefixLength -ge 16 }
    return $false
}

function Test-RFC1918Address([string]$IPAddress) {
    if ([string]::IsNullOrWhiteSpace($IPAddress) -or $IPAddress -eq '0.0.0.0') { return $false }
    return Test-RFC1918Prefix "$IPAddress/32"
}

function Get-PhysicalRFC1918RoutePrefixes([uint32]$TunnelInterfaceIndex) {
    $out = @()
    foreach ($route in @(Get-NetRoute -AddressFamily IPv4 -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
        if ([uint32]$route.InterfaceIndex -eq $TunnelInterfaceIndex) { continue }
        $prefix = [string]$route.DestinationPrefix
        if (Test-RFC1918Prefix $prefix) { $out += $prefix }
    }
    return @($out | Select-Object -Unique)
}

'''
replace_once("scripts/windows_tun_route.ps1", "function Read-PrefixFile", insert_helpers + "function Read-PrefixFile")
replace_once(
    "scripts/windows_tun_route.ps1",
    "if ($Mode -eq 'Full') {\n    $capture4 = @('0.0.0.0/1','128.0.0.0/1')\n    $capture6 = if ($addr6) { @('::/1','8000::/1') } else { @() }\n} else {\n    $capture4 = @($Prefix4)\n    $capture6 = @($Prefix6)\n    if ($capture4.Count -eq 0 -and $capture6.Count -eq 0 -and $DNSServers.Count -eq 0) { throw 'Split mode requires Prefix4/Prefix6 and/or DNSServer' }\n}\n",
    "if ($Mode -eq 'Full') {\n    $capture4 = @('0.0.0.0/1','128.0.0.0/1')\n    $capture6 = if ($addr6) { @('::/1','8000::/1') } else { @() }\n} elseif ($Mode -eq 'Split') {\n    $capture4 = @($Prefix4)\n    $capture6 = @($Prefix6)\n} else {\n    $capture4 = @()\n    $capture6 = @()\n}\nif ($CaptureLAN) { $capture4 = @($capture4) + @('10.0.0.0/8','172.16.0.0/12','192.168.0.0/16') }\nif ($Mode -eq 'Split' -and $capture4.Count -eq 0 -and $capture6.Count -eq 0 -and $DNSServers.Count -eq 0) { throw 'Split mode requires Prefix4/Prefix6, CaptureLAN and/or DNSServer' }\n",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    'Write-Output "WBD_WINDOWS_TUN_PLAN mode=$Mode adapter=$AdapterAlias mtu=$MTU"',
    'Write-Output "WBD_WINDOWS_TUN_PLAN mode=$Mode adapter=$AdapterAlias mtu=$MTU capture_lan=$([int]$CaptureLAN.IsPresent)"',
)
replace_once(
    "scripts/windows_tun_route.ps1",
    "    if ($State.PSObject.Properties.Name -contains 'MTU4' -and $null -ne $State.MTU4) {\n        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -NlMtuBytes ([uint32]$State.MTU4) -ErrorAction SilentlyContinue\n    }\n",
    "    if ($State.PSObject.Properties.Name -contains 'MTU4' -and $null -ne $State.MTU4) {\n        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -NlMtuBytes ([uint32]$State.MTU4) -ErrorAction SilentlyContinue\n    }\n    if ($State.PSObject.Properties.Name -contains 'InterfaceMetric4' -and $null -ne $State.InterfaceMetric4) {\n        Set-NetIPInterface -InterfaceIndex ([uint32]$State.AdapterInterfaceIndex) -AddressFamily IPv4 -InterfaceMetric ([uint32]$State.InterfaceMetric4) -ErrorAction SilentlyContinue\n    }\n",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    "$ifIndex = [uint32]$adapter.ifIndex\nWrite-Output \"WBD_WINDOWS_TUN_ADAPTER_READY adapter=$AdapterAlias ifindex=$ifIndex\"\n\n$ipif4 = Get-NetIPInterface",
    "$ifIndex = [uint32]$adapter.ifIndex\nWrite-Output \"WBD_WINDOWS_TUN_ADAPTER_READY adapter=$AdapterAlias ifindex=$ifIndex\"\nif ($CaptureLAN) {\n    $physicalLAN = @(Get-PhysicalRFC1918RoutePrefixes -TunnelInterfaceIndex $ifIndex)\n    $capture4 = @($capture4 + $physicalLAN | Select-Object -Unique)\n    Write-Output \"WBD_WINDOWS_TUN_LAN_CAPTURE_PLAN physical_prefixes=$($physicalLAN.Count) total_capture4=$($capture4.Count)\"\n}\n\n$ipif4 = Get-NetIPInterface",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    "    MTU4 = if ($ipif4) { [uint32]$ipif4.NlMtu } else { $null }\n    MTU6 = if ($configureIPv6 -and $ipif6) { [uint32]$ipif6.NlMtu } else { $null }\n",
    "    MTU4 = if ($ipif4) { [uint32]$ipif4.NlMtu } else { $null }\n    InterfaceMetric4 = if ($ipif4) { [uint32]$ipif4.InterfaceMetric } else { $null }\n    MTU6 = if ($configureIPv6 -and $ipif6) { [uint32]$ipif6.NlMtu } else { $null }\n",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    "        $state.DirectRoutes = @($directCreate)\n        Save-State $state\n        foreach ($route in $directCreate) {\n",
    "        if ($CaptureLAN -and $underlayRoute4 -and (Test-RFC1918Address ([string]$underlayRoute4.NextHop))) {\n            $gatewayPrefix = \"$([string]$underlayRoute4.NextHop)/32\"\n            $gatewayExists = Get-NetRoute -DestinationPrefix $gatewayPrefix -InterfaceIndex ([uint32]$underlayRoute4.InterfaceIndex) -NextHop ([string]$underlayRoute4.NextHop) -PolicyStore ActiveStore -ErrorAction SilentlyContinue\n            if (-not $gatewayExists) {\n                $directCreate += [ordered]@{ DestinationPrefix=$gatewayPrefix; InterfaceIndex=[uint32]$underlayRoute4.InterfaceIndex; NextHop=[string]$underlayRoute4.NextHop }\n            }\n        }\n        $state.DirectRoutes = @($directCreate)\n        Save-State $state\n        foreach ($route in $directCreate) {\n",
)
# The gateway escape must also be created when there are no CN DirectPrefix4
# entries, so widen the enclosing condition from DirectPrefix4-only to LAN too.
replace_once(
    "scripts/windows_tun_route.ps1",
    "    if ($DirectPrefix4.Count -gt 0) {\n        if (-not $underlayRoute4) { throw 'pre-WBD IPv4 route is unavailable for direct-prefix routing' }\n",
    "    if ($DirectPrefix4.Count -gt 0 -or $CaptureLAN) {\n        if (-not $underlayRoute4) { throw 'pre-WBD IPv4 route is unavailable for direct-prefix/LAN routing' }\n",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    "    if ($ipif4) { Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -NlMtuBytes $MTU }\n",
    "    if ($ipif4) { if ($CaptureLAN) { Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -InterfaceMetric 5 }; Set-NetIPInterface -InterfaceIndex $ifIndex -AddressFamily IPv4 -NlMtuBytes $MTU }\n",
)
replace_once(
    "scripts/windows_tun_route.ps1",
    'Write-Output "WBD_WINDOWS_TUN_READY mode=$Mode adapter=$AdapterAlias ifindex=$ifIndex mtu=$MTU direct4=$($DirectPrefix4.Count) capture4=$($capture4.Count) dns=$($DNSServers.Count)"',
    'Write-Output "WBD_WINDOWS_TUN_READY mode=$Mode adapter=$AdapterAlias ifindex=$ifIndex mtu=$MTU direct4=$($DirectPrefix4.Count) capture4=$($capture4.Count) capture_lan=$([int]$CaptureLAN.IsPresent) dns=$($DNSServers.Count)"',
)

write(
    "scripts/windows_routing_policy_action_test.ps1",
    r'''param(
    [Parameter(Mandatory=$true)]
    [ValidatePattern('^[01]{3}$')]
    [string]$Policy
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$proxyLAN = $Policy[0] -eq '1'
$proxyChina = $Policy[1] -eq '1'
$proxyOther = $Policy[2] -eq '1'
$mode = if ($proxyOther) { 'Full' } elseif ($proxyChina -or $proxyLAN) { 'Split' } else { 'None' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ("wbd-routing-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $cn = Join-Path $temp 'cn4.txt'
    [IO.File]::WriteAllText($cn, "1.2.0.0/16`n", (New-Object Text.UTF8Encoding($false)))
    $routeScript = Join-Path $PSScriptRoot 'windows_tun_route.ps1'
    $args = @('-NoProfile','-ExecutionPolicy','Bypass','-File',$routeScript,'-Action','Render','-Mode',$mode,'-AdapterAlias','WBD','-TunnelAddress4','10.66.0.2/30','-Underlay4','198.51.100.10','-MTU','1400')
    if ($proxyChina -ne $proxyOther) {
        if ($proxyChina) { $args += @('-PrefixFile4',$cn) } else { $args += @('-DirectPrefixFile4',$cn) }
    }
    if ($proxyLAN) { $args += '-CaptureLAN' }
    $lines = @(& powershell.exe @args)
    if ($LASTEXITCODE -ne 0) { throw "route render failed policy=$Policy" }
    $plan = ($lines | Where-Object { $_ -like 'WBD_WINDOWS_TUN_PLAN*' } | Select-Object -First 1)
    if (-not $plan -or $plan -notmatch "mode=$mode") { throw "policy=$Policy mode mismatch: $plan" }
    $wantLAN = if ($proxyLAN) { 1 } else { 0 }
    if ($plan -notmatch "capture_lan=$wantLAN") { throw "policy=$Policy LAN marker mismatch: $plan" }
    if ($proxyLAN) {
        foreach ($prefix in @('10.0.0.0/8','172.16.0.0/12','192.168.0.0/16')) {
            if (-not ($lines | Where-Object { $_ -eq "03 CAPTURE IPv4 $prefix on WBD" })) { throw "policy=$Policy missing LAN capture $prefix" }
        }
    }
    if ($proxyChina -ne $proxyOther) {
        if ($proxyChina) {
            if (-not ($lines | Where-Object { $_ -eq '03 CAPTURE IPv4 1.2.0.0/16 on WBD' })) { throw "policy=$Policy CN prefix not captured" }
        } else {
            if (-not ($lines | Where-Object { $_ -eq '01 DIRECT IPv4 1.2.0.0/16 through the pre-WBD physical route' })) { throw "policy=$Policy CN prefix not direct" }
        }
    }
    if ($mode -eq 'None' -and ($lines | Where-Object { $_ -like '03 CAPTURE IPv4*' })) { throw "policy=$Policy None mode unexpectedly captured IPv4" }
    Write-Output "WBD_WINDOWS_ROUTING_POLICY_TEST_PASS policy=$Policy mode=$mode lan=$proxyLAN china=$proxyChina other=$proxyOther"
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}
''',
)

write(
    ".github/workflows/windows-routing-policy.yml",
    r'''name: Windows routing policy

on:
  push:
    branches:
      - agent/fix-per-association-mtu-20260914
  workflow_dispatch:

permissions:
  contents: read

jobs:
  unit:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false
      - name: Routing and CN updater Go tests
        shell: pwsh
        run: |
          go test ./internal/ipset ./internal/windowsruntime ./internal/windowsgui
          go test ./cmd/wbd-windows-gui ./cmd/wbd-windows-portable

  split-routing-matrix:
    runs-on: windows-latest
    strategy:
      fail-fast: false
      matrix:
        policy: ['000','001','010','011','100','101','110','111']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false
      - name: Go policy case
        shell: pwsh
        env:
          WBD_ROUTING_POLICY_CASE: ${{ matrix.policy }}
        run: go test ./internal/windowsruntime -run TestRoutingPolicyActionCase -count=1
      - name: Windows route render case
        shell: pwsh
        run: powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/windows_routing_policy_action_test.ps1 -Policy ${{ matrix.policy }}
''',
)

print("routing policy patch applied")
