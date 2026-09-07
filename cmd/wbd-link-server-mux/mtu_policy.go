package main

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
)

const defaultInnerMTU = 1360

// linkPolicyForInnerMTU keeps the existing LINK wire contract immutable while
// making the server configuration fail closed on an MTU mismatch. The configured
// value is the complete inner IPv4/IPv6 packet seen by the WBD virtual interfaces;
// the private WBDP+Game envelope is added here before validating LINK plaintext.
func linkPolicyForInnerMTU(mtu int) (control.LinkPolicy, error) {
	linkMTU, err := gamepath.LinkPlaintextMTU(mtu)
	if err != nil {
		minInner, maxInner, boundsErr := gamepath.InnerMTUBounds()
		if boundsErr != nil {
			return control.LinkPolicy{}, boundsErr
		}
		return control.LinkPolicy{}, fmt.Errorf("-mtu must be %d..%d (inner IP MTU): %w", minInner, maxInner, err)
	}
	policy := control.CurrentLinkPolicy()
	policy.MinMTU = uint16(linkMTU)
	policy.MaxMTU = uint16(linkMTU)
	return policy, nil
}
