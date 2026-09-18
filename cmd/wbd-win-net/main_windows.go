//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func main() {
	if err := windowsruntime.RunWindowsNetControl(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
