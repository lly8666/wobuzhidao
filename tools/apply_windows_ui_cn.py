from pathlib import Path


def replace_once(path, old, new):
    p=Path(path); s=p.read_text(encoding='utf-8'); n=s.count(old)
    if n != 1: raise SystemExit(f'{path}: expected one occurrence got {n}: {old[:100]!r}')
    p.write_text(s.replace(old,new,1),encoding='utf-8')

editor=r'''package windowsgui

import (
    "fmt"
    "strconv"
    "strings"
)

type ProfileEditorValues struct {
    Name, ServerIP, ServerPort, ServerFront, ServerRaw string
    ServerName, RouteKey, Username, Password string
    VerifyServer bool
    FEC, IfName, MTU, RouteMode string
    ProxyLAN, ProxyChina, ProxyOther bool
    DNSMode, DNSServer, Lanes string
    IdleTimeout, Keepalive, RotationMin, RotationMax string
    TunnelIPv4 string
}

func boolPtr(v bool)*bool{return &v}
func intText(v int)string{if v==0{return ""};return strconv.Itoa(v)}
func optIntText(v *int)string{if v==nil{return ""};return strconv.Itoa(*v)}
func boolValue(v *bool, fallback bool)bool{if v==nil{return fallback};return *v}

func EditorValuesFromSavedProfile(p SavedProfile) ProfileEditorValues {
    c:=p.Config
    return ProfileEditorValues{Name:p.Name,ServerIP:c.ServerIP,ServerPort:intText(c.ServerPort),ServerFront:c.ServerFront,ServerRaw:c.ServerRaw,ServerName:c.ServerName,RouteKey:c.RouteKey,Username:c.Username,Password:c.Password,VerifyServer:c.VerifyServer,FEC:c.FEC,IfName:c.IfName,MTU:intText(c.MTU),RouteMode:c.RouteMode,ProxyLAN:boolValue(c.ProxyLAN,false),ProxyChina:boolValue(c.ProxyChina,true),ProxyOther:boolValue(c.ProxyOther,true),DNSMode:c.DNSMode,DNSServer:c.DNSServer,Lanes:intText(c.Lanes),IdleTimeout:optIntText(c.IdleTimeout),Keepalive:optIntText(c.KeepaliveSeconds),RotationMin:optIntText(c.LaneRotationMinSeconds),RotationMax:optIntText(c.LaneRotationMaxSeconds),TunnelIPv4:c.TunnelIPv4}
}
func parseIntField(name,s string)(int,error){s=strings.TrimSpace(s);if s==""{return 0,nil};v,e:=strconv.Atoi(s);if e!=nil{return 0,fmt.Errorf("%s 必须是整数",name)};return v,nil}
func parseOptionalInt(name,s string)(*int,error){s=strings.TrimSpace(s);if s==""{return nil,nil};v,e:=strconv.Atoi(s);if e!=nil{return nil,fmt.Errorf("%s 必须是整数或留空",name)};return &v,nil}
func (v ProfileEditorValues) ApplyToSavedProfile(p SavedProfile)(SavedProfile,error){
    var err error
    c:=p.Config
    p.Name=strings.TrimSpace(v.Name);if p.Name==""{return p,fmt.Errorf("配置名称不能为空")}
    c.ServerIP=strings.TrimSpace(v.ServerIP);if c.ServerPort,err=parseIntField("服务器端口",v.ServerPort);err!=nil{return p,err}
    c.ServerFront=strings.TrimSpace(v.ServerFront);c.ServerRaw=strings.TrimSpace(v.ServerRaw);c.ServerName=strings.TrimSpace(v.ServerName);c.RouteKey=strings.TrimSpace(v.RouteKey);c.Username=strings.TrimSpace(v.Username);c.Password=v.Password;c.VerifyServer=v.VerifyServer;c.FEC=strings.TrimSpace(v.FEC);c.IfName=strings.TrimSpace(v.IfName)
    if c.MTU,err=parseIntField("MTU",v.MTU);err!=nil{return p,err};c.RouteMode=strings.TrimSpace(v.RouteMode);c.ProxyLAN=boolPtr(v.ProxyLAN);c.ProxyChina=boolPtr(v.ProxyChina);c.ProxyOther=boolPtr(v.ProxyOther);c.DNSMode=strings.TrimSpace(v.DNSMode);c.DNSServer=strings.TrimSpace(v.DNSServer)
    if c.Lanes,err=parseIntField("并发线路数",v.Lanes);err!=nil{return p,err};if c.IdleTimeout,err=parseOptionalInt("空闲超时",v.IdleTimeout);err!=nil{return p,err};if c.KeepaliveSeconds,err=parseOptionalInt("Keepalive",v.Keepalive);err!=nil{return p,err};if c.LaneRotationMinSeconds,err=parseOptionalInt("线路轮换最小秒数",v.RotationMin);err!=nil{return p,err};if c.LaneRotationMaxSeconds,err=parseOptionalInt("线路轮换最大秒数",v.RotationMax);err!=nil{return p,err};c.TunnelIPv4=strings.TrimSpace(v.TunnelIPv4)
    p.Config=c;return p,nil
}
'''
Path('internal/windowsgui/editor.go').write_text(editor,encoding='utf-8')

test=r'''package windowsgui
import("reflect";"testing")
func TestProfileEditorRoundTripsEveryConfigFieldAndExplicitZero(t *testing.T){
    z:=0; idle:=77; a:=true;b:=false
    p:=SavedProfile{ID:"id",Name:"东京",Config:RuntimeProfileFile{ServerIP:"198.51.100.8",ServerPort:443,ServerFront:"",ServerRaw:"",ServerName:"sni",RouteKey:"rk",Username:"u",Password:"p",VerifyServer:true,FEC:"20:10",IfName:"WBD-X",MTU:1350,RouteMode:"Foreign",ProxyLAN:&a,ProxyChina:&b,ProxyOther:&a,DNSMode:"Custom",DNSServer:"9.9.9.9",Lanes:4,IdleTimeout:&idle,KeepaliveSeconds:&z,LaneRotationMinSeconds:&idle,LaneRotationMaxSeconds:&z,TunnelIPv4:"10.0.0.2/30"}}
    v:=EditorValuesFromSavedProfile(p); if v.Keepalive!="0"||v.RotationMax!="0"{t.Fatalf("explicit zero lost: %+v",v)}
    got,err:=v.ApplyToSavedProfile(p);if err!=nil{t.Fatal(err)};if !reflect.DeepEqual(got,p){t.Fatalf("roundtrip\n got=%+v\nwant=%+v",got,p)}
}
func TestProfileEditorBlankOptionalIntsRemainNil(t *testing.T){
    p:=SavedProfile{ID:"id",Name:"A"}; v:=EditorValuesFromSavedProfile(p);v.Name="A"; got,err:=v.ApplyToSavedProfile(p);if err!=nil{t.Fatal(err)}
    if got.Config.IdleTimeout!=nil||got.Config.KeepaliveSeconds!=nil||got.Config.LaneRotationMinSeconds!=nil||got.Config.LaneRotationMaxSeconds!=nil{t.Fatalf("blank optional fields became explicit values: %+v",got.Config)}
}
'''
Path('internal/windowsgui/editor_test.go').write_text(test,encoding='utf-8')

ui=r'''//go:build windows
package main

import(
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "unsafe"
    "github.com/lly8666/wobuzhidao/internal/windowsgui"
    "github.com/lly8666/wobuzhidao/internal/windowsruntime"
)
const(
    wsBorder=0x00800000; wsVScroll=0x00200000; esAutoHScroll=0x0080; esPassword=0x0020; lbsNotify=0x0001; cbsDropdownList=0x0003
    lbAddString=0x0180;lbResetContent=0x0184;lbSetCurSel=0x0186;lbGetCurSel=0x0188;lbnSelChange=1
    cbAddString=0x0143;cbGetCurSel=0x0147;cbSetCurSel=0x014e
    idProfileList=3000;idProfileAdd=3001;idProfileSave=3002;idProfileDelete=3003;idProfileCurrent=3004
)
var procGetWindowTextLengthW=user32.NewProc("GetWindowTextLengthW")
var procGetWindowTextW=user32.NewProc("GetWindowTextW")
type profileUIState struct{
    store windowsgui.ProfileStore; storePath,activePath,editingID string
    list,name,serverIP,serverPort,serverFront,serverRaw,serverName,routeKey,username,password uintptr
    verify,fec,ifName,mtu,routeMode,dnsMode,dnsServer,lanes,idle,keepalive,rotMin,rotMax,tunnel uintptr
    add,save,del,current uintptr
    editors []uintptr
}
var profileUI profileUIState

type comboOption struct{label,value string}
var fecOptions=[]comboOption{{"关闭","off"},{"弱保护 1.5x (20:10)","20:10"},{"强保护 2x (20:20)","20:20"}}
var routeOptions=[]comboOption{{"全部 IPv4","Full"},{"仅国外 IPv4","Foreign"},{"仅中国大陆 IPv4","China"}}
var dnsOptions=[]comboOption{{"自动","Auto"},{"系统 DNS","System"},{"Cloudflare","Cloudflare"},{"自定义","Custom"}}

func defaultProfileConfig() windowsgui.RuntimeProfileFile{a,b:=false,true;return windowsgui.RuntimeProfileFile{FEC:"off",IfName:"WBD",MTU:windowsruntime.DefaultTunnelMTU,RouteMode:windowsruntime.RouteFull,ProxyLAN:&a,ProxyChina:&b,ProxyOther:&b,DNSMode:windowsruntime.DNSAuto,Lanes:1}}
func initializeProfiles(importPath string) error{
    pd:=os.Getenv("ProgramData");if pd==""{return fmt.Errorf("ProgramData 未设置")};dir:=filepath.Join(pd,"WBD");if err:=os.MkdirAll(dir,0o700);err!=nil{return err}
    profileUI.storePath=filepath.Join(dir,"profiles.json");profileUI.activePath=filepath.Join(dir,"active-profile.json")
    s,err:=windowsgui.LoadProfileStore(profileUI.storePath);if err!=nil{return err};profileUI.store=s
    if len(s.Profiles)==0 && strings.TrimSpace(importPath)!="" {p,e:=windowsgui.ImportRuntimeProfile(importPath,"已导入服务器");if e!=nil{return e};profileUI.store.Profiles=append(profileUI.store.Profiles,p);profileUI.store.SelectedID=p.ID}
    if len(profileUI.store.Profiles)==0 {p,e:=profileUI.store.Add("新服务器",defaultProfileConfig());if e!=nil{return e};profileUI.store.SelectedID=p.ID}
    if err:=windowsgui.SaveProfileStore(profileUI.storePath,profileUI.store);err!=nil{return err};profileUI.editingID=profileUI.store.SelectedID
    _=loadCurrentRuntimeProfile();return nil
}
func loadCurrentRuntimeProfile()error{
    p,ok:=profileUI.store.Selected();if !ok{app.profileReady=false;return fmt.Errorf("没有当前服务器配置")}
    if err:=windowsgui.WriteRuntimeProfileFile(profileUI.activePath,p.Config);err!=nil{app.profileReady=false;app.profileErr=err;return err}
    app.profilePath=profileUI.activePath
    if err:=loadRuntimeProfile(profileUI.activePath);err!=nil{app.profileReady=false;app.profileErr=err;return err};app.profileErr=nil;return nil
}
func createStyled(parent uintptr,class,text string,x,y,w,h int,id,style uintptr)uintptr{ins,_,_:=procGetModuleHandleW.Call(0);r,_,_:=procCreateWindowExW.Call(0,uintptr(unsafe.Pointer(utf16Ptr(class))),uintptr(unsafe.Pointer(utf16Ptr(text))),wsChild|wsVisible|style,uintptr(x),uintptr(y),uintptr(w),uintptr(h),parent,id,ins,0);if r!=0&&app.font!=0{procSendMessageW.Call(r,wmSetFont,app.font,1)};return r}
func label(parent uintptr,text string,x,y,w int)uintptr{return createStyled(parent,"STATIC",text,x,y,w,22,0,0)}
func edit(parent uintptr,x,y,w int,password bool)uintptr{st:=uintptr(wsBorder|esAutoHScroll);if password{st|=esPassword};h:=createStyled(parent,"EDIT","",x,y,w,24,0,st);profileUI.editors=append(profileUI.editors,h);return h}
func combo(parent uintptr,x,y,w int,opts []comboOption)uintptr{h:=createStyled(parent,"COMBOBOX","",x,y,w,180,0,cbsDropdownList|wsVScroll);for _,o:=range opts{procSendMessageW.Call(h,cbAddString,0,uintptr(unsafe.Pointer(utf16Ptr(o.label))))};profileUI.editors=append(profileUI.editors,h);return h}
func setText(h uintptr,s string){if h!=0{procSetWindowTextW.Call(h,uintptr(unsafe.Pointer(utf16Ptr(s))))}}
func getText(h uintptr)string{if h==0{return ""};n,_,_:=procGetWindowTextLengthW.Call(h);b:=make([]uint16,n+1);procGetWindowTextW.Call(h,uintptr(unsafe.Pointer(&b[0])),n+1);return syscall.UTF16ToString(b)}
func setCombo(h uintptr,opts []comboOption,value string){idx:=0;for i,o:=range opts{if o.value==value{idx=i;break}};procSendMessageW.Call(h,cbSetCurSel,uintptr(idx),0)}
func getCombo(h uintptr,opts []comboOption)string{i,_,_:=procSendMessageW.Call(h,cbGetCurSel,0,0);if int(i)>=0&&int(i)<len(opts){return opts[i].value};return opts[0].value}
func setProfileEditorEnabled(v bool){for _,h:=range profileUI.editors{setEnabled(h,v)};setEnabled(profileUI.list,v);setEnabled(profileUI.add,v);setEnabled(profileUI.save,v);setEnabled(profileUI.del,v);setEnabled(profileUI.current,v)}

func createControls(hwnd uintptr){
    label(hwnd,"服务器",18,18,80); profileUI.add=createControl(hwnd,"BUTTON","新增",18,44,60,28,idProfileAdd);profileUI.save=createControl(hwnd,"BUTTON","保存",84,44,60,28,idProfileSave);profileUI.del=createControl(hwnd,"BUTTON","删除",150,44,60,28,idProfileDelete);profileUI.current=createControl(hwnd,"BUTTON","设为当前",216,44,82,28,idProfileCurrent)
    profileUI.list=createStyled(hwnd,"LISTBOX","",18,80,280,565,idProfileList,wsBorder|wsVScroll|lbsNotify)
    x1,lw,ew:=320,110,215;x2:=675
    row:=func(y int,title string,e *uintptr,password bool){label(hwnd,title,x1,y,lw);*e=edit(hwnd,x1+lw,y-2,ew,password)}
    row2:=func(y int,title string,e *uintptr,password bool){label(hwnd,title,x2,y,lw);*e=edit(hwnd,x2+lw,y-2,ew,password)}
    row(82,"配置名称",&profileUI.name,false);row2(82,"服务器 IP",&profileUI.serverIP,false)
    row(116,"服务器端口",&profileUI.serverPort,false);row2(116,"SNI/服务器名",&profileUI.serverName,false)
    row(150,"Route Key",&profileUI.routeKey,false);row2(150,"用户名",&profileUI.username,false)
    row(184,"密码",&profileUI.password,true);label(hwnd,"验证服务器",x2,184,lw);profileUI.verify=createCheckbox(hwnd,"启用证书/身份验证",x2+lw,180,215,28,0);profileUI.editors=append(profileUI.editors,profileUI.verify)
    label(hwnd,"FEC",x1,218,lw);profileUI.fec=combo(hwnd,x1+lw,214,ew,fecOptions);label(hwnd,"Wintun 名称",x2,218,lw);profileUI.ifName=edit(hwnd,x2+lw,214,ew,false)
    row(252,"连接 MTU",&profileUI.mtu,false);label(hwnd,"传统路由模式",x2,252,lw);profileUI.routeMode=combo(hwnd,x2+lw,248,ew,routeOptions)
    label(hwnd,"IP 分流",x1,286,lw);app.proxyLAN=createCheckbox(hwnd,"局域网经过代理",x1+lw,282,160,26,idProxyLAN);app.proxyChina=createCheckbox(hwnd,"国内 IPv4 经过代理",x1+lw+160,282,175,26,idProxyChina);app.proxyOther=createCheckbox(hwnd,"其他 IPv4 经过代理",x1+lw,310,180,26,idProxyOther);profileUI.editors=append(profileUI.editors,app.proxyLAN,app.proxyChina,app.proxyOther)
    label(hwnd,"DNS 模式",x2,320,lw);profileUI.dnsMode=combo(hwnd,x2+lw,316,ew,dnsOptions);row(354,"DNS 服务器",&profileUI.dnsServer,false);row2(354,"并发线路数",&profileUI.lanes,false)
    row(388,"空闲超时(秒)",&profileUI.idle,false);row2(388,"Keepalive(秒)",&profileUI.keepalive,false)
    row(422,"轮换最小(秒)",&profileUI.rotMin,false);row2(422,"轮换最大(秒)",&profileUI.rotMax,false)
    row(456,"兼容 server_front",&profileUI.serverFront,false);row2(456,"兼容 server_raw",&profileUI.serverRaw,false)
    row(490,"兼容 tunnel_ipv4",&profileUI.tunnel,false)
    createControl(hwnd,"STATIC","兼容字段只用于打开旧配置；新配置请使用服务器 IP + 端口。留空的可选数字表示未设置，输入 0 表示显式 0。",675,488,440,44,0)
    app.connectButton=createControl(hwnd,"BUTTON","连接",320,548,92,34,idConnectButton);app.disconnectButton=createControl(hwnd,"BUTTON","断开",420,548,92,34,idDisconnectButton);app.reconnectButton=createControl(hwnd,"BUTTON","重连传输",520,548,105,34,idReconnectButton);app.npcapButton=createControl(hwnd,"BUTTON","安装/修复 Npcap",635,548,140,34,idNpcapButton);app.hideButton=createControl(hwnd,"BUTTON","最小化到托盘",785,548,130,34,idHideButton);app.exitButton=createControl(hwnd,"BUTTON","退出 WBD",925,548,100,34,idExitButton)
    app.status=createControl(hwnd,"STATIC","状态：未连接",320,602,795,48,0);createControl(hwnd,"STATIC","提示：关闭窗口或最小化不会断开；真正退出会先清理路由、DNS 与 IPv6 阻断，再停止运行进程。",320,655,795,36,0)
    refreshProfileList();loadEditor(profileUI.editingID);refreshControls()
}
func refreshProfileList(){if profileUI.list==0{return};procSendMessageW.Call(profileUI.list,lbResetContent,0,0);sel:=0;for i,p:=range profileUI.store.Profiles{prefix:="  ";if p.ID==profileUI.store.SelectedID{prefix="● "};procSendMessageW.Call(profileUI.list,lbAddString,0,uintptr(unsafe.Pointer(utf16Ptr(prefix+p.Name))));if p.ID==profileUI.editingID{sel=i}};procSendMessageW.Call(profileUI.list,lbSetCurSel,uintptr(sel),0)}
func loadEditor(id string){p,ok:=profileUI.store.Find(id);if !ok{return};profileUI.editingID=id;v:=windowsgui.EditorValuesFromSavedProfile(p);setText(profileUI.name,v.Name);setText(profileUI.serverIP,v.ServerIP);setText(profileUI.serverPort,v.ServerPort);setText(profileUI.serverFront,v.ServerFront);setText(profileUI.serverRaw,v.ServerRaw);setText(profileUI.serverName,v.ServerName);setText(profileUI.routeKey,v.RouteKey);setText(profileUI.username,v.Username);setText(profileUI.password,v.Password);setChecked(profileUI.verify,v.VerifyServer);setCombo(profileUI.fec,fecOptions,v.FEC);setText(profileUI.ifName,v.IfName);setText(profileUI.mtu,v.MTU);setCombo(profileUI.routeMode,routeOptions,v.RouteMode);setChecked(app.proxyLAN,v.ProxyLAN);setChecked(app.proxyChina,v.ProxyChina);setChecked(app.proxyOther,v.ProxyOther);setCombo(profileUI.dnsMode,dnsOptions,v.DNSMode);setText(profileUI.dnsServer,v.DNSServer);setText(profileUI.lanes,v.Lanes);setText(profileUI.idle,v.IdleTimeout);setText(profileUI.keepalive,v.Keepalive);setText(profileUI.rotMin,v.RotationMin);setText(profileUI.rotMax,v.RotationMax);setText(profileUI.tunnel,v.TunnelIPv4)}
func editorValues()windowsgui.ProfileEditorValues{return windowsgui.ProfileEditorValues{Name:getText(profileUI.name),ServerIP:getText(profileUI.serverIP),ServerPort:getText(profileUI.serverPort),ServerFront:getText(profileUI.serverFront),ServerRaw:getText(profileUI.serverRaw),ServerName:getText(profileUI.serverName),RouteKey:getText(profileUI.routeKey),Username:getText(profileUI.username),Password:getText(profileUI.password),VerifyServer:isChecked(profileUI.verify),FEC:getCombo(profileUI.fec,fecOptions),IfName:getText(profileUI.ifName),MTU:getText(profileUI.mtu),RouteMode:getCombo(profileUI.routeMode,routeOptions),ProxyLAN:isChecked(app.proxyLAN),ProxyChina:isChecked(app.proxyChina),ProxyOther:isChecked(app.proxyOther),DNSMode:getCombo(profileUI.dnsMode,dnsOptions),DNSServer:getText(profileUI.dnsServer),Lanes:getText(profileUI.lanes),IdleTimeout:getText(profileUI.idle),Keepalive:getText(profileUI.keepalive),RotationMin:getText(profileUI.rotMin),RotationMax:getText(profileUI.rotMax),TunnelIPv4:getText(profileUI.tunnel)}
func saveEditing(selectCurrent bool)error{p,ok:=profileUI.store.Find(profileUI.editingID);if !ok{return fmt.Errorf("找不到正在编辑的服务器")};var err error;p,err=editorValues().ApplyToSavedProfile(p);if err!=nil{return err};if err=profileUI.store.Upsert(p);err!=nil{return err};if selectCurrent{profileUI.store.SelectedID=p.ID};if err=windowsgui.SaveProfileStore(profileUI.storePath,profileUI.store);err!=nil{return err};refreshProfileList();if p.ID==profileUI.store.SelectedID{return loadCurrentRuntimeProfile()};return nil}
func saveActiveProfileFromUI()error{return saveEditing(true)}
func handleProfileCommand(hwnd,wParam uintptr)bool{id:=lowWord(wParam);notify:=highWord(wParam);if id==idProfileList&&notify==lbnSelChange{if app.controller.State()!=windowsruntime.RuntimeDisconnected||app.operation!=""{return true};idx,_,_:=procSendMessageW.Call(profileUI.list,lbGetCurSel,0,0);if int(idx)>=0&&int(idx)<len(profileUI.store.Profiles){loadEditor(profileUI.store.Profiles[idx].ID)};return true};switch id{case idProfileAdd: if app.controller.State()!=windowsruntime.RuntimeDisconnected{return true};p,e:=profileUI.store.Add("新服务器",defaultProfileConfig());if e!=nil{messageBox("WBD",e.Error());return true};profileUI.editingID=p.ID;_ = windowsgui.SaveProfileStore(profileUI.storePath,profileUI.store);refreshProfileList();loadEditor(p.ID);return true;case idProfileSave:if e:=saveEditing(false);e!=nil{messageBox("保存配置",e.Error())}else{messageBoxInfo("保存配置","配置已保存")};return true;case idProfileCurrent:if e:=saveEditing(true);e!=nil{messageBox("设为当前",e.Error())}else{messageBoxInfo("设为当前","已切换当前服务器")};return true;case idProfileDelete:if app.controller.State()!=windowsruntime.RuntimeDisconnected{return true};if !profileUI.store.Delete(profileUI.editingID){return true};if len(profileUI.store.Profiles)==0{p,_:=profileUI.store.Add("新服务器",defaultProfileConfig());profileUI.store.SelectedID=p.ID};profileUI.editingID=profileUI.store.SelectedID;_ = windowsgui.SaveProfileStore(profileUI.storePath,profileUI.store);_ = loadCurrentRuntimeProfile();refreshProfileList();loadEditor(profileUI.editingID);return true};return false}
func highWord(v uintptr)uintptr{return(v>>16)&0xffff}
func parseDisplayInt(s string)int{v,_:=strconv.Atoi(strings.TrimSpace(s));return v}
'''
Path('cmd/wbd-windows-gui/profile_ui_windows.go').write_text(ui,encoding='utf-8')

# Main integration: store initialization, larger Chinese window, new command dispatcher,
# connect saves/activates selected profile, editing locks while connected/busy.
replace_once('cmd/wbd-windows-gui/main_windows.go','''\tapp.profilePath = *profilePath\n\tif *profilePath != "" {\n\t\tif err := loadRuntimeProfile(*profilePath); err != nil {\n\t\t\tapp.profileErr = err\n\t\t\tmessageBox("WBD Windows GUI profile", err.Error())\n\t\t}\n\t}''','''\tapp.profilePath = *profilePath\n\tif err := initializeProfiles(*profilePath); err != nil {\n\t\tapp.profileErr = err\n\t\tmessageBox("WBD 配置", err.Error())\n\t}''')
replace_once('cmd/wbd-windows-gui/main_windows.go','title := utf16Ptr("WBD Windows Client")','title := utf16Ptr("WBD Windows 客户端")')
replace_once('cmd/wbd-windows-gui/main_windows.go','wsOverlappedWindow, 180, 140, 780, 445','wsOverlappedWindow, 70, 60, 1180, 760')
replace_once('cmd/wbd-windows-gui/main_windows.go','''\tcase wmCommand:\n\t\tswitch lowWord(wParam) {''','''\tcase wmCommand:\n\t\tif handleProfileCommand(hwnd, wParam) { return 0 }\n\t\tswitch lowWord(wParam) {''')
replace_once('cmd/wbd-windows-gui/main_windows.go','func createControls(hwnd uintptr) {','func createLegacyControls(hwnd uintptr) {')
replace_once('cmd/wbd-windows-gui/main_windows.go','''func beginConnect(hwnd uintptr) {\n\tif app.operation != "" || app.exitRequested {\n\t\treturn\n\t}\n\tif !app.profileReady {''','''func beginConnect(hwnd uintptr) {\n\tif app.operation != "" || app.exitRequested {\n\t\treturn\n\t}\n\tif err := saveActiveProfileFromUI(); err != nil { messageBox("连接配置无效", err.Error()); return }\n\tif !app.profileReady {''')
replace_once('cmd/wbd-windows-gui/main_windows.go','''\tsetEnabled(app.proxyOther, routingEnabled)\n\tsetEnabled(app.hideButton, !app.exitRequested)''','''\tsetEnabled(app.proxyOther, routingEnabled)\n\tsetProfileEditorEnabled(routingEnabled)\n\tsetEnabled(app.hideButton, !app.exitRequested)''')
replace_once('cmd/wbd-windows-gui/main_windows.go','copyUTF16(nid.SzTip[:], "WBD Windows Client")','copyUTF16(nid.SzTip[:], "WBD Windows 客户端")')
replace_once('cmd/wbd-windows-gui/main_windows.go','appendMenu(menu, mfString, idTrayShow, "Show WBD")','appendMenu(menu, mfString, idTrayShow, "显示 WBD")')
replace_once('cmd/wbd-windows-gui/main_windows.go','appendMenu(menu, mfString, idTrayExit, "Exit WBD")','appendMenu(menu, mfString, idTrayExit, "退出 WBD")')
# visible runtime status localization (keep underlying errors verbatim after the prefix)
p=Path('cmd/wbd-windows-gui/main_windows.go');s=p.read_text(encoding='utf-8')
for a,b in {
"Status: connecting; preparing routing policy and CN IP ranges if required":"状态：正在连接，准备路由策略和国内 IP 列表",
"Status: disconnecting; removing routes, DNS and IPv6 block":"状态：正在断开，清理路由、DNS 与 IPv6 阻断",
"Status: reconnecting transport lanes with make-before-break; Wintun/routes stay active":"状态：正在重连传输线路，Wintun 与路由保持工作",
"Status: Npcap setup; downloading and verifying official installer":"状态：正在安装/修复 Npcap 并验证官方安装包",
"Status: disconnected; connect failed: ":"状态：未连接；连接失败：",
"Status: connected; IPv4 WBD active; device IPv6 blocked":"状态：已连接；IPv4 WBD 工作中，设备 IPv6 已阻断",
"Status: disconnected; WBD routes/DNS/IPv6 block removed and runtime stopped":"状态：已断开；路由、DNS、IPv6 阻断和运行进程均已清理",
"Status: disconnected; Npcap ready":"状态：未连接；Npcap 已就绪",
"Status: connected; transport lanes refreshed; Wintun/routes stayed active":"状态：已连接；传输线路已刷新，Wintun/路由未中断",
}.items(): s=s.replace(a,b)
p.write_text(s,encoding='utf-8')
print('Chinese multi-profile UI patch applied')
