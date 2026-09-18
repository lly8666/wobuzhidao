//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureTunnelInterfaceMTU(name string, mtu int) (func() error, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("Windows tunnel adapter name is required")
	}
	if mtu < 576 || mtu > 9000 {
		return nil, fmt.Errorf("invalid Windows tunnel MTU %d", mtu)
	}
	var previous uint32
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, found, err := adapterMTUByFriendlyName(name)
		if err != nil {
			return nil, err
		}
		if found {
			previous = got
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("IPv4 interface row for %s did not become ready", name)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := setAdapterMTUNetsh(name, uint32(mtu)); err != nil {
		return nil, fmt.Errorf("apply Windows tunnel MTU %d: %w", mtu, err)
	}
	verify, found, err := adapterMTUByFriendlyName(name)
	if err != nil || !found || verify != uint32(mtu) {
		_ = setAdapterMTUNetsh(name, previous)
		if err != nil {
			return nil, fmt.Errorf("read back Windows tunnel MTU %d: %w", mtu, err)
		}
		return nil, fmt.Errorf("IPv4 MTU readback mismatch: got=%d want=%d", verify, mtu)
	}
	fmt.Fprintf(os.Stderr, "WBD_TUN_WINDOWS_MTU_READY ifname=%s mtu=%d previous=%d verified=1 control=program\n", name, mtu, previous)

	restored := false
	return func() error {
		if restored {
			return nil
		}
		restored = true
		_, found, err := adapterMTUByFriendlyName(name)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if err := setAdapterMTUNetsh(name, previous); err != nil {
			return fmt.Errorf("restore Windows tunnel MTU %d: %w", previous, err)
		}
		verify, found, err := adapterMTUByFriendlyName(name)
		if err != nil {
			return err
		}
		if found && verify != previous {
			return fmt.Errorf("IPv4 MTU restore readback mismatch: got=%d want=%d", verify, previous)
		}
		fmt.Fprintf(os.Stderr, "WBD_TUN_WINDOWS_MTU_RESTORED ifname=%s mtu=%d verified=1 control=program\n", name, previous)
		return nil
	}, nil
}

func setAdapterMTUNetsh(name string, mtu uint32) error {
	cmd := exec.Command("netsh.exe", "interface", "ipv4", "set", "subinterface", name, "mtu="+strconv.FormatUint(uint64(mtu), 10), "store=active")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func adapterMTUByFriendlyName(name string) (uint32, bool, error) {
	size := uint32(16 * 1024)
	for attempt := 0; attempt < 4; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(uint32(windows.AF_INET), windows.GAA_FLAG_INCLUDE_PREFIX, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return 0, false, err
		}
		for a := first; a != nil; a = a.Next {
			friendly := ""
			if a.FriendlyName != nil {
				friendly = windows.UTF16PtrToString(a.FriendlyName)
			}
			if strings.EqualFold(strings.TrimSpace(friendly), name) {
				return a.Mtu, true, nil
			}
		}
		return 0, false, nil
	}
	return 0, false, errors.New("GetAdaptersAddresses buffer size changed repeatedly")
}
