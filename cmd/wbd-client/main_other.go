//go:build !linux && !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	if parseAndHandleVersion() {
		return
	}
	fmt.Fprintln(os.Stderr, "wbd-client: supported product platforms are Linux/OpenWrt and Windows")
	os.Exit(2)
}
