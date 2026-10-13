package windowsclient

import (
    "fmt"
    "net/netip"
)

// BuildCompactNetworkPlan captures public IPv4 with exactly two /1 routes.
// China/LAN/custom Direct prefixes are intentionally never installed in the
// Windows host route table: they are only used by TUNScopedFlows. Existing
// physical connected/on-link LAN routes can remain more specific than /1;
// explicit capture of LAN or DNS is a separate controlled policy.
func BuildCompactNetworkPlan(cfg Config) (NetworkPlan, error) {
    cfg.Direct4 = nil
    cfg.Bypass4 = nil
    plan, err := BuildNetworkPlan(cfg)
    if err != nil { return NetworkPlan{}, err }
    want := []netip.Prefix{
        netip.MustParsePrefix("0.0.0.0/1"),
        netip.MustParsePrefix("128.0.0.0/1"),
    }
    if len(plan.DirectRoutes)!=0 || len(plan.CaptureRoutes)>8 {
        return NetworkPlan{}, fmt.Errorf("%w: compact routing cap exceeded", ErrNetworkPlan)
    }
    for _, prefix := range want {
        found := false
        for _, route := range plan.CaptureRoutes {
            if route.Prefix==prefix { found=true; break }
        }
        if !found {return NetworkPlan{},fmt.Errorf("%w: missing full capture route %v",ErrNetworkPlan,prefix)}
    }
    return plan,nil
}
