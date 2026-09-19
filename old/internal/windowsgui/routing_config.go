package windowsgui

import (
	"fmt"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func routingPolicyFromConfig(cfg RuntimeProfileFile) (*windowsruntime.RoutingPolicy, error) {
	supplied := 0
	for _, v := range []*bool{cfg.ProxyLAN, cfg.ProxyChina, cfg.ProxyOther} {
		if v != nil {
			supplied++
		}
	}
	if supplied == 0 {
		return nil, nil
	}
	if supplied != 3 {
		return nil, fmt.Errorf("proxy_lan, proxy_china and proxy_other must be set together")
	}
	if strings.TrimSpace(cfg.RouteMode) != "" {
		return nil, fmt.Errorf("new proxy_lan/proxy_china/proxy_other settings cannot be mixed with legacy route_mode")
	}
	return &windowsruntime.RoutingPolicy{ProxyLAN: *cfg.ProxyLAN, ProxyChina: *cfg.ProxyChina, ProxyOther: *cfg.ProxyOther}, nil
}
