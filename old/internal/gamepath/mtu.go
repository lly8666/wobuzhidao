package gamepath

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

const MinimumInnerMTU = 576

// DatagramOverhead is the private plaintext budget added between the virtual
// interface's complete inner IP packet and LINK plaintext for the Game path.
// It excludes FEC, Reality/DTLS, FakeTCP, outer IP and link-layer overhead.
func DatagramOverhead() int {
	return dataplane.HeaderLen + gamelane.HeaderSize
}

// InnerMTUBounds returns the user-visible inner IP MTU range that fits inside
// the existing LINK MTU policy after the private WBDP+Game envelope is added.
func InnerMTUBounds() (int, int, error) {
	policy := control.CurrentLinkPolicy()
	overhead := DatagramOverhead()
	minInner := MinimumInnerMTU
	if fromLink := int(policy.MinMTU) - overhead; fromLink > minInner {
		minInner = fromLink
	}
	maxInner := int(policy.MaxMTU) - overhead
	if maxInner < minInner {
		return 0, 0, fmt.Errorf("LINK MTU policy %d..%d cannot carry the Game/WBDP overhead %d with minimum inner MTU %d", policy.MinMTU, policy.MaxMTU, overhead, MinimumInnerMTU)
	}
	return minInner, maxInner, nil
}

// LinkPlaintextMTU converts the user-visible inner IP MTU into the immutable
// LINK proposal used by Game lanes. Both endpoints configure the inner value;
// neither endpoint should expose this private envelope adjustment to operators.
func LinkPlaintextMTU(innerMTU int) (int, error) {
	minInner, maxInner, err := InnerMTUBounds()
	if err != nil {
		return 0, err
	}
	if innerMTU < minInner || innerMTU > maxInner {
		policy := control.CurrentLinkPolicy()
		return 0, fmt.Errorf("Game inner MTU %d outside %d..%d; WBDP+Game overhead=%d must keep LINK plaintext within %d..%d", innerMTU, minInner, maxInner, DatagramOverhead(), policy.MinMTU, policy.MaxMTU)
	}
	return innerMTU + DatagramOverhead(), nil
}
