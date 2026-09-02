package windowsgui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

const defaultPayloadIdleTimeoutSeconds = 15 * 60

type RuntimeProfileFile struct {
	ServerIP string `json:"server_ip"`; ServerPort int `json:"server_port"`
	// Legacy endpoint fields remain decode-only so existing same-endpoint profiles
	// can be opened. New product profiles must use server_ip + server_port.
	ServerFront string `json:"server_front"`; ServerRaw string `json:"server_raw"`
	ServerName string `json:"server_name"`; RouteKey string `json:"route_key"`; Username string `json:"username"`; Password string `json:"password"`; VerifyServer bool `json:"verify_server"`; FEC string `json:"fec"`; IfName string `json:"if_name"`; MTU int `json:"mtu"`; RouteMode string `json:"route_mode"`; DNSMode string `json:"dns_mode"`; DNSServer string `json:"dns_server"`; Lanes int `json:"lanes"`; IdleTimeout *int `json:"idle_timeout"`
	// tunnel_ipv4 is decode-only compatibility. ADR-0012 address ownership is
	// server-assigned and authenticated during same-flow bootstrap.
	TunnelIPv4 string `json:"tunnel_ipv4"`
}

func resolveServerEndpoints(cfg RuntimeProfileFile) (string, string, error) {
	serverIP := strings.TrimSpace(cfg.ServerIP)
	usingNew := serverIP != "" || cfg.ServerPort != 0
	usingLegacy := strings.TrimSpace(cfg.ServerFront) != "" || strings.TrimSpace(cfg.ServerRaw) != ""
	if usingNew && usingLegacy { return "", "", fmt.Errorf("Windows profile cannot mix server_ip/server_port with legacy server_front/server_raw") }
	if usingNew {
		ip, err := netip.ParseAddr(serverIP)
		if err != nil || !ip.Is4() { return "", "", fmt.Errorf("server_ip must be one IPv4 address") }
		if cfg.ServerPort < 1 || cfg.ServerPort > 65535 { return "", "", fmt.Errorf("server_port must be 1..65535") }
		endpoint := netip.AddrPortFrom(ip, uint16(cfg.ServerPort)).String()
		return endpoint, endpoint, nil
	}
	if !usingLegacy { return "", "", fmt.Errorf("Windows profile requires server_ip and server_port") }
	front, errFront := netip.ParseAddrPort(strings.TrimSpace(cfg.ServerFront))
	raw, errRaw := netip.ParseAddrPort(strings.TrimSpace(cfg.ServerRaw))
	if errFront != nil || errRaw != nil || !front.Addr().Is4() || !raw.Addr().Is4() { return "", "", fmt.Errorf("legacy server_front/server_raw must both be IPv4 address:port values") }
	if front != raw { return "", "", fmt.Errorf("legacy server_front/server_raw differ; migrate wbd.json to server_ip + server_port for the shared public port") }
	return front.String(), raw.String(), nil
}

func ensureInstallationID(stateDir string) (logicaltunnel.InstallationID, error) {
	if strings.TrimSpace(stateDir) == "" { return "", fmt.Errorf("Windows state directory is required") }
	if err := os.MkdirAll(stateDir, 0o700); err != nil { return "", fmt.Errorf("create Windows state directory: %w", err) }
	path := filepath.Join(stateDir, "installation-id")
	read := func() (logicaltunnel.InstallationID, error) {
		b, err := os.ReadFile(path)
		if err != nil { return "", err }
		id, err := logicaltunnel.ParseInstallationID(strings.TrimSpace(string(b)))
		if err != nil { return "", fmt.Errorf("invalid persisted WBD installation id: %w", err) }
		return id, nil
	}
	if id, err := read(); err == nil { return id, nil } else if !os.IsNotExist(err) { return "", err }
	id, err := logicaltunnel.NewInstallationID()
	if err != nil { return "", fmt.Errorf("generate WBD installation id: %w", err) }
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) { return read() }
		return "", fmt.Errorf("persist WBD installation id: %w", err)
	}
	_, werr := io.WriteString(f, string(id)+"\n")
	cerr := f.Close()
	if werr != nil { _ = os.Remove(path); return "", werr }
	if cerr != nil { _ = os.Remove(path); return "", cerr }
	return id, nil
}

func LoadRuntimeProfile(path, binDir, stateDir string) (windowsruntime.Profile, error) {
	f, err := os.Open(path); if err != nil { return windowsruntime.Profile{}, err }; defer f.Close()
	decoder := json.NewDecoder(f); decoder.DisallowUnknownFields(); var cfg RuntimeProfileFile
	if err := decoder.Decode(&cfg); err != nil { return windowsruntime.Profile{}, fmt.Errorf("decode Windows GUI profile: %w", err) }
	if err := ensureJSONEOF(decoder); err != nil { return windowsruntime.Profile{}, err }
	serverFront, serverRaw, err := resolveServerEndpoints(cfg); if err != nil { return windowsruntime.Profile{}, err }
	if cfg.FEC==""{cfg.FEC="off"};if cfg.IfName==""{cfg.IfName="WBD"};if cfg.MTU==0{cfg.MTU=1400};if cfg.RouteMode==""{cfg.RouteMode=windowsruntime.RouteFull};if cfg.DNSMode==""{cfg.DNSMode=windowsruntime.DNSAuto};if cfg.Lanes==0{cfg.Lanes=1}
	idleTimeoutSeconds := defaultPayloadIdleTimeoutSeconds
	if cfg.IdleTimeout != nil { idleTimeoutSeconds = *cfg.IdleTimeout }
	installationID, err := ensureInstallationID(stateDir); if err != nil { return windowsruntime.Profile{}, err }

	cnSetDir := filepath.Join(stateDir, "ipsets", "cn")
	if portable := os.Getenv("WBD_PORTABLE_DIR"); portable != "" { cnSetDir = filepath.Clean(portable) }
	profile := windowsruntime.Profile{
		BinDir:filepath.Clean(binDir), ServerFront:serverFront, ServerName:cfg.ServerName, RouteKey:cfg.RouteKey, Username:cfg.Username, Password:cfg.Password, ServerRaw:serverRaw, VerifyServer:cfg.VerifyServer, FEC:cfg.FEC, IfName:cfg.IfName, MTU:cfg.MTU, RouteMode:cfg.RouteMode, CNSetDir:cnSetDir, DNSMode:cfg.DNSMode, DNSServer:cfg.DNSServer,
		InstallationID:string(installationID), Lanes:cfg.Lanes, IdleTimeoutSeconds:idleTimeoutSeconds, TunnelIPv4:"",
		TicketPath:filepath.Join(stateDir,"reality-ticket.tmp"), TunnelConfigPath:filepath.Join(stateDir,"tunnel-config.json"), RouteState:filepath.Join(stateDir,"route-state.json"),
	}
	if err:=profile.Validate();err!=nil{return windowsruntime.Profile{},fmt.Errorf("validate Windows GUI profile: %w",err)}
	return profile,nil
}

func ensureJSONEOF(decoder *json.Decoder) error { var extra any; if err:=decoder.Decode(&extra);err==io.EOF{return nil}else if err!=nil{return fmt.Errorf("decode Windows GUI profile trailer: %w",err)};return fmt.Errorf("Windows GUI profile must contain exactly one JSON object") }
