package windowsruntime

import "github.com/lly8666/wobuzhidao/internal/gamepath"

const minimumGameInnerMTU = gamepath.MinimumInnerMTU

func gameDatagramOverhead() int {
	return gamepath.DatagramOverhead()
}

func gameInnerMTUBounds() (int, int, error) {
	return gamepath.InnerMTUBounds()
}

func gameLinkPlaintextMTU(innerMTU int) (int, error) {
	return gamepath.LinkPlaintextMTU(innerMTU)
}
