package windowsgui

import (
	"fmt"
	"strconv"
	"strings"
)

type ProfileEditorValues struct {
	Name, ServerIP, ServerPort, ServerFront, ServerRaw string
	ServerName, RouteKey, Username, Password           string
	VerifyServer                                       bool
	FEC, IfName, MTU, RouteMode                        string
	ProxyLAN, ProxyChina, ProxyOther                   bool
	DNSMode, DNSServer, Lanes                          string
	IdleTimeout, Keepalive, RotationMin, RotationMax   string
	TunnelIPv4                                         string
}

func boolPtr(v bool) *bool { return &v }
func intText(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}
func optIntText(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
func boolValue(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func legacyRoutingDefaults(mode string) (lan, china, other bool) {
	switch strings.TrimSpace(mode) {
	case "Foreign":
		return false, false, true
	case "China":
		return false, true, false
	default:
		return false, true, true
	}
}

func EditorValuesFromSavedProfile(p SavedProfile) ProfileEditorValues {
	c := p.Config
	legacyLAN, legacyChina, legacyOther := legacyRoutingDefaults(c.RouteMode)
	return ProfileEditorValues{Name: p.Name, ServerIP: c.ServerIP, ServerPort: intText(c.ServerPort), ServerFront: c.ServerFront, ServerRaw: c.ServerRaw, ServerName: c.ServerName, RouteKey: c.RouteKey, Username: c.Username, Password: c.Password, VerifyServer: c.VerifyServer, FEC: c.FEC, IfName: c.IfName, MTU: intText(c.MTU), RouteMode: c.RouteMode, ProxyLAN: boolValue(c.ProxyLAN, legacyLAN), ProxyChina: boolValue(c.ProxyChina, legacyChina), ProxyOther: boolValue(c.ProxyOther, legacyOther), DNSMode: c.DNSMode, DNSServer: c.DNSServer, Lanes: intText(c.Lanes), IdleTimeout: optIntText(c.IdleTimeout), Keepalive: optIntText(c.KeepaliveSeconds), RotationMin: optIntText(c.LaneRotationMinSeconds), RotationMax: optIntText(c.LaneRotationMaxSeconds), TunnelIPv4: c.TunnelIPv4}
}
func parseIntField(name, s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	v, e := strconv.Atoi(s)
	if e != nil {
		return 0, fmt.Errorf("%s 必须是整数", name)
	}
	return v, nil
}
func parseOptionalInt(name, s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	v, e := strconv.Atoi(s)
	if e != nil {
		return nil, fmt.Errorf("%s 必须是整数或留空", name)
	}
	return &v, nil
}
func (v ProfileEditorValues) ApplyToSavedProfile(p SavedProfile) (SavedProfile, error) {
	var err error
	c := p.Config
	p.Name = strings.TrimSpace(v.Name)
	if p.Name == "" {
		return p, fmt.Errorf("配置名称不能为空")
	}
	c.ServerIP = strings.TrimSpace(v.ServerIP)
	if c.ServerPort, err = parseIntField("服务器端口", v.ServerPort); err != nil {
		return p, err
	}
	// server_front/server_raw are decode-only migration inputs. As soon as the
	// operator uses the current server_ip + server_port fields, remove the old
	// endpoint pair so a migrated profile cannot become an invalid mixed form.
	if c.ServerIP != "" || c.ServerPort != 0 {
		c.ServerFront = ""
		c.ServerRaw = ""
	} else {
		c.ServerFront = strings.TrimSpace(v.ServerFront)
		c.ServerRaw = strings.TrimSpace(v.ServerRaw)
	}
	c.ServerName = strings.TrimSpace(v.ServerName)
	c.RouteKey = strings.TrimSpace(v.RouteKey)
	c.Username = strings.TrimSpace(v.Username)
	c.Password = v.Password
	c.VerifyServer = v.VerifyServer
	c.FEC = strings.TrimSpace(v.FEC)
	c.IfName = strings.TrimSpace(v.IfName)
	if c.MTU, err = parseIntField("MTU", v.MTU); err != nil {
		return p, err
	}
	c.RouteMode = ""
	c.ProxyLAN = boolPtr(v.ProxyLAN)
	c.ProxyChina = boolPtr(v.ProxyChina)
	c.ProxyOther = boolPtr(v.ProxyOther)
	c.DNSMode = strings.TrimSpace(v.DNSMode)
	c.DNSServer = strings.TrimSpace(v.DNSServer)
	if c.Lanes, err = parseIntField("并发线路数", v.Lanes); err != nil {
		return p, err
	}
	if c.IdleTimeout, err = parseOptionalInt("空闲超时", v.IdleTimeout); err != nil {
		return p, err
	}
	if c.KeepaliveSeconds, err = parseOptionalInt("Keepalive", v.Keepalive); err != nil {
		return p, err
	}
	if c.LaneRotationMinSeconds, err = parseOptionalInt("线路轮换最小秒数", v.RotationMin); err != nil {
		return p, err
	}
	if c.LaneRotationMaxSeconds, err = parseOptionalInt("线路轮换最大秒数", v.RotationMax); err != nil {
		return p, err
	}
	// tunnel_ipv4 is server-assigned authenticated state. Preserve compatibility
	// when reading old files, but never write an operator-edited value back.
	c.TunnelIPv4 = ""
	p.Config = c
	return p, nil
}
