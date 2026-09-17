//go:build !windows

package main

func configureTunnelInterfaceMTU(_ string, _ int) (func() error, error) {
	return func() error { return nil }, nil
}
