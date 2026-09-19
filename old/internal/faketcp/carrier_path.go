package faketcp

import (
	"fmt"
	"net"
)

// InterfaceMTUForIPv4 returns the MTU of the local interface that owns ip.
// Product FakeTCP already binds a concrete raw source IPv4, so this uses the
// actual carrier interface without introducing another negotiated/config MTU.
func InterfaceMTUForIPv4(ip net.IP) (int, error) {
	target := ip.To4()
	if target == nil {
		return 0, fmt.Errorf("%w: carrier source is not IPv4", ErrCarrierPathMTU)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0, fmt.Errorf("enumerate carrier interfaces: %w", err)
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var candidate net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				candidate = v.IP
			case *net.IPAddr:
				candidate = v.IP
			default:
				parsed, _, parseErr := net.ParseCIDR(addr.String())
				if parseErr == nil {
					candidate = parsed
				}
			}
			if candidate == nil || !target.Equal(candidate.To4()) {
				continue
			}
			if _, err := CarrierPayloadBudget(iface.MTU); err != nil {
				return 0, fmt.Errorf("carrier interface %s mtu=%d: %w", iface.Name, iface.MTU, err)
			}
			return iface.MTU, nil
		}
	}
	return 0, fmt.Errorf("%w: no local interface owns %s", ErrCarrierPathMTU, target.String())
}

// EffectiveCarrierMTU applies an optional configured outer connection-MTU
// ceiling to a concrete local interface MTU. A configured value of zero keeps
// legacy standalone behavior. The configured ceiling can only shrink the
// carrier; it can never manufacture a larger path than the local interface.
func EffectiveCarrierMTU(interfaceMTU, configured int) (int, error) {
	if _, err := CarrierPayloadBudget(interfaceMTU); err != nil {
		return 0, fmt.Errorf("carrier interface mtu=%d: %w", interfaceMTU, err)
	}
	if configured == 0 {
		return interfaceMTU, nil
	}
	if configured < 0 {
		return 0, fmt.Errorf("%w: configured connection MTU must be non-negative", ErrCarrierPathMTU)
	}
	if _, err := CarrierPayloadBudget(configured); err != nil {
		return 0, fmt.Errorf("configured connection mtu=%d: %w", configured, err)
	}
	if configured < interfaceMTU {
		return configured, nil
	}
	return interfaceMTU, nil
}

// CarrierMTUForIPv4 resolves the concrete local interface and then applies the
// operator/configured outer connection-MTU ceiling. It intentionally does not
// claim to discover end-to-end PMTU; callers must provide that ceiling when
// the local interface MTU is larger than the real path.
func CarrierMTUForIPv4(ip net.IP, configured int) (int, error) {
	interfaceMTU, err := InterfaceMTUForIPv4(ip)
	if err != nil {
		return 0, err
	}
	return EffectiveCarrierMTU(interfaceMTU, configured)
}
