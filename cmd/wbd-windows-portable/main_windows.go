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
	"time"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/ipset"
	"github.com/lly8666/wobuzhidao/internal/windowsdiag"
	"github.com/lly8666/wobuzhidao/internal/windowsgui"
)

var (
	user32Portable  = syscall.NewLazyDLL("user32.dll")
	shell32Portable = syscall.NewLazyDLL("shell32.dll")
	messageBoxW     = user32Portable.NewProc("MessageBoxW")
	isUserAnAdmin   = shell32Portable.NewProc("IsUserAnAdmin")
	shellExecuteW   = shell32Portable.NewProc("ShellExecuteW")
)

func main() {
	if elevated, err := ensureElevated(); err != nil {
		showMessage("WBD administrator access", err.Error(), true)
		os.Exit(1)
	} else if elevated {
		return
	}

	selfTest := flag.Bool("self-test", false, "run full automatic diagnostics then cleanup and exit")
	selfTestLog := flag.String("self-test-log", "", "support JSONL log path; default is logs\\self-test-*.jsonl beside wbd.exe")
	importCN := flag.String("import-cn", "", "manually import a CIDR/APNIC delegated CN IP range file")
	rollbackCN := flag.Bool("rollback-cn", false, "restore the previous validated CN IP range generation")
	installNpcap := flag.Bool("install-npcap", false, "download, verify and launch the pinned personal-use Npcap installer")
	show := flag.Bool("show", false, "show the GUI immediately instead of the default tray-minimized startup")
	flag.Parse()

	if err := run(*selfTest, *selfTestLog, *importCN, *rollbackCN, *installNpcap, *show); err != nil {
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

func timestampedLogPath(portableDir, prefix, ext string) (string, error) {
	logDir := filepath.Join(portableDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(logDir, prefix+time.Now().Format("20060102-150405.000")+ext), nil
}

func run(selfTest bool, selfTestLog, importCN string, rollbackCN, installNpcap, show bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve portable executable: %w", err)
	}
	portableDir := filepath.Dir(exe)
	if err := os.Setenv("WBD_PORTABLE_DIR", portableDir); err != nil {
		return fmt.Errorf("set portable directory: %w", err)
	}

	// Portable mode is strict: every WBD-owned config/state/log/runtime file
	// lives beside wbd.exe (or in a child directory of that folder). Never use
	// ProgramData, LocalAppData, TEMP, or another machine-wide/user-wide store.
	stateDir := portableDir
	cnDir := portableDir

	modeCount := 0
	for _, active := range []bool{selfTest, importCN != "", rollbackCN, installNpcap} {
		if active {
			modeCount++
		}
	}
	if modeCount > 1 {
		return errors.New("choose only one command mode: self-test, import-cn, rollback-cn, or install-npcap")
	}

	if importCN != "" {
		path, err := filepath.Abs(importCN)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		prefixes, parseErr := ipset.ParseCN(f)
		_ = f.Close()
		if parseErr != nil {
			return parseErr
		}
		m, err := ipset.WriteCNBundle(cnDir, "manual:"+filepath.Base(path), prefixes)
		if err != nil {
			return err
		}
		showMessage("WBD IP ranges", fmt.Sprintf("IP range update succeeded beside wbd.exe.\n\nIPv4: %d\nIPv6: %d\n\nFiles: cn4.txt, cn6.txt, cn-manifest.json\nThe new list is used on the next Connect.", m.IPv4Count, m.IPv6Count), false)
		return nil
	}
	if rollbackCN {
		if err := ipset.RestorePrevious(cnDir); err != nil {
			return err
		}
		m, err := ipset.VerifyCNBundle(cnDir)
		if err != nil {
			return err
		}
		showMessage("WBD IP ranges", fmt.Sprintf("Previous IP range list restored beside wbd.exe.\n\nIPv4: %d\nIPv6: %d\n\nReconnect WBD to apply it.", m.IPv4Count, m.IPv6Count), false)
		return nil
	}

	runtimeDir := portableDir
	if err := validateInstalledRuntime(runtimeDir); err != nil {
		return err
	}
	if installNpcap {
		script := filepath.Join(runtimeDir, "windows_npcap_prepare.ps1")
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("Npcap preparation failed: %w: %s", err, string(output))
		}
		showMessage("WBD Npcap", "Npcap preparation completed. You can now Connect.", false)
		return nil
	}

	profilePath := filepath.Join(portableDir, "wbd.json")
	if selfTest {
		if profilePath == "" {
			return errors.New("self-test requires wbd.json beside wbd.exe")
		}
		if strings.TrimSpace(selfTestLog) == "" {
			selfTestLog, err = timestampedLogPath(portableDir, "self-test-", ".jsonl")
			if err != nil {
				return fmt.Errorf("prepare self-test log beside wbd.exe: %w", err)
			}
		}
		profile, err := windowsgui.LoadRuntimeProfile(profilePath, runtimeDir, stateDir)
		if err != nil {
			return err
		}
		result, testErr := windowsdiag.Run(profile, selfTestLog)
		if testErr != nil {
			showMessage("WBD self-test failed", fmt.Sprintf("The test finished and cleanup was attempted.\n\nSupport log:\n%s\n\nSend this JSONL log for diagnosis.\n\nError: %v", result.LogPath, testErr), true)
			return testErr
		}
		showMessage("WBD self-test", fmt.Sprintf("Automatic test passed, including cleanup.\n\nSupport log:\n%s", result.LogPath), false)
		return nil
	}

	gui := filepath.Join(runtimeDir, "wbd-windows-gui.exe")
	args := []string{"-start-minimized=true"}
	if show {
		args[0] = "-start-minimized=false"
	}
	logPath, err := timestampedLogPath(portableDir, "runtime-", ".log")
	if err != nil {
		return fmt.Errorf("prepare runtime log beside wbd.exe: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open runtime log %s: %w", logPath, err)
	}
	cmd := exec.Command(gui, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Dir = portableDir
	cmd.Env = append(os.Environ(), "WBD_PORTABLE_DIR="+portableDir, "WBD_RUNTIME_LOG="+logPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start WBD GUI: %w", err)
	}
	_ = logFile.Close()
	return nil
}

var requiredInstalledRuntimeFiles = []string{
	"wbd-reality-front.exe", "wbd-faketcp.exe", "wbd_dtls_shim.exe", "wbd-link-proxy.exe", "wbd-game-lane-client.exe", "wbd-tun.exe", "wbd-windows-gui.exe",
	"wintun.dll", "windows_tun_route.ps1", "windows_tun_rebind.ps1", "windows_ipv6_killswitch.ps1", "windows_faketcp_underlay.ps1", "windows_npcap_prepare.ps1",
}

func validateInstalledRuntime(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("WBD 安装目录为空")
	}
	for _, name := range requiredInstalledRuntimeFiles {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() {
			return fmt.Errorf("WBD 安装目录缺少运行文件 %s: %v", name, err)
		}
	}
	return nil
}

func showMessage(title, text string, isError bool) {
	flags := uintptr(0x40)
	if isError {
		flags = 0x10
	}
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	messageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), flags)
}
