package windowsgui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

type RuntimeProfileFile struct {
	ServerFront string `json:"server_front"`; ServerName string `json:"server_name"`; RouteKey string `json:"route_key"`; Username string `json:"username"`; Password string `json:"password"`; ServerRaw string `json:"server_raw"`; VerifyServer bool `json:"verify_server"`; FEC string `json:"fec"`; IfName string `json:"if_name"`; MTU int `json:"mtu"`; RouteMode string `json:"route_mode"`; DNSMode string `json:"dns_mode"`; DNSServer string `json:"dns_server"`; TunnelIPv4 string `json:"tunnel_ipv4"`
}

func LoadRuntimeProfile(path, binDir, stateDir string) (windowsruntime.Profile, error) {
	f, err := os.Open(path); if err != nil { return windowsruntime.Profile{}, err }; defer f.Close()
	decoder := json.NewDecoder(f); decoder.DisallowUnknownFields(); var cfg RuntimeProfileFile
	if err := decoder.Decode(&cfg); err != nil { return windowsruntime.Profile{}, fmt.Errorf("decode Windows GUI profile: %w", err) }
	if err := ensureJSONEOF(decoder); err != nil { return windowsruntime.Profile{}, err }
	if cfg.FEC==""{cfg.FEC="off"};if cfg.IfName==""{cfg.IfName="WBD"};if cfg.MTU==0{cfg.MTU=1400};if cfg.RouteMode==""{cfg.RouteMode=windowsruntime.RouteFull};if cfg.DNSMode==""{cfg.DNSMode=windowsruntime.DNSAuto};if cfg.TunnelIPv4==""{cfg.TunnelIPv4="10.66.0.2/30"}

	cnSetDir := filepath.Join(stateDir, "ipsets", "cn")
	if portable := os.Getenv("WBD_PORTABLE_DIR"); portable != "" { cnSetDir = filepath.Clean(portable) }
	profile := windowsruntime.Profile{
		BinDir:filepath.Clean(binDir), ServerFront:cfg.ServerFront, ServerName:cfg.ServerName, RouteKey:cfg.RouteKey, Username:cfg.Username, Password:cfg.Password, ServerRaw:cfg.ServerRaw, VerifyServer:cfg.VerifyServer, FEC:cfg.FEC, IfName:cfg.IfName, MTU:cfg.MTU, RouteMode:cfg.RouteMode, CNSetDir:cnSetDir, DNSMode:cfg.DNSMode, DNSServer:cfg.DNSServer, TunnelIPv4:cfg.TunnelIPv4, TicketPath:filepath.Join(stateDir,"reality-ticket.tmp"), RouteState:filepath.Join(stateDir,"route-state.json"),
	}
	if err:=profile.Validate();err!=nil{return windowsruntime.Profile{},fmt.Errorf("validate Windows GUI profile: %w",err)}
	return profile,nil
}

func ensureJSONEOF(decoder *json.Decoder) error { var extra any; if err:=decoder.Decode(&extra);err==io.EOF{return nil}else if err!=nil{return fmt.Errorf("decode Windows GUI profile trailer: %w",err)};return fmt.Errorf("Windows GUI profile must contain exactly one JSON object") }
