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
