//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "wbd-server: Linux shared-TUN/raw endpoint is required")
	os.Exit(2)
}
