//go:build !linux

package main

import "fmt"

func main() {
	fmt.Println("wbd-ip-gateway-shared is supported on Linux only")
}
