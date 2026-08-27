//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/ipset"
	"github.com/lly8666/wobuzhidao/internal/windowsbundle"
	"github.com/lly8666/wobuzhidao/internal/windowsdiag"
	"github.com/lly8666/wobuzhidao/internal/windowsgui"
)

var (
	user32Portable = syscall.NewLazyDLL("user32.dll")
	messageBoxW     = user32Portable.NewProc("MessageBoxW")
)

func main() {
	profilePath := flag.String("profile", "", "path to the WBD Windows client JSON profile")
	selfTest := flag.Bool("self-test", false, "run full automatic diagnostics then cleanup and exit")
	selfTestLog := flag.String("self-test-log", "", "support JSONL log path; default is a timestamped file under TEMP\\WBD")
	importCN := flag.String("import-cn", "", "manually import a CIDR/APNIC delegated CN IP range file")
	rollbackCN := flag.Bool("rollback-cn", false, "restore the previous validated CN IP range generation")
	installNpcap := flag.Bool("install-npcap", false, "download, verify and launch the pinned personal-use Npcap installer")
	show := flag.Bool("show", false, "show the GUI immediately instead of the default tray-minimized startup")
	flag.Parse()

	if err := run(*profilePath, *selfTest, *selfTestLog, *importCN, *rollbackCN, *installNpcap, *show); err != nil {
		showMessage("WBD", err.Error(), true)
		os.Exit(1)
	}
}

func run(profilePath string, selfTest bool, selfTestLog, importCN string, rollbackCN, installNpcap, show bool) error {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return errors.New("ProgramData is not set")
	}
	stateDir := filepath.Join(programData, "WBD")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	cnDir := filepath.Join(stateDir, "ipsets", "cn")

	modeCount := 0
	for _, active := range []bool{selfTest, importCN != "", rollbackCN, installNpcap} {
		if active { modeCount++ }
	}
	if modeCount > 1 {
		return errors.New("choose only one command mode: self-test, import-cn, rollback-cn, or install-npcap")
	}

	if importCN != "" {
		path, err := filepath.Abs(importCN)
		if err != nil { return err }
		f, err := os.Open(path)
		if err != nil { return err }
		prefixes, parseErr := ipset.ParseCN(f)
		_ = f.Close()
		if parseErr != nil { return parseErr }
		m, err := ipset.WriteCNBundle(cnDir, "manual:"+filepath.Base(path), prefixes)
		if err != nil { return err }
		showMessage("WBD IP ranges", fmt.Sprintf("IP range update succeeded.\n\nIPv4: %d\nIPv6: %d\n\nThe new list is used on the next Connect.", m.IPv4Count, m.IPv6Count), false)
		return nil
	}
	if rollbackCN {
		if err := ipset.RestorePrevious(cnDir); err != nil { return err }
		m, err := ipset.VerifyCNBundle(cnDir)
		if err != nil { return err }
		showMessage("WBD IP ranges", fmt.Sprintf("Previous IP range list restored.\n\nIPv4: %d\nIPv6: %d\n\nReconnect WBD to apply it.", m.IPv4Count, m.IPv6Count), false)
		return nil
	}

	runtimeInfo, err := windowsbundle.EnsureRuntime()
	if err != nil {
		return fmt.Errorf("prepare embedded WBD runtime: %w", err)
	}
	if installNpcap {
		script := filepath.Join(runtimeInfo.Dir, "windows_npcap_prepare.ps1")
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("Npcap preparation failed: %w: %s", err, string(output))
		}
		showMessage("WBD Npcap", "Npcap preparation completed. You can now Connect.", false)
		return nil
	}

	if profilePath != "" {
		profilePath, err = filepath.Abs(profilePath)
		if err != nil { return err }
	}
	if selfTest {
		if profilePath == "" {
			return errors.New("-self-test requires -profile <path>")
		}
		diagState := filepath.Join(stateDir, "diagnostics")
		if err := os.MkdirAll(diagState, 0o700); err != nil { return err }
		profile, err := windowsgui.LoadRuntimeProfile(profilePath, runtimeInfo.Dir, diagState)
		if err != nil { return err }
		result, testErr := windowsdiag.Run(profile, selfTestLog)
		if testErr != nil {
			showMessage("WBD self-test failed", fmt.Sprintf("The test finished and cleanup was attempted.\n\nSupport log:\n%s\n\nSend this JSONL log for diagnosis.\n\nError: %v", result.LogPath, testErr), true)
			return testErr
		}
		showMessage("WBD self-test", fmt.Sprintf("Automatic test passed, including cleanup.\n\nSupport log:\n%s", result.LogPath), false)
		return nil
	}

	gui := filepath.Join(runtimeInfo.Dir, "wbd-windows-gui.exe")
	args := []string{"-start-minimized=true"}
	if show {
		args[0] = "-start-minimized=false"
	}
	if profilePath != "" {
		args = append(args, "-profile", profilePath)
	}
	cmd := exec.Command(gui, args...)
	cmd.Dir = runtimeInfo.Dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start WBD GUI: %w", err)
	}
	return nil
}

func showMessage(title, text string, isError bool) {
	flags := uintptr(0x40) // MB_ICONINFORMATION
	if isError { flags = 0x10 }
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	messageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), flags)
}
