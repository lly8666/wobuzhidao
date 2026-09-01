//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/ipset"
	"github.com/lly8666/wobuzhidao/internal/windowsbundle"
	"github.com/lly8666/wobuzhidao/internal/windowsdiag"
	"github.com/lly8666/wobuzhidao/internal/windowsgui"
)

var (
	user32Portable = syscall.NewLazyDLL("user32.dll")
	shell32Portable = syscall.NewLazyDLL("shell32.dll")
	messageBoxW = user32Portable.NewProc("MessageBoxW")
	isUserAnAdmin = shell32Portable.NewProc("IsUserAnAdmin")
	shellExecuteW = shell32Portable.NewProc("ShellExecuteW")
)

func main() {
	if elevated, err := ensureElevated(); err != nil {
		showMessage("WBD administrator access", err.Error(), true)
		os.Exit(1)
	} else if elevated {
		return
	}

	profilePath := flag.String("profile", "", "path to the WBD Windows client JSON profile; default is wbd.json beside wbd.exe")
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

// ensureElevated makes the portable client a true double-click entry point.
// It relaunches exactly the same outer EXE with Windows' standard runas verb,
// preserving all arguments and using the EXE directory as the working folder.
// The returned bool is true only in the original unelevated process after the
// elevated child has been launched, so that process should exit immediately.
func ensureElevated() (bool, error) {
	r, _, _ := isUserAnAdmin.Call()
	if r != 0 {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve WBD executable for elevation: %w", err)
	}
	args := make([]string, 0, len(os.Args)-1)
	for _, arg := range os.Args[1:] {
		args = append(args, syscall.EscapeArg(arg))
	}
	params := strings.Join(args, " ")
	runas, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	paramPtr, _ := syscall.UTF16PtrFromString(params)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	result, _, callErr := shellExecuteW.Call(0, uintptr(unsafe.Pointer(runas)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(paramPtr)), uintptr(unsafe.Pointer(dir)), 1)
	if result <= 32 {
		return false, fmt.Errorf("administrator elevation was not started: code=%d error=%v", result, callErr)
	}
	return true, nil
}

func run(profilePath string, selfTest bool, selfTestLog, importCN string, rollbackCN, installNpcap, show bool) error {
	exe, err := os.Executable()
	if err != nil { return fmt.Errorf("resolve portable executable: %w", err) }
	portableDir := filepath.Dir(exe)
	if err := os.Setenv("WBD_PORTABLE_DIR", portableDir); err != nil { return fmt.Errorf("set portable directory: %w", err) }

	programData := os.Getenv("ProgramData")
	if programData == "" { return errors.New("ProgramData is not set") }
	stateDir := filepath.Join(programData, "WBD")
	if err := os.MkdirAll(stateDir, 0o700); err != nil { return err }
	cnDir := portableDir

	modeCount := 0
	for _, active := range []bool{selfTest, importCN != "", rollbackCN, installNpcap} { if active { modeCount++ } }
	if modeCount > 1 { return errors.New("choose only one command mode: self-test, import-cn, rollback-cn, or install-npcap") }

	if importCN != "" {
		path, err := filepath.Abs(importCN); if err != nil { return err }
		f, err := os.Open(path); if err != nil { return err }
		prefixes, parseErr := ipset.ParseCN(f); _ = f.Close(); if parseErr != nil { return parseErr }
		m, err := ipset.WriteCNBundle(cnDir, "manual:"+filepath.Base(path), prefixes); if err != nil { return err }
		showMessage("WBD IP ranges", fmt.Sprintf("IP range update succeeded beside wbd.exe.\n\nIPv4: %d\nIPv6: %d\n\nFiles: cn4.txt, cn6.txt, cn-manifest.json\nThe new list is used on the next Connect.", m.IPv4Count, m.IPv6Count), false)
		return nil
	}
	if rollbackCN {
		if err := ipset.RestorePrevious(cnDir); err != nil { return err }
		m, err := ipset.VerifyCNBundle(cnDir); if err != nil { return err }
		showMessage("WBD IP ranges", fmt.Sprintf("Previous IP range list restored beside wbd.exe.\n\nIPv4: %d\nIPv6: %d\n\nReconnect WBD to apply it.", m.IPv4Count, m.IPv6Count), false)
		return nil
	}

	runtimeInfo, err := windowsbundle.EnsureRuntime()
	if err != nil { return fmt.Errorf("prepare embedded WBD runtime: %w", err) }
	if installNpcap {
		script := filepath.Join(runtimeInfo.Dir, "windows_npcap_prepare.ps1")
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
		if output, err := cmd.CombinedOutput(); err != nil { return fmt.Errorf("Npcap preparation failed: %w: %s", err, string(output)) }
		showMessage("WBD Npcap", "Npcap preparation completed. You can now Connect.", false)
		return nil
	}

	if profilePath == "" {
		candidate := filepath.Join(portableDir, "wbd.json")
		if _, statErr := os.Stat(candidate); statErr == nil { profilePath = candidate } else if !errors.Is(statErr, os.ErrNotExist) { return fmt.Errorf("inspect default profile: %w", statErr) }
	}
	if profilePath != "" { profilePath, err = filepath.Abs(profilePath); if err != nil { return err } }
	if selfTest {
		if profilePath == "" { return errors.New("self-test requires wbd.json beside wbd.exe or -profile <path>") }
		diagState := filepath.Join(stateDir, "diagnostics"); if err := os.MkdirAll(diagState, 0o700); err != nil { return err }
		profile, err := windowsgui.LoadRuntimeProfile(profilePath, runtimeInfo.Dir, diagState); if err != nil { return err }
		result, testErr := windowsdiag.Run(profile, selfTestLog)
		if testErr != nil { showMessage("WBD self-test failed", fmt.Sprintf("The test finished and cleanup was attempted.\n\nSupport log:\n%s\n\nSend this JSONL log for diagnosis.\n\nError: %v", result.LogPath, testErr), true); return testErr }
		showMessage("WBD self-test", fmt.Sprintf("Automatic test passed, including cleanup.\n\nSupport log:\n%s", result.LogPath), false); return nil
	}

	gui := filepath.Join(runtimeInfo.Dir, "wbd-windows-gui.exe")
	args := []string{"-start-minimized=true"}; if show { args[0] = "-start-minimized=false" }; if profilePath != "" { args = append(args, "-profile", profilePath) }
	cmd := exec.Command(gui, args...); cmd.Dir = portableDir; cmd.Env = append(os.Environ(), "WBD_PORTABLE_DIR="+portableDir)
	if err := cmd.Start(); err != nil { return fmt.Errorf("start WBD GUI: %w", err) }
	return nil
}

func showMessage(title, text string, isError bool) {
	// Physical/self-hosted qualification must exercise the exact portable EXE
	// without hanging on modal UI. WBD_HEADLESS changes presentation only: it
	// leaves elevation, runtime, transport, probes and cleanup untouched.
	if strings.TrimSpace(os.Getenv("WBD_HEADLESS")) == "1" {
		if isError {
			fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
		} else {
			fmt.Fprintf(os.Stdout, "%s: %s\n", title, text)
		}
		return
	}
	flags := uintptr(0x40); if isError { flags = 0x10 }
	titlePtr, _ := syscall.UTF16PtrFromString(title); textPtr, _ := syscall.UTF16PtrFromString(text)
	messageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), flags)
}
