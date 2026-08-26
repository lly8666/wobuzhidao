//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/windowsgui"
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

	idHideButton = 1001
	idExitButton = 1002
	idTrayShow   = 2001
	idTrayExit   = 2002

	trayCallbackMessage = wmApp + 1
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
	procLoadIconW              = user32.NewProc("LoadIconW")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procSendMessageW           = user32.NewProc("SendMessageW")
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

var app struct {
	window         uintptr
	status         uintptr
	hideButton     uintptr
	exitButton     uintptr
	icon           uintptr
	font           uintptr
	taskbarCreated uint32
	state          windowsgui.WindowState
}

func main() {
	startMinimized := flag.Bool("start-minimized", false, "start with the main window hidden in the notification area")
	flag.Parse()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	app.state = windowsgui.NewWindowState(*startMinimized)
	if err := run(); err != nil {
		messageBox("WBD Windows GUI", err.Error())
		os.Exit(1)
	}
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
		200, 160, 560, 300,
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
		app.state.Close()
		procShowWindow.Call(hwnd, swHide)
		return 0
	case wmCommand:
		switch lowWord(wParam) {
		case idHideButton:
			minimizeToTray(hwnd)
			return 0
		case idExitButton:
			exitApplication(hwnd)
			return 0
		case idTrayShow:
			restoreFromTray(hwnd)
			return 0
		case idTrayExit:
			exitApplication(hwnd)
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
	case wmDestroy:
		deleteTrayIcon(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func createControls(hwnd uintptr) {
	app.status = createControl(hwnd, "STATIC", "Status: platform UI ready; VPN runtime orchestration is the next checkpoint", 24, 28, 500, 28, 0)
	createControl(hwnd, "STATIC", "Minimize or close this window to keep WBD resident in the notification area. Use the tray menu to restore or explicitly exit.", 24, 68, 500, 56, 0)
	app.hideButton = createControl(hwnd, "BUTTON", "Minimize to tray", 24, 156, 160, 36, idHideButton)
	app.exitButton = createControl(hwnd, "BUTTON", "Exit WBD", 200, 156, 120, 36, idExitButton)
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

func minimizeToTray(hwnd uintptr) {
	app.state.Minimize()
	procShowWindow.Call(hwnd, swHide)
}

func restoreFromTray(hwnd uintptr) {
	app.state.Restore()
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWindow.Call(hwnd)
}

func exitApplication(hwnd uintptr) {
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
