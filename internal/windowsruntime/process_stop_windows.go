//go:build windows

package windowsruntime

import (
	"errors"
	"os"
	"syscall"
)

const windowsErrorInvalidHandle = syscall.Errno(6)

func isBenignProcessStopError(err error) bool {
	return errors.Is(err, os.ErrProcessDone) ||
		errors.Is(err, syscall.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windowsErrorInvalidHandle)
}
