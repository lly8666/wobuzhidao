//go:build !windows

package main

import "os"

func armSupervisorShutdown(stop chan<- os.Signal) {}
