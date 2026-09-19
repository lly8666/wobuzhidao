//go:build !windows

package windowsruntime

import "os/exec"

func configureHiddenProcess(cmd *exec.Cmd) {}
