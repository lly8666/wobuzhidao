package windowsgui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func writeTestProfile(t *testing.T, dir, extra string) string {
	t.Helper()
	path := filepath.Join(dir, "profile.json")
	body := `{
  "server_ip": "198.51.100.10",
  "server_port": 40443,
  "server_name": "front.example",
  "route_key": "0123456789abcdef",
  "username": "solo",
  "password": "shared-password",
  "verify_server": false,
  "fec": "20:20"` + extra + `
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil { t.Fatal(err) }
	return path
}

func TestLoadRuntimeProfileAppliesADR0012Defaults(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir(); path := writeTestProfile(t, dir, "")
	stateDir := filepath.Join(dir, "state")
	profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), stateDir); if err != nil { t.Fatal(err) }
	if profile.ServerFront!="198.51.100.10:40443"||profile.ServerRaw!=profile.ServerFront{t.Fatalf("shared server endpoint not derived: %+v",profile)}
	if profile.FEC!="20:20"||profile.IfName!="WBD"||profile.MTU!=1400||profile.RouteMode!=windowsruntime.RouteFull||profile.DNSMode!=windowsruntime.DNSAuto||profile.Lanes!=1 { t.Fatalf("unexpected defaults: %+v",profile) }
	if profile.TunnelIPv4!="" { t.Fatalf("tunnel address must be server-assigned after bootstrap, got=%q", profile.TunnelIPv4) }
	if _,err:=logicaltunnel.ParseInstallationID(profile.InstallationID);err!=nil{t.Fatalf("installation id invalid: %q: %v",profile.InstallationID,err)}
	if !strings.HasSuffix(profile.TicketPath,filepath.Join("state","reality-ticket.tmp"))||!strings.HasSuffix(profile.TunnelConfigPath,filepath.Join("state","tunnel-config.json"))||!strings.HasSuffix(profile.RouteState,filepath.Join("state","route-state.json"))||!strings.HasSuffix(profile.CNSetDir,filepath.Join("state","ipsets","cn")){t.Fatalf("WBD-owned paths escaped installed state dir: %+v",profile)}
}

func TestLoadRuntimeProfilePersistsInstallationIdentityAcrossLoads(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir(); path:=writeTestProfile(t,dir,""); stateDir:=filepath.Join(dir,"state")
	first,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),stateDir);if err!=nil{t.Fatal(err)}
	second,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),stateDir);if err!=nil{t.Fatal(err)}
	if first.InstallationID==""||first.InstallationID!=second.InstallationID{t.Fatalf("installation identity not stable: first=%q second=%q",first.InstallationID,second.InstallationID)}
	b,err:=os.ReadFile(filepath.Join(stateDir,"installation-id"));if err!=nil{t.Fatal(err)}
	if strings.TrimSpace(string(b))!=first.InstallationID{t.Fatalf("persisted installation id=%q want=%q",strings.TrimSpace(string(b)),first.InstallationID)}
}

func TestLoadRuntimeProfileAcceptsProductLaneRange(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	for _, lanes := range []int{1,2,3,4} {
		t.Run(fmt.Sprintf("accept-lanes-%d",lanes),func(t *testing.T){
			dir:=t.TempDir();path:=writeTestProfile(t,dir,fmt.Sprintf(",\n  \"lanes\": %d",lanes))
			p,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));if err!=nil{t.Fatal(err)}
			if p.Lanes!=lanes{t.Fatalf("lanes=%d want=%d",p.Lanes,lanes)}
		})
	}
	// Omitted/zero remains the profile default sentinel and normalizes to one.
	dir:=t.TempDir();zeroPath:=writeTestProfile(t,dir,`,
  "lanes": 0`);p,err:=LoadRuntimeProfile(zeroPath,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));if err!=nil{t.Fatal(err)};if p.Lanes!=1{t.Fatalf("zero/default lanes normalized to %d want=1",p.Lanes)}
	for _, lanes := range []int{-1,5,8} {
		t.Run(fmt.Sprintf("reject-lanes-%d", lanes), func(t *testing.T) {
			dir:=t.TempDir(); badPath:=writeTestProfile(t,dir,fmt.Sprintf(",\n  \"lanes\": %d",lanes))
			if _,err:=LoadRuntimeProfile(badPath,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));err==nil{t.Fatalf("invalid product lane count %d unexpectedly accepted",lanes)}
		})
	}
}

func TestLegacyTunnelIPv4FieldDoesNotOverrideAuthenticatedLease(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir();path:=writeTestProfile(t,dir,`,
  "tunnel_ipv4": "10.66.0.2/30"`)
	p,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));if err!=nil{t.Fatal(err)}
	if p.TunnelIPv4!=""{t.Fatalf("legacy tunnel_ipv4 became authority: %q",p.TunnelIPv4)}
}

func TestLoadRuntimeProfilePortableCNAssetsStayBesideOuterExecutable(t *testing.T) {
	dir:=t.TempDir(); portable:=filepath.Join(dir,"portable client"); if err:=os.MkdirAll(portable,0o700);err!=nil{t.Fatal(err)};t.Setenv("WBD_PORTABLE_DIR",portable)
	path:=writeTestProfile(t,dir,`,
  "route_mode": "Foreign"`)
	profile,err:=LoadRuntimeProfile(path,filepath.Join(dir,"runtime"),filepath.Join(dir,"state"));if err!=nil{t.Fatal(err)}
	if profile.CNSetDir!=filepath.Clean(portable){t.Fatalf("portable CNSetDir=%q want=%q",profile.CNSetDir,portable)}
	if !strings.HasSuffix(profile.RouteState,filepath.Join("state","route-state.json")){t.Fatalf("mutable state must remain outside portable asset directory: %+v",profile)}
}

func TestLoadRuntimeProfileAcceptsUserRoutingAndDNSPolicyWithoutPathOverride(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir();path:=writeTestProfile(t,dir,`,
  "route_mode": "Foreign",
  "dns_mode": "Custom",
  "dns_server": "9.9.9.9"`);stateDir:=filepath.Join(dir,"state")
	profile,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),stateDir);if err!=nil{t.Fatal(err)}
	if profile.RouteMode!=windowsruntime.RouteForeign||profile.DNSMode!=windowsruntime.DNSCustom||profile.DNSServer!="9.9.9.9"{t.Fatalf("routing policy=%+v",profile)}
	if profile.CNSetDir!=filepath.Join(stateDir,"ipsets","cn"){t.Fatalf("CNSetDir=%q",profile.CNSetDir)}
}

func TestLoadRuntimeProfileRejectsPathInjectionFields(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir();path:=filepath.Join(dir,"profile.json");body:=`{"server_ip":"198.51.100.10","server_port":40443,"server_name":"front.example","route_key":"0123456789abcdef","username":"solo","password":"shared-password","bin_dir":"C:\\evil"}`;if err:=os.WriteFile(path,[]byte(body),0o600);err!=nil{t.Fatal(err)};if _,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));err==nil{t.Fatal("unknown path-bearing profile field must be rejected")}
}
func TestLoadRuntimeProfileRejectsUnfrozenFEC(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir();path:=filepath.Join(dir,"profile.json");body:=`{"server_ip":"198.51.100.10","server_port":40443,"server_name":"front.example","route_key":"0123456789abcdef","username":"solo","password":"shared-password","fec":"auto"}`;if err:=os.WriteFile(path,[]byte(body),0o600);err!=nil{t.Fatal(err)};if _,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));err==nil{t.Fatal("unfrozen FEC must be rejected")}
}

func TestLoadRuntimeProfileAcceptsOnlySameEndpointLegacyProfile(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir(); path:=filepath.Join(dir,"profile.json")
	body:=`{"server_front":"198.51.100.10:40443","server_raw":"198.51.100.10:40443","server_name":"front.example","route_key":"0123456789abcdef","username":"solo","password":"shared-password"}`
	if err:=os.WriteFile(path,[]byte(body),0o600);err!=nil{t.Fatal(err)}
	p,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));if err!=nil{t.Fatal(err)}
	if p.ServerFront!=p.ServerRaw||p.ServerRaw!="198.51.100.10:40443"{t.Fatalf("legacy same endpoint=%+v",p)}
}

func TestLoadRuntimeProfileRejectsLegacySplitPorts(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir(); path:=filepath.Join(dir,"profile.json")
	body:=`{"server_front":"198.51.100.10:40443","server_raw":"198.51.100.10:40000","server_name":"front.example","route_key":"0123456789abcdef","username":"solo","password":"shared-password"}`
	if err:=os.WriteFile(path,[]byte(body),0o600);err!=nil{t.Fatal(err)}
	_,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));if err==nil||!strings.Contains(err.Error(),"server_ip + server_port"){t.Fatalf("split legacy profile error=%v",err)}
}

func TestLoadRuntimeProfileRejectsMixedEndpointSchemas(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir:=t.TempDir(); path:=filepath.Join(dir,"profile.json")
	body:=`{"server_ip":"198.51.100.10","server_port":40443,"server_front":"198.51.100.10:40443","server_raw":"198.51.100.10:40443","server_name":"front.example","route_key":"0123456789abcdef","username":"solo","password":"shared-password"}`
	if err:=os.WriteFile(path,[]byte(body),0o600);err!=nil{t.Fatal(err)}
	if _,err:=LoadRuntimeProfile(path,filepath.Join(dir,"bin"),filepath.Join(dir,"state"));err==nil{t.Fatal("mixed shared/legacy endpoint schemas unexpectedly accepted")}
}
