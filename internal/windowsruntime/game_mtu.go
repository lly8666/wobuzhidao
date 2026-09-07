package windowsruntime

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

const minimumGameInnerMTU = 576

func gameDatagramOverhead() int {
	return dataplane.HeaderLen + gamelane.HeaderSize
}

func gameInnerMTUBounds() (int, int, error) {
	policy := control.CurrentLinkPolicy()
	overhead := gameDatagramOverhead()
	minInner := minimumGameInnerMTU
	if fromLink := int(policy.MinMTU) - overhead; fromLink > minInner {
		minInner = fromLink
	}
	maxInner := int(policy.MaxMTU) - overhead
	if maxInner < minInner {
		return 0, 0, fmt.Errorf("LINK MTU policy %d..%d cannot carry the Game/WBDP overhead %d with minimum inner MTU %d", policy.MinMTU, policy.MaxMTU, overhead, minimumGameInnerMTU)
	}
	return minInner, maxInner, nil
}

func gameLinkPlaintextMTU(innerMTU int) (int, error) {
	minInner, maxInner, err := gameInnerMTUBounds()
	if err != nil {
		return 0, err
	}
	if innerMTU < minInner || innerMTU > maxInner {
		policy := control.CurrentLinkPolicy()
		return 0, fmt.Errorf("Game inner MTU %d outside %d..%d; WBDP+Game overhead=%d must keep LINK plaintext within %d..%d", innerMTU, minInner, maxInner, gameDatagramOverhead(), policy.MinMTU, policy.MaxMTU)
	}
	return innerMTU + gameDatagramOverhead(), nil
}
