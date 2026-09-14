package main

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
)

const defaultInnerMTU = 1360

// linkPolicyForInnerMTU validates the server's compatibility/default inner MTU
// while keeping the protocol-representable range independent of that value.
// The operator-visible MTU belongs to each client interface; the client
// proposes LINK plaintext MTU = inner MTU + WBDP/Game overhead during LINK_INIT.
// A successful association freezes that exact proposal for its lifetime.
//
// The protocol min/max remain broad enough for different clients to select
// different valid inner MTUs. Actual local carrying capacity is enforced by the
// config-aware validator, which derives a ceiling from -connection-mtu and the
// association's negotiated FEC mode before LINK_ACCEPT can be emitted.
func linkPolicyForInnerMTU(mtu int) (control.LinkPolicy, error) {
	if _, err := gamepath.LinkPlaintextMTU(mtu); err != nil {
		minInner, maxInner, boundsErr := gamepath.InnerMTUBounds()
		if boundsErr != nil {
			return control.LinkPolicy{}, boundsErr
		}
		return control.LinkPolicy{}, fmt.Errorf("-mtu must be %d..%d (inner IP MTU): %w", minInner, maxInner, err)
	}

	minInner, maxInner, err := gamepath.InnerMTUBounds()
	if err != nil {
		return control.LinkPolicy{}, err
	}
	minLink, err := gamepath.LinkPlaintextMTU(minInner)
	if err != nil {
		return control.LinkPolicy{}, err
	}
	maxLink, err := gamepath.LinkPlaintextMTU(maxInner)
	if err != nil {
		return control.LinkPolicy{}, err
	}
	carrierValidator, err := linkConfigCarrierValidator(serverConnectionMTU)
	if err != nil {
		return control.LinkPolicy{}, fmt.Errorf("-connection-mtu: %w", err)
	}

	policy := control.CurrentLinkPolicy()
	policy.MinMTU = uint16(minLink)
	policy.MaxMTU = uint16(maxLink)
	policy.ValidateConfig = carrierValidator
	return policy, nil
}
