//go:build !windows

package windowsruntime

import (
	"errors"
	"os"
)

func isBenignProcessStopError(err error) bool {
	return errors.Is(err, os.ErrProcessDone)
}
