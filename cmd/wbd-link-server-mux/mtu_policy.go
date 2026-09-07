package main

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
)

const defaultInnerMTU = 1360

// linkPolicyForInnerMTU keeps the existing LINK wire contract immutable while
// making the server configuration fail closed on an MTU mismatch. The value is
// the complete inner IPv4/IPv6 packet size seen by the WBD virtual interfaces;
// WBD/FEC, Reality/TLS, FakeTCP, outer IP and link-layer overhead are excluded.
func linkPolicyForInnerMTU(mtu int) (control.LinkPolicy, error) {
	policy := control.CurrentLinkPolicy()
	if mtu < int(policy.MinMTU) || mtu > int(policy.MaxMTU) {
		return control.LinkPolicy{}, fmt.Errorf("-mtu must be %d..%d (inner IP MTU)", policy.MinMTU, policy.MaxMTU)
	}
	policy.MinMTU = uint16(mtu)
	policy.MaxMTU = uint16(mtu)
	return policy, nil
}
