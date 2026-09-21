//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	if parseAndHandleVersion() {
		return
	}
	fmt.Fprintln(os.Stderr, "wbd-server: Linux shared-TUN/raw endpoint is required")
	os.Exit(2)
}
