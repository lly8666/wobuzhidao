//go:build windows

package main

import (
	"fmt"
	"os"
	"strconv"

	"golang.org/x/sys/windows"
)

const supervisorEventPrefix = `Local\WBDLinkShutdown-`

func armSupervisorShutdown(stop chan<- os.Signal) {
	name, err := windows.UTF16PtrFromString(supervisorEventPrefix + strconv.Itoa(os.Getpid()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_LINK_SUPERVISOR_EVENT_FAIL", err)
		return
	}
	h, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_LINK_SUPERVISOR_EVENT_FAIL", err)
		return
	}
	go func() {
		defer windows.CloseHandle(h)
		if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
			fmt.Fprintln(os.Stderr, "WBD_LINK_SUPERVISOR_EVENT_FAIL", err)
			return
		}
		select {
		case stop <- supervisorRetireSignal{}:
		default:
		}
	}()
}
