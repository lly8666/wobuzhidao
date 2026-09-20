//go:build !windows

package windowsclient

import "net/netip"

func DiscoverPhysicalUnderlay(netip.Addr, uint32) (PhysicalUnderlay, error) {
	return PhysicalUnderlay{}, ErrPhysicalUnderlayUnsupported
}
