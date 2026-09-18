//go:build windows

package main

import (
	"errors"
	"fmt"
	"github.com/lly8666/wobuzhidao/internal/windowsgui"
	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	wsBorder         = 0x00800000
	wsVScroll        = 0x00200000
	esAutoHScroll    = 0x0080
	esPassword       = 0x0020
	lbsNotify        = 0x0001
	cbsDropdownList  = 0x0003
	lbAddString      = 0x0180
	lbResetContent   = 0x0184
	lbSetCurSel      = 0x0186
	lbGetCurSel      = 0x0188
	lbnSelChange     = 1
	cbAddString      = 0x0143
	cbGetCurSel      = 0x0147
	cbSetCurSel      = 0x014e
	idProfileList    = 3000
	idProfileAdd     = 3001
	idProfileSave    = 3002
	idProfileDelete  = 3003
	idProfileCurrent = 3004
)

var procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
var procGetWindowTextW = user32.NewProc("GetWindowTextW")

type profileUIState struct {
	store                                                                                                   windowsgui.ProfileStore
	configPath, editingID                                                                                   string
	list, name, serverIP, serverPort, serverFront, serverRaw, serverName, routeKey, username, password      uintptr
	verify, fec, ifName, mtu, routeMode, dnsMode, dnsServer, lanes, idle, keepalive, rotMin, rotMax, tunnel uintptr
	add, save, del, current                                                                                 uintptr
	editors                                                                                                 []uintptr
}

var profileUI profileUIState

type comboOption struct{ label, value string }

var fecOptions = []comboOption{{"关闭 (off)", "off"}, {"20% 冗余 (20:4)", "20:4"}, {"40% 冗余 (20:8)", "20:8"}, {"50% 冗余 (20:10)", "20:10"}, {"60% 冗余 (20:12)", "20:12"}, {"80% 冗余 (20:16)", "20:16"}, {"100% 冗余 (20:20)", "20:20"}}
var routeOptions = []comboOption{{"全部 IPv4", "Full"}, {"仅国外 IPv4", "Foreign"}, {"仅中国大陆 IPv4", "China"}}
var dnsOptions = []comboOption{{"自动", "Auto"}, {"系统 DNS", "System"}, {"Cloudflare", "Cloudflare"}, {"自定义", "Custom"}}

func defaultProfileConfig() windowsgui.RuntimeProfileFile {
	a, b := false, true
	idle, keepalive := 120, 15
	return windowsgui.RuntimeProfileFile{FEC: "off", IfName: "WBD", MTU: windowsruntime.DefaultTunnelMTU, ProxyLAN: &a, ProxyChina: &b, ProxyOther: &b, DNSMode: windowsruntime.DNSAuto, Lanes: 1, IdleTimeout: &idle, KeepaliveSeconds: &keepalive}
}
const portableProfileID = "portable-wbd-json"

func initializeProfiles() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve GUI executable: %w", err)
	}
	dir := filepath.Dir(exe)
	profileUI.configPath = filepath.Join(dir, "wbd.json")
	cfg, err := windowsgui.ReadRuntimeProfileFile(profileUI.configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg = defaultProfileConfig()
		if err := windowsgui.WriteRuntimeProfileFile(profileUI.configPath, cfg); err != nil {
			return fmt.Errorf("create portable wbd.json: %w", err)
		}
	} else if err != nil {
		return err
	}
	profileUI.store = windowsgui.NewProfileStore()
	profileUI.store.Profiles = []windowsgui.SavedProfile{{ID: portableProfileID, Name: "wbd.json", Config: cfg}}
	profileUI.store.SelectedID = portableProfileID
	profileUI.editingID = portableProfileID
	if err := loadCurrentRuntimeProfile(); err != nil {
		// Keep the editor usable for a new/incomplete local wbd.json. Connect stays
		// disabled until the operator saves a valid configuration.
		app.profileReady = false
		app.profileErr = err
	}
	return nil
}
func loadCurrentRuntimeProfile() error {
	if _, ok := profileUI.store.Selected(); !ok {
		app.profileReady = false
		return fmt.Errorf("portable wbd.json is not loaded")
	}
	app.profilePath = profileUI.configPath
	if err := loadRuntimeProfile(profileUI.configPath); err != nil {
		app.profileReady = false
		app.profileErr = err
		return err
	}
	app.profileErr = nil
	return nil
}
func createStyled(parent uintptr, class, text string, x, y, w, h int, id, style uintptr) uintptr {
	ins, _, _ := procGetModuleHandleW.Call(0)
	r, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(text))), wsChild|wsVisible|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, ins, 0)
	if r != 0 && app.font != 0 {
		procSendMessageW.Call(r, wmSetFont, app.font, 1)
	}
	return r
}
func label(parent uintptr, text string, x, y, w int) uintptr {
	return createStyled(parent, "STATIC", text, x, y, w, 22, 0, 0)
}
func edit(parent uintptr, x, y, w int, password bool) uintptr {
	st := uintptr(wsBorder | esAutoHScroll)
	if password {
		st |= esPassword
	}
	h := createStyled(parent, "EDIT", "", x, y, w, 24, 0, st)
	profileUI.editors = append(profileUI.editors, h)
	return h
}
func combo(parent uintptr, x, y, w int, opts []comboOption) uintptr {
	h := createStyled(parent, "COMBOBOX", "", x, y, w, 180, 0, cbsDropdownList|wsVScroll)
	for _, o := range opts {
		procSendMessageW.Call(h, cbAddString, 0, uintptr(unsafe.Pointer(utf16Ptr(o.label))))
	}
	profileUI.editors = append(profileUI.editors, h)
	return h
}
func setText(h uintptr, s string) {
	if h != 0 {
		procSetWindowTextW.Call(h, uintptr(unsafe.Pointer(utf16Ptr(s))))
	}
}
func getText(h uintptr) string {
	if h == 0 {
		return ""
	}
	n, _, _ := procGetWindowTextLengthW.Call(h)
	b := make([]uint16, n+1)
	procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&b[0])), n+1)
	return syscall.UTF16ToString(b)
}
func setCombo(h uintptr, opts []comboOption, value string) {
	idx := 0
	for i, o := range opts {
		if o.value == value {
			idx = i
			break
		}
	}
	procSendMessageW.Call(h, cbSetCurSel, uintptr(idx), 0)
}
func getCombo(h uintptr, opts []comboOption) string {
	i, _, _ := procSendMessageW.Call(h, cbGetCurSel, 0, 0)
	if int(i) >= 0 && int(i) < len(opts) {
		return opts[i].value
	}
	return opts[0].value
}
func setProfileEditorEnabled(v bool) {
	for _, h := range profileUI.editors {
		setEnabled(h, v)
	}
	setEnabled(profileUI.list, v)
	setEnabled(profileUI.add, v)
	setEnabled(profileUI.save, v)
	setEnabled(profileUI.del, v)
	setEnabled(profileUI.current, v)
}

func createControls(hwnd uintptr) {
	label(hwnd, "便携配置", 18, 18, 80)
	createControl(hwnd, "STATIC", "wbd.json（仅程序目录）", 104, 16, 190, 24, 0)
	profileUI.save = createControl(hwnd, "BUTTON", "保存 wbd.json", 18, 48, 130, 30, idProfileSave)
	x1, lw, ew := 320, 110, 215
	x2 := 675
	row := func(y int, title string, e *uintptr, password bool) {
		label(hwnd, title, x1, y, lw)
		*e = edit(hwnd, x1+lw, y-2, ew, password)
	}
	row2 := func(y int, title string, e *uintptr, password bool) {
		label(hwnd, title, x2, y, lw)
		*e = edit(hwnd, x2+lw, y-2, ew, password)
	}
	row(82, "服务器 IP", &profileUI.serverIP, false)
	row2(82, "服务器端口", &profileUI.serverPort, false)
	row(116, "SNI/服务器名", &profileUI.serverName, false)
	row2(116, "Wintun 名称", &profileUI.ifName, false)
	row(150, "Route Key", &profileUI.routeKey, false)
	row2(150, "用户名", &profileUI.username, false)
	row(184, "密码", &profileUI.password, true)
	label(hwnd, "验证服务器", x2, 184, lw)
	profileUI.verify = createCheckbox(hwnd, "启用证书/身份验证", x2+lw, 180, 215, 28, 0)
	profileUI.editors = append(profileUI.editors, profileUI.verify)
	label(hwnd, "FEC", x1, 218, lw)
	profileUI.fec = combo(hwnd, x1+lw, 214, ew, fecOptions)
	row(252, "连接 MTU", &profileUI.mtu, false)
	label(hwnd, "旧 route_mode（保存时迁移）", x2, 252, lw)
	profileUI.routeMode = combo(hwnd, x2+lw, 248, ew, routeOptions)
	setEnabled(profileUI.routeMode, false)
	label(hwnd, "IP 分流", x1, 286, lw)
	app.proxyLAN = createCheckbox(hwnd, "局域网经过代理", x1+lw, 282, 160, 26, idProxyLAN)
	app.proxyChina = createCheckbox(hwnd, "国内 IPv4 经过代理", x1+lw+160, 282, 175, 26, idProxyChina)
	app.proxyOther = createCheckbox(hwnd, "其他 IPv4 经过代理", x1+lw, 310, 180, 26, idProxyOther)
	profileUI.editors = append(profileUI.editors, app.proxyLAN, app.proxyChina, app.proxyOther)
	label(hwnd, "DNS 模式", x2, 320, lw)
	profileUI.dnsMode = combo(hwnd, x2+lw, 316, ew, dnsOptions)
	row(354, "DNS 服务器", &profileUI.dnsServer, false)
	row2(354, "并发线路数", &profileUI.lanes, false)
	row(388, "空闲超时(秒)", &profileUI.idle, false)
	row2(388, "Keepalive(秒)", &profileUI.keepalive, false)
	row(422, "轮换最小(秒)", &profileUI.rotMin, false)
	row2(422, "轮换最大(秒)", &profileUI.rotMax, false)
	row(456, "兼容 server_front", &profileUI.serverFront, false)
	row2(456, "兼容 server_raw", &profileUI.serverRaw, false)
	row(490, "兼容 tunnel_ipv4", &profileUI.tunnel, false)
	createControl(hwnd, "STATIC", "兼容字段只用于打开旧配置；新配置请使用服务器 IP + 端口。留空的可选数字表示未设置，输入 0 表示显式 0。", 675, 488, 440, 44, 0)
	app.connectButton = createControl(hwnd, "BUTTON", "连接", 320, 548, 92, 34, idConnectButton)
	app.disconnectButton = createControl(hwnd, "BUTTON", "断开", 420, 548, 92, 34, idDisconnectButton)
	app.reconnectButton = createControl(hwnd, "BUTTON", "重连传输", 520, 548, 105, 34, idReconnectButton)
	app.npcapButton = createControl(hwnd, "BUTTON", "安装/修复 Npcap", 635, 548, 140, 34, idNpcapButton)
	app.hideButton = createControl(hwnd, "BUTTON", "最小化到托盘", 785, 548, 130, 34, idHideButton)
	app.exitButton = createControl(hwnd, "BUTTON", "退出 WBD", 925, 548, 100, 34, idExitButton)
	app.status = createControl(hwnd, "STATIC", "状态：未连接", 320, 602, 795, 48, 0)
	createControl(hwnd, "STATIC", "提示：关闭窗口或最小化不会断开；真正退出会先清理路由、DNS 与 IPv6 阻断，再停止运行进程。", 320, 655, 795, 36, 0)
	loadEditor(profileUI.editingID)
	refreshControls()
}
func refreshProfileList() {}
func loadEditor(id string) {
	p, ok := profileUI.store.Find(id)
	if !ok {
		return
	}
	profileUI.editingID = id
	v := windowsgui.EditorValuesFromSavedProfile(p)
		setText(profileUI.serverIP, v.ServerIP)
	setText(profileUI.serverPort, v.ServerPort)
	setText(profileUI.serverFront, v.ServerFront)
	setText(profileUI.serverRaw, v.ServerRaw)
	setText(profileUI.serverName, v.ServerName)
	setText(profileUI.routeKey, v.RouteKey)
	setText(profileUI.username, v.Username)
	setText(profileUI.password, v.Password)
	setChecked(profileUI.verify, v.VerifyServer)
	setCombo(profileUI.fec, fecOptions, v.FEC)
	setText(profileUI.ifName, v.IfName)
	setText(profileUI.mtu, v.MTU)
	setCombo(profileUI.routeMode, routeOptions, v.RouteMode)
	setChecked(app.proxyLAN, v.ProxyLAN)
	setChecked(app.proxyChina, v.ProxyChina)
	setChecked(app.proxyOther, v.ProxyOther)
	setCombo(profileUI.dnsMode, dnsOptions, v.DNSMode)
	setText(profileUI.dnsServer, v.DNSServer)
	setText(profileUI.lanes, v.Lanes)
	setText(profileUI.idle, v.IdleTimeout)
	setText(profileUI.keepalive, v.Keepalive)
	setText(profileUI.rotMin, v.RotationMin)
	setText(profileUI.rotMax, v.RotationMax)
	setText(profileUI.tunnel, v.TunnelIPv4)
}
func editorValues() windowsgui.ProfileEditorValues {
	return windowsgui.ProfileEditorValues{Name: "wbd.json", ServerIP: getText(profileUI.serverIP), ServerPort: getText(profileUI.serverPort), ServerFront: getText(profileUI.serverFront), ServerRaw: getText(profileUI.serverRaw), ServerName: getText(profileUI.serverName), RouteKey: getText(profileUI.routeKey), Username: getText(profileUI.username), Password: getText(profileUI.password), VerifyServer: isChecked(profileUI.verify), FEC: getCombo(profileUI.fec, fecOptions), IfName: getText(profileUI.ifName), MTU: getText(profileUI.mtu), RouteMode: getCombo(profileUI.routeMode, routeOptions), ProxyLAN: isChecked(app.proxyLAN), ProxyChina: isChecked(app.proxyChina), ProxyOther: isChecked(app.proxyOther), DNSMode: getCombo(profileUI.dnsMode, dnsOptions), DNSServer: getText(profileUI.dnsServer), Lanes: getText(profileUI.lanes), IdleTimeout: getText(profileUI.idle), Keepalive: getText(profileUI.keepalive), RotationMin: getText(profileUI.rotMin), RotationMax: getText(profileUI.rotMax), TunnelIPv4: getText(profileUI.tunnel)}
}
func saveEditing(selectCurrent bool) error {
	p, ok := profileUI.store.Find(profileUI.editingID)
	if !ok {
		return fmt.Errorf("portable wbd.json is not loaded")
	}
	var err error
	p, err = editorValues().ApplyToSavedProfile(p)
	if err != nil {
		return err
	}
	p.ID = portableProfileID
	p.Name = "wbd.json"
	profileUI.store.Profiles = []windowsgui.SavedProfile{p}
	profileUI.store.SelectedID = portableProfileID
	profileUI.editingID = portableProfileID
	if err := windowsgui.WriteRuntimeProfileFile(profileUI.configPath, p.Config); err != nil {
		return err
	}
	return loadCurrentRuntimeProfile()
}
func saveActiveProfileFromUI() error { return saveEditing(true) }
func handleProfileCommand(hwnd, wParam uintptr) bool {
	if lowWord(wParam) != idProfileSave {
		return false
	}
	if e := saveEditing(true); e != nil {
		messageBox("保存配置", e.Error())
	} else {
		messageBoxInfo("保存配置", "wbd.json 已保存到程序目录")
	}
	return true
}
func highWord(v uintptr) uintptr   { return (v >> 16) & 0xffff }
func parseDisplayInt(s string) int { v, _ := strconv.Atoi(strings.TrimSpace(s)); return v }
