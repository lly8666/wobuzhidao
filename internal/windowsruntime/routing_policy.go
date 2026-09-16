package windowsruntime

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
