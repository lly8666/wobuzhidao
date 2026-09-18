//go:build !windows

package main

import (
	"errors"
	"net/netip"
)

func mutateIPv4Route(action string, prefix netip.Prefix, ifIndex uint32, nextHop netip.Addr, metric uint32) error {
	return errors.New("wbd-route-batch is only supported on Windows")
}
