//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/windowsgui"
	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

const (
	csHRedraw = 0x0002
	csVRedraw = 0x0001

	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	bsPushButton       = 0x00000000

	wmCreate     = 0x0001
	wmDestroy    = 0x0002
	wmSize       = 0x0005
	wmClose      = 0x0010
	wmCommand    = 0x0111
	wmSetFont    = 0x0030
	wmLButtonDbl = 0x0203
	wmRButtonUp  = 0x0205
	wmApp        = 0x8000

	sizeMinimized = 1

	swHide    = 0
	swNormal  = 1
	swRestore = 9

	nimAdd     = 0x00000000
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020

	colorWindow    = 5
	idiApplication = 32512
	idcArrow       = 32512
	defaultGUIFont = 17

	idConnectButton    = 1000
	idHideButton       = 1001
	idExitButton       = 1002
	idDisconnectButton = 1003
	idTrayShow         = 2001
	idTrayExit         = 2002

	trayCallbackMessage = wmApp + 1
	runtimeResultMessage = wmApp + 2
	trayIconID           = 1
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procShowWindow             = user32.NewProc("ShowWindow")
	procUpdateWindow           = user32.NewProc("UpdateWindow")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procLoadIconW              = user32.NewProc("LoadIconW")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procSendMessageW           = user32.NewProc("SendMessageW")
	procSetWindowTextW         = user32.NewProc("SetWindowTextW")
	procEnableWindow           = user32.NewProc("EnableWindow")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procGetStockObject    = gdi32.NewProc("GetStockObject")
)

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd     uintptr
	Message  uint32
	_        uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	CbSize     uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type notifyIconData struct {
	CbSize            uint32
	HWnd              uintptr
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             uintptr
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          [16]byte
	HBalloonIcon      uintptr
}

type runtimeResult struct {
	action string
	err    error
}

var app struct {
	window           uintptr
	status           uintptr
	connectButton    uintptr
	disconnectButton uintptr
	hideButton       uintptr
	exitButton       uintptr
	icon             uintptr
	font             uintptr
	taskbarCreated   uint32
	state            windowsgui.WindowState
	controller       *windowsruntime.Controller
	profile          windowsruntime.Profile
	profileReady     bool
	profileErr       error
	profilePath      string
	results          chan runtimeResult
	operation        string
	exitRequested    bool
	cleanupFailed    bool
}

func main() {
	startMinimized := flag.Bool("start-minimized", false, "start with the main window hidden in the notification area")
	profilePath := flag.String("profile", "", "path to the WBD Windows client JSON profile")
	flag.Parse()

	app.state = windowsgui.NewWindowState(*startMinimized)
	app.controller = windowsruntime.NewController(nil, nil, nil)
	app.results = make(chan runtimeResult, 4)
	app.profilePath = *profilePath
	if *profilePath != "" {
		if err := loadRuntimeProfile(*profilePath); err != nil {
			app.profileErr = err
			messageBox("WBD Windows GUI profile", err.Error())
		}
	}

	runtime.LockOSThread()
	exitCode := 0
	if err := run(); err != nil {
		messageBox("WBD Windows GUI", err.Error())
		exitCode = 1
	}
	// Explicit Exit already disconnects before posting WM_QUIT. This is the
	// second safety net for any other controlled process exit, including a
	// Win32 message-loop error: wait for an in-flight lifecycle action, then run
	// the idempotent cleanup path before allowing the process to terminate.
	if err := cleanupBeforeProcessExit(); err != nil {
		messageBox("WBD cleanup before exit", err.Error())
		exitCode = 1
	}
	runtime.UnlockOSThread()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func cleanupBeforeProcessExit() error {
	if app.controller == nil {
		return nil
	}
	if app.operation != "" {
		// If the UI loop itself has ended, no window procedure will consume this
		// result. Wait rather than racing a still-starting Wintun stack with
		// process termination. A failed disconnect is retried below.
		<-app.results
		app.operation = ""
	}
	return app.controller.Disconnect()
}

func loadRuntimeProfile(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve GUI executable: %w", err)
	}
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return fmt.Errorf("ProgramData is not set")
	}
	stateDir := filepath.Join(programData, "WBD")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create WBD state directory: %w", err)
	}
	profile, err := windowsgui.LoadRuntimeProfile(path, filepath.Dir(exe), stateDir)
	if err != nil {
		return err
	}
	app.profile = profile
	app.profileReady = true
	return nil
}

func run() error {
	instance, _, err := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return fmt.Errorf("GetModuleHandleW: %v", err)
	}

	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	app.icon = icon
	app.font = font

	className := utf16Ptr("WBDWindowsGUIWindow")
	wc := wndClassEx{
		CbSize:     uint32(unsafe.Sizeof(wndClassEx{})),
		Style:      csHRedraw | csVRedraw,
		WndProc:    syscall.NewCallback(windowProc),
		Instance:   instance,
		Icon:       icon,
		Cursor:     cursor,
		Background: colorWindow + 1,
		ClassName:  className,
		IconSm:     icon,
	}
	if r, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassExW: %v", callErr)
	}

	app.taskbarCreated = registerWindowMessage("TaskbarCreated")
	title := utf16Ptr("WBD Windows Client")
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow,
		200, 160, 640, 320,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %v", callErr)
	}
	app.window = hwnd

	if app.state.Visible {
		procShowWindow.Call(hwnd, swNormal)
		procUpdateWindow.Call(hwnd)
	} else {
		procShowWindow.Call(hwnd, swHide)
	}

	var m msg
	for {
		r, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			return fmt.Errorf("GetMessageW: %v", callErr)
		}
		if r == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if app.taskbarCreated != 0 && message == app.taskbarCreated {
		_ = addTrayIcon(hwnd)
		return 0
	}

	switch message {
	case wmCreate:
		app.window = hwnd
		createControls(hwnd)
		if err := addTrayIcon(hwnd); err != nil {
			messageBox("WBD Windows GUI", err.Error())
			return ^uintptr(0)
		}
		return 0
	case wmSize:
		if wParam == sizeMinimized {
			minimizeToTray(hwnd)
			return 0
		}
	case wmClose:
		// Window close is intentionally tray-only. It never calls the runtime
		// controller, so an active VPN remains running.
		app.state.Close()
		procShowWindow.Call(hwnd, swHide)
		return 0
	case wmCommand:
		switch lowWord(wParam) {
		case idConnectButton:
			beginConnect(hwnd)
			return 0
		case idDisconnectButton:
			beginDisconnect(hwnd)
			return 0
		case idHideButton:
			minimizeToTray(hwnd)
			return 0
		case idExitButton:
			requestExit(hwnd)
			return 0
		case idTrayShow:
			restoreFromTray(hwnd)
			return 0
		case idTrayExit:
			requestExit(hwnd)
			return 0
		}
	case trayCallbackMessage:
		switch uint32(lParam) {
		case wmLButtonDbl:
			restoreFromTray(hwnd)
			return 0
		case wmRButtonUp:
			showTrayMenu(hwnd)
			return 0
		}
	case runtimeResultMessage:
		handleRuntimeResult(hwnd)
		return 0
	case wmDestroy:
		deleteTrayIcon(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func createControls(hwnd uintptr) {
	status := "Status: disconnected; load a profile with -profile <path> to connect"
	if app.profileReady {
		status = "Status: disconnected; profile loaded"
	} else if app.profileErr != nil {
		status = "Status: disconnected; profile invalid"
	}
	app.status = createControl(hwnd, "STATIC", status, 24, 26, 580, 30, 0)
	createControl(hwnd, "STATIC", "Minimize or X keeps an active VPN running in the tray. Disconnect tears down routes/tunnel but keeps WBD open. Exit performs cleanup before terminating.", 24, 66, 580, 58, 0)
	app.connectButton = createControl(hwnd, "BUTTON", "Connect", 24, 156, 110, 36, idConnectButton)
	app.disconnectButton = createControl(hwnd, "BUTTON", "Disconnect", 146, 156, 110, 36, idDisconnectButton)
	app.hideButton = createControl(hwnd, "BUTTON", "Minimize to tray", 268, 156, 150, 36, idHideButton)
	app.exitButton = createControl(hwnd, "BUTTON", "Exit WBD", 430, 156, 110, 36, idExitButton)
	refreshControls()
}

func createControl(parent uintptr, class, text string, x, y, width, height int, id uintptr) uintptr {
	instance, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		wsChild|wsVisible|bsPushButton,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		parent, id, instance, 0,
	)
	if hwnd != 0 && app.font != 0 {
		procSendMessageW.Call(hwnd, wmSetFont, app.font, 1)
	}
	return hwnd
}

func beginConnect(hwnd uintptr) {
	if app.operation != "" || app.exitRequested {
		return
	}
	if !app.profileReady {
		if app.profileErr != nil {
			messageBox("WBD Windows GUI profile", app.profileErr.Error())
		} else {
			messageBox("WBD Windows GUI", "No runtime profile is loaded. Restart WBD with -profile <path>.")
		}
		return
	}
	app.operation = "connect"
	app.cleanupFailed = false
	setStatus("Status: connecting; Reality admission and physical underlay discovery run before Wintun capture")
	refreshControls()
	go func(profile windowsruntime.Profile) {
		postRuntimeResult(runtimeResult{action: "connect", err: app.controller.Connect(profile)})
	}(app.profile)
}

func beginDisconnect(hwnd uintptr) {
	if app.operation != "" || app.exitRequested {
		return
	}
	launchDisconnect()
}

func launchDisconnect() {
	app.operation = "disconnect"
	if app.exitRequested {
		setStatus("Status: exiting; removing capture routes before stopping Wintun/LINK/DTLS/FakeTCP")
	} else if app.cleanupFailed {
		setStatus("Status: retrying capture-route cleanup")
	} else {
		setStatus("Status: disconnecting; removing capture routes before reverse teardown")
	}
	refreshControls()
	go func() {
		postRuntimeResult(runtimeResult{action: "disconnect", err: app.controller.Disconnect()})
	}()
}

func requestExit(hwnd uintptr) {
	if app.exitRequested {
		return
	}
	app.exitRequested = true
	if app.operation == "connect" {
		setStatus("Status: Exit requested; waiting for connection setup to finish before cleanup")
		refreshControls()
		return
	}
	if app.operation == "disconnect" {
		setStatus("Status: Exit requested; waiting for route cleanup and teardown")
		refreshControls()
		return
	}
	launchDisconnect()
}

func postRuntimeResult(result runtimeResult) {
	app.results <- result
	procPostMessageW.Call(app.window, runtimeResultMessage, 0, 0)
}

func handleRuntimeResult(hwnd uintptr) {
	var result runtimeResult
	select {
	case result = <-app.results:
	default:
		return
	}
	app.operation = ""

	switch result.action {
	case "connect":
		if result.err != nil {
			setStatus("Status: disconnected; connect failed: " + result.err.Error())
			if !app.exitRequested {
				messageBox("WBD Connect failed", result.err.Error())
			}
		} else {
			setStatus("Status: connected; WBD FakeTCP -> DTLS 1.3 -> LINK -> Wintun is active")
		}
		if app.exitRequested {
			launchDisconnect()
			return
		}
	case "disconnect":
		if result.err != nil {
			app.cleanupFailed = true
			setStatus("Status: disconnected, but capture-route cleanup needs retry: " + result.err.Error())
			messageBox("WBD cleanup needs retry", result.err.Error())
			// Do not silently terminate with a broad capture route still pending.
			// A subsequent Disconnect or Exit retries Executor.Stop cleanup.
			app.exitRequested = false
		} else {
			app.cleanupFailed = false
			setStatus("Status: disconnected; routes and WBD runtime are stopped")
			if app.exitRequested {
				finalizeExit(hwnd)
				return
			}
		}
	}
	refreshControls()
}

func refreshControls() {
	busy := app.operation != ""
	connectEnabled := app.profileReady && !busy && !app.exitRequested && !app.cleanupFailed && app.controller.State() == windowsruntime.RuntimeDisconnected
	disconnectEnabled := !busy && !app.exitRequested && (app.cleanupFailed || app.controller.State() == windowsruntime.RuntimeConnected)
	setEnabled(app.connectButton, connectEnabled)
	setEnabled(app.disconnectButton, disconnectEnabled)
	setEnabled(app.hideButton, !app.exitRequested)
	setEnabled(app.exitButton, !app.exitRequested)
}

func setStatus(text string) {
	if app.status == 0 {
		return
	}
	procSetWindowTextW.Call(app.status, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func setEnabled(hwnd uintptr, enabled bool) {
	if hwnd == 0 {
		return
	}
	var value uintptr
	if enabled {
		value = 1
	}
	procEnableWindow.Call(hwnd, value)
}

func minimizeToTray(hwnd uintptr) {
	app.state.Minimize()
	procShowWindow.Call(hwnd, swHide)
}

func restoreFromTray(hwnd uintptr) {
	app.state.Restore()
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWindow.Call(hwnd)
}

func finalizeExit(hwnd uintptr) {
	app.state.Exit()
	deleteTrayIcon(hwnd)
	procPostQuitMessage.Call(0)
}

func addTrayIcon(hwnd uintptr) error {
	nid := notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             hwnd,
		UID:              trayIconID,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: trayCallbackMessage,
		HIcon:            app.icon,
	}
	copyUTF16(nid.SzTip[:], "WBD Windows Client")
	r, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIconW(NIM_ADD): %v", callErr)
	}
	return nil
}

func deleteTrayIcon(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	nid := notifyIconData{
		CbSize: uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:   hwnd,
		UID:    trayIconID,
	}
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func showTrayMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendMenu(menu, mfString, idTrayShow, "Show WBD")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, idTrayExit, "Exit WBD")

	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	procSetForegroundWindow.Call(hwnd)
	procTrackPopupMenu.Call(menu, tpmRightButton|tpmBottomAlign, uintptr(p.X), uintptr(p.Y), 0, hwnd, 0)
}

func appendMenu(menu uintptr, flags, id uintptr, text string) {
	var ptr uintptr
	if text != "" {
		ptr = uintptr(unsafe.Pointer(utf16Ptr(text)))
	}
	procAppendMenuW.Call(menu, flags, id, ptr)
}

func registerWindowMessage(name string) uint32 {
	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16Ptr(name))))
	return uint32(r)
}

func messageBox(title, text string) {
	proc := user32.NewProc("MessageBoxW")
	proc.Call(0, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))), 0x10)
}

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

func copyUTF16(dst []uint16, src string) {
	encoded, err := syscall.UTF16FromString(src)
	if err != nil {
		return
	}
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
		encoded[len(encoded)-1] = 0
	}
	copy(dst, encoded)
}

func lowWord(v uintptr) uintptr { return v & 0xffff }
