package windowsgui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

type RuntimeProfileFile struct {
	ServerIP string `json:"server_ip"`; ServerPort int `json:"server_port"`
	// Legacy endpoint fields remain decode-only so existing same-endpoint profiles
	// can be opened. New product profiles must use server_ip + server_port.
	ServerFront string `json:"server_front"`; ServerRaw string `json:"server_raw"`
	ServerName string `json:"server_name"`; RouteKey string `json:"route_key"`; Username string `json:"username"`; Password string `json:"password"`; VerifyServer bool `json:"verify_server"`; FEC string `json:"fec"`; IfName string `json:"if_name"`; MTU int `json:"mtu"`; RouteMode string `json:"route_mode"`; DNSMode string `json:"dns_mode"`; DNSServer string `json:"dns_server"`; TunnelIPv4 string `json:"tunnel_ipv4"`
}

func resolveServerEndpoints(cfg RuntimeProfileFile) (string, string, error) {
	serverIP := strings.TrimSpace(cfg.ServerIP)
	usingNew := serverIP != "" || cfg.ServerPort != 0
	usingLegacy := strings.TrimSpace(cfg.ServerFront) != "" || strings.TrimSpace(cfg.ServerRaw) != ""
	if usingNew && usingLegacy {
		return "", "", fmt.Errorf("Windows profile cannot mix server_ip/server_port with legacy server_front/server_raw")
	}
	if usingNew {
		ip, err := netip.ParseAddr(serverIP)
		if err != nil || !ip.Is4() {
			return "", "", fmt.Errorf("server_ip must be one IPv4 address")
		}
		if cfg.ServerPort < 1 || cfg.ServerPort > 65535 {
			return "", "", fmt.Errorf("server_port must be 1..65535")
		}
		endpoint := netip.AddrPortFrom(ip, uint16(cfg.ServerPort)).String()
		return endpoint, endpoint, nil
	}
	if !usingLegacy {
		return "", "", fmt.Errorf("Windows profile requires server_ip and server_port")
	}
	front, errFront := netip.ParseAddrPort(strings.TrimSpace(cfg.ServerFront))
	raw, errRaw := netip.ParseAddrPort(strings.TrimSpace(cfg.ServerRaw))
	if errFront != nil || errRaw != nil || !front.Addr().Is4() || !raw.Addr().Is4() {
		return "", "", fmt.Errorf("legacy server_front/server_raw must both be IPv4 address:port values")
	}
	if front != raw {
		return "", "", fmt.Errorf("legacy server_front/server_raw differ; migrate wbd.json to server_ip + server_port for the shared public port")
	}
	return front.String(), raw.String(), nil
}

func LoadRuntimeProfile(path, binDir, stateDir string) (windowsruntime.Profile, error) {
	f, err := os.Open(path); if err != nil { return windowsruntime.Profile{}, err }; defer f.Close()
	decoder := json.NewDecoder(f); decoder.DisallowUnknownFields(); var cfg RuntimeProfileFile
	if err := decoder.Decode(&cfg); err != nil { return windowsruntime.Profile{}, fmt.Errorf("decode Windows GUI profile: %w", err) }
	if err := ensureJSONEOF(decoder); err != nil { return windowsruntime.Profile{}, err }
	serverFront, serverRaw, err := resolveServerEndpoints(cfg); if err != nil { return windowsruntime.Profile{}, err }
	if cfg.FEC==""{cfg.FEC="off"};if cfg.IfName==""{cfg.IfName="WBD"};if cfg.MTU==0{cfg.MTU=1400};if cfg.RouteMode==""{cfg.RouteMode=windowsruntime.RouteFull};if cfg.DNSMode==""{cfg.DNSMode=windowsruntime.DNSAuto};if cfg.TunnelIPv4==""{cfg.TunnelIPv4="10.66.0.2/30"}

	cnSetDir := filepath.Join(stateDir, "ipsets", "cn")
	if portable := os.Getenv("WBD_PORTABLE_DIR"); portable != "" { cnSetDir = filepath.Clean(portable) }
	profile := windowsruntime.Profile{
		BinDir:filepath.Clean(binDir), ServerFront:serverFront, ServerName:cfg.ServerName, RouteKey:cfg.RouteKey, Username:cfg.Username, Password:cfg.Password, ServerRaw:serverRaw, VerifyServer:cfg.VerifyServer, FEC:cfg.FEC, IfName:cfg.IfName, MTU:cfg.MTU, RouteMode:cfg.RouteMode, CNSetDir:cnSetDir, DNSMode:cfg.DNSMode, DNSServer:cfg.DNSServer, TunnelIPv4:cfg.TunnelIPv4, TicketPath:filepath.Join(stateDir,"reality-ticket.tmp"), RouteState:filepath.Join(stateDir,"route-state.json"),
	}
	if err:=profile.Validate();err!=nil{return windowsruntime.Profile{},fmt.Errorf("validate Windows GUI profile: %w",err)}
	return profile,nil
}

func ensureJSONEOF(decoder *json.Decoder) error { var extra any; if err:=decoder.Decode(&extra);err==io.EOF{return nil}else if err!=nil{return fmt.Errorf("decode Windows GUI profile trailer: %w",err)};return fmt.Errorf("Windows GUI profile must contain exactly one JSON object") }
