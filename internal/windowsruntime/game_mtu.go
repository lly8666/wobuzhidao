package windowsruntime

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/gamepath"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

const minimumGameInnerMTU = gamepath.MinimumInnerMTU

func gameDatagramOverhead() int { return gamepath.DatagramOverhead() }

func gameInnerMTUBounds() (int, int, error) { return gamepath.InnerMTUBounds() }

func gameLinkPlaintextMTU(innerMTU int) (int, error) { return gamepath.LinkPlaintextMTU(innerMTU) }

// gameConnectionMTUBudget applies the product-facing MTU meaning. Profile.MTU
// is the maximum outer connection MTU; it is not the Wintun or LINK MTU. Game
// and FEC consume their own budget only when those features are enabled.
func gameConnectionMTUBudget(connectionMTU int, fecMode string) (pathmtu.Budget, error) {
	fecEnabled := false
	switch fecMode {
	case "", "off":
	case "20:4", "20:8", "20:10", "20:12", "20:16", "20:20":
		fecEnabled = true
	default:
		return pathmtu.Budget{}, fmt.Errorf("unsupported FEC mode %q for MTU budget", fecMode)
	}
	return pathmtu.Derive(connectionMTU, pathmtu.Features{FEC: fecEnabled, Game: true})
}
